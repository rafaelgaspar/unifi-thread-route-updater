package main

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/hashicorp/mdns"
)

// discoverMatterDevices discovers Matter devices using mDNS
func discoverMatterDevices() ([]DeviceInfo, error) {
	entries, err := queryMDNS("_matter._tcp", 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("error discovering _matter._tcp: %v", err)
	}

	var devices []DeviceInfo
	for _, entry := range entries {
		ip := extractIPv6(entry)
		if ip == nil {
			continue
		}
		devices = append(devices, DeviceInfo{
			Name:     entry.Name,
			IPv6Addr: ip,
			LastSeen: time.Now(),
		})
	}
	return devices, nil
}

// discoverThreadBorderRouters discovers Thread Border Routers using mDNS
func discoverThreadBorderRouters() ([]ThreadBorderRouter, error) {
	entries, err := queryMDNS("_meshcop._udp", 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("error discovering _meshcop._udp: %v", err)
	}

	var routers []ThreadBorderRouter
	for _, entry := range entries {
		ip := extractIPv6(entry)
		if ip == nil {
			continue
		}
		routers = append(routers, ThreadBorderRouter{
			Name:     extractRouterName(entry.Name),
			IPv6Addr: ip,
			CIDR:     calculateCIDR64(ip),
			LastSeen: time.Now(),
		})
	}
	return routers, nil
}

// queryMDNS performs an mDNS query and returns all entries found within the timeout.
func queryMDNS(service string, timeout time.Duration) ([]*mdns.ServiceEntry, error) {
	entriesCh := make(chan *mdns.ServiceEntry, 64)
	params := &mdns.QueryParam{
		Service: service,
		Domain:  "local",
		Timeout: timeout,
		Entries: entriesCh,
	}

	var entries []*mdns.ServiceEntry
	done := make(chan struct{})
	go func() {
		defer close(done)
		for e := range entriesCh {
			if e != nil {
				entries = append(entries, e)
			}
		}
	}()

	if err := mdns.Query(params); err != nil {
		return nil, err
	}
	close(entriesCh)
	<-done
	return entries, nil
}

// extractIPv6 returns the first routable IPv6 address from an mDNS entry.
func extractIPv6(entry *mdns.ServiceEntry) net.IP {
	ip := entry.AddrV6
	if ip == nil && entry.AddrV6IPAddr != nil {
		ip = entry.AddrV6IPAddr.IP
	}
	if ip == nil {
		return nil
	}
	if ip.To4() != nil {
		return nil // IPv4-mapped, skip
	}
	if ip.To16() == nil {
		return nil
	}
	return ip
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

// extractRouterName extracts the simple router name from its FQDN and unescapes it
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
