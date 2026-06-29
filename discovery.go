package main

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/grandcat/zeroconf"
)

// browseMatterDevices browses for Matter devices solely to extract Thread mesh prefixes
// from their ULA addresses — a fallback for TBRs that don't advertise omr= in mDNS.
func browseMatterDevices(state *DaemonState, done <-chan struct{}) {
	browseService("_matter._tcp", done, 5*time.Minute, func(entry *zeroconf.ServiceEntry) {
		for _, ip := range extractIPv6s(entry) {
			if len(ip) == 16 && (ip[0]&0xfe) == 0xfc {
				cidr := calculateCIDR64(ip)
				if cidr == "" {
					continue
				}
				state.mu.Lock()
				if _, known := state.ThreadMeshPrefixes[cidr]; !known {
					logInfo("Discovered Thread mesh prefix from Matter device %s: %s",
						extractRouterName(entry.ServiceInstanceName()), cidr)
				}
				state.ThreadMeshPrefixes[cidr] = time.Now()
				state.mu.Unlock()
			}
		}
	})
}

// browseThreadBorderRouters continuously browses for Thread Border Routers using zeroconf.
func browseThreadBorderRouters(state *DaemonState, done <-chan struct{}) {
	browseService("_meshcop._udp", done, 5*time.Minute, func(entry *zeroconf.ServiceEntry) {
		ips := extractIPv6s(entry)
		logDebug("Thread Border Router mDNS entry: name=%s ips=%v txt=%v",
			entry.ServiceInstanceName(), ips, entry.Text)
		if len(ips) == 0 {
			return
		}
		mergeRouters(state, []ThreadBorderRouter{{
			Name:      extractRouterName(entry.ServiceInstanceName()),
			IPv6Addrs: ips,
			LastSeen:  time.Now(),
		}})
		if prefix := extractOMRPrefix(entry.Text); prefix != "" {
			state.mu.Lock()
			if _, known := state.ThreadMeshPrefixes[prefix]; !known {
				logInfo("Discovered Thread mesh prefix from OMR TXT record (%s): %s",
					extractRouterName(entry.ServiceInstanceName()), prefix)
			}
			state.ThreadMeshPrefixes[prefix] = time.Now()
			state.mu.Unlock()
		}
	})
}

// maskPrefix zeroes out host bits beyond prefixLen.
func maskPrefix(ip net.IP, prefixLen int) net.IP {
	masked := make(net.IP, 16)
	copy(masked, ip)
	mask := net.CIDRMask(prefixLen, 128)
	for i := range masked {
		masked[i] &= mask[i]
	}
	return masked
}

// extractOMRPrefix parses the Thread Off-Mesh Route prefix from _meshcop._udp TXT records.
// The omr= field is: 1 byte prefix-length, followed by ceil(prefixLen/8) prefix bytes.
// The prefix bytes are not zero-padded to 16 bytes — only significant bytes are included.
func extractOMRPrefix(txt []string) string {
	for _, field := range txt {
		if !strings.HasPrefix(field, "omr=") {
			continue
		}
		val := []byte(field[4:])
		logDebug("extractOMRPrefix: val len=%d bytes=%x", len(val), val)
		if len(val) < 2 {
			logDebug("extractOMRPrefix: too short")
			continue
		}
		prefixLen := int(val[0])
		logDebug("extractOMRPrefix: prefixLen=%d", prefixLen)
		if prefixLen == 0 || prefixLen > 128 {
			logDebug("extractOMRPrefix: invalid prefixLen")
			continue
		}
		// Pad to 16 bytes
		prefix := make(net.IP, 16)
		copy(prefix, val[1:])
		logDebug("extractOMRPrefix: prefix=%s ula=%v", prefix.String(), (prefix[0]&0xfe) == 0xfc)
		// Only accept ULA prefixes (fc00::/7)
		if (prefix[0] & 0xfe) != 0xfc {
			continue
		}
		masked := maskPrefix(prefix, prefixLen)
		return fmt.Sprintf("%s/%d", masked.String(), prefixLen)
	}
	return ""
}

// browseService runs a zeroconf Browse loop for the given service type until done is closed.
// On error it waits 5 seconds before restarting. The handler is called for each entry.
// If refreshInterval > 0, the browse is restarted on that interval to send fresh mDNS queries,
// which forces devices to re-announce and prevents stale state.
// The key rule: never close the entries channel — only cancel the context; zeroconf owns it.
func browseService(service string, done <-chan struct{}, refreshInterval time.Duration, handler func(*zeroconf.ServiceEntry)) {
	for {
		ctx, cancel := context.WithCancel(context.Background())

		// Stop browsing when done is closed, or restart after refreshInterval.
		go func() {
			if refreshInterval > 0 {
				select {
				case <-done:
					cancel()
				case <-time.After(refreshInterval):
					logDebug("mDNS browse %s: periodic refresh", service)
					cancel()
				case <-ctx.Done():
				}
			} else {
				select {
				case <-done:
					cancel()
				case <-ctx.Done():
				}
			}
		}()

		resolver, err := zeroconf.NewResolver()
		if err != nil {
			cancel()
			logWarn("mDNS browse %s: failed to create resolver: %v — retrying in 5s", service, err)
			select {
			case <-done:
				return
			case <-time.After(5 * time.Second):
				continue
			}
		}

		// zeroconf owns entries and closes it when ctx is cancelled. Never close it here.
		entries := make(chan *zeroconf.ServiceEntry)
		go func() {
			for entry := range entries {
				handler(entry)
			}
		}()

		if err := resolver.Browse(ctx, service, "local.", entries); err != nil {
			cancel()
			logWarn("mDNS browse %s: error: %v — retrying in 5s", service, err)
			select {
			case <-done:
				return
			case <-time.After(5 * time.Second):
				continue
			}
		}

		// Browse returned — either context was cancelled (done) or an error.
		<-ctx.Done()
		cancel()

		select {
		case <-done:
			return
		default:
			// Context was cancelled for another reason; restart.
			logDebug("mDNS browse %s: restarting", service)
			time.Sleep(5 * time.Second)
		}
	}
}

