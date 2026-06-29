package main

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/grandcat/zeroconf"
)

// browseMatterDevices continuously browses for Matter devices using zeroconf.
// zeroconf sends active queries AND listens passively for announcements, so
// devices that don't respond to queries are still discovered when they announce.
// The browse runs until done is closed, restarting on error with a short backoff.
func browseMatterDevices(state *DaemonState, done <-chan struct{}) {
	browseService("_matter._tcp", done, func(entry *zeroconf.ServiceEntry) {
		ips := extractIPv6s(entry)
		if len(ips) == 0 {
			return
		}
		mergeDevices(state, []DeviceInfo{{
			Name:      entry.ServiceInstanceName(),
			IPv6Addrs: ips,
			LastSeen:  time.Now(),
		}})
	})
}

// browseThreadBorderRouters continuously browses for Thread Border Routers using zeroconf.
func browseThreadBorderRouters(state *DaemonState, done <-chan struct{}) {
	browseService("_meshcop._udp", done, func(entry *zeroconf.ServiceEntry) {
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
	})
}

// browseService runs a zeroconf Browse loop for the given service type until done is closed.
// On error it waits 5 seconds before restarting. The handler is called for each entry.
// The key rule: never close the entries channel — only cancel the context; zeroconf owns it.
func browseService(service string, done <-chan struct{}, handler func(*zeroconf.ServiceEntry)) {
	for {
		ctx, cancel := context.WithCancel(context.Background())

		// Stop browsing when done is closed.
		go func() {
			select {
			case <-done:
				cancel()
			case <-ctx.Done():
			}
		}()

		iface := getMDNSInterface()
		var opts []zeroconf.ClientOption
		if iface != nil {
			opts = append(opts, zeroconf.SelectIfaces([]net.Interface{*iface}))
		}

		resolver, err := zeroconf.NewResolver(opts...)
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