// extractIPv6s returns all IPv6 addresses from a zeroconf ServiceEntry.
func extractIPv6s(entry *zeroconf.ServiceEntry) []net.IP {
	var ips []net.IP
	for _, ip := range entry.AddrIPv6 {
		if ip.To4() != nil || ip.To16() == nil {
			continue
		}
		ips = appendUnique(ips, ip)
	}
	return ips
}

// appendUnique appends ip to the slice only if not already present.
func appendUnique(ips []net.IP, ip net.IP) []net.IP {
	for _, existing := range ips {
		if existing.Equal(ip) {
			return ips
		}
	}
	return append(ips, ip)
}

// calculateCIDR64 calculates the /64 CIDR block for an IPv6 address.
// Returns "" for nil, IPv4, or unrecognised addresses.
func calculateCIDR64(ip net.IP) string {
	if ip == nil || ip.To4() != nil {
		return ""
	}
	if ip.To16() != nil {
		cidr := make(net.IP, 16)
		copy(cidr, ip[:8])
		return fmt.Sprintf("%s/64", cidr.String())
	}
	return ""
}

// extractRouterName extracts the simple router name from its FQDN and unescapes it.
func extractRouterName(fqdn string) string {
	name := fqdn
	if idx := strings.Index(fqdn, "."); idx != -1 {
		name = fqdn[:idx]
	}

	name = strings.ReplaceAll(name, "\\ ", " ")
	name = strings.ReplaceAll(name, "\\(", "(")
	name = strings.ReplaceAll(name, "\\)", ")")
	name = strings.ReplaceAll(name, "\\", "")

	return name
}

// formatDuration formats a duration to a human-readable string (e.g., "1h30m", "45m", "30s")
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	} else if d < time.Hour {
		return fmt.Sprintf("%.0fm", d.Minutes())
	} else {
		hours := int(d.Hours())
		minutes := int(d.Minutes()) % 60
		if minutes == 0 {
			return fmt.Sprintf("%dh", hours)
		}
		return fmt.Sprintf("%dh%dm", hours, minutes)
	}
}

// isRoutableCIDR checks if a CIDR block is routable (not link-local, loopback, etc.)
func isRoutableCIDR(cidr string) bool {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return false
	}

	ip := network.IP

	if ip[0] == 0xfe && (ip[1]&0xc0) == 0x80 {
		return false
	}
	if ip.Equal(net.ParseIP("::1")) {
		return false
	}
	if ip.Equal(net.ParseIP("::")) {
		return false
	}
	if ip[0] == 0xff {
		return false
	}
	if len(ip) >= 4 && ip[0] == 0x20 && ip[1] == 0x01 && ip[2] == 0x0d && ip[3] == 0xb8 {
		return false
	}
	if len(ip) >= 4 && ip[0] == 0x20 && ip[1] == 0x01 && ip[2] == 0x00 && ip[3] == 0x00 {
		return false
	}
	if len(ip) >= 2 && ip[0] == 0x20 && ip[1] == 0x02 {
		return false
	}

	return true
}

// isRoutableRouterAddress checks if a Thread Border Router IPv6 address is routable
func isRoutableRouterAddress(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.To4() != nil {
		return false
	}
	if ip.To16() != nil {
		if ip[0] == 0xfe && (ip[1]&0xc0) == 0x80 {
			return false
		}
		if ip.Equal(net.ParseIP("::1")) {
			return false
		}
		if ip.Equal(net.ParseIP("::")) {
			return false
		}
		if ip[0] == 0xff {
			return false
		}
		if len(ip) >= 1 && (ip[0]&0xfe) == 0xfc {
			return false
		}
		if len(ip) >= 4 && ip[0] == 0x20 && ip[1] == 0x01 && ip[2] == 0x0d && ip[3] == 0xb8 {
			return false
		}
		if len(ip) >= 4 && ip[0] == 0x20 && ip[1] == 0x01 && ip[2] == 0x00 && ip[3] == 0x00 {
			return false
		}
		if len(ip) >= 2 && ip[0] == 0x20 && ip[1] == 0x02 {
			return false
		}
	}

	return true
}
