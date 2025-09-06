package main

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/grandcat/zeroconf"
)

// discoverMatterDevices discovers Matter devices using mDNS
func discoverMatterDevices() ([]DeviceInfo, error) {
	var devices []DeviceInfo
	serviceType := "_matter._tcp"

	serviceDevices, err := discoverService(serviceType, "Matter")
	if err != nil {
		return devices, fmt.Errorf("error discovering %s: %v", serviceType, err)
	}
	devices = append(devices, serviceDevices...)

	return devices, nil
}

// discoverThreadBorderRouters discovers Thread Border Routers using mDNS
func discoverThreadBorderRouters() ([]ThreadBorderRouter, error) {
	var routers []ThreadBorderRouter
	serviceType := "_meshcop._udp"

	serviceRouters, err := discoverThreadService(serviceType)
	if err != nil {
		return routers, fmt.Errorf("error discovering %s: %v", serviceType, err)
	}
	routers = append(routers, serviceRouters...)

	return routers, nil
}

// discoverService discovers services of a specific type
func discoverService(serviceType, deviceType string) ([]DeviceInfo, error) {
	var devices []DeviceInfo

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Browse for services
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return devices, fmt.Errorf("failed to initialize resolver: %v", err)
	}

	entries := make(chan *zeroconf.ServiceEntry)
	done := make(chan bool)

	go func() {
		defer func() {
			select {
			case <-done:
				// Channel already closed
			default:
				close(entries)
			}
		}()
		err := resolver.Browse(ctx, serviceType, "local.", entries)
		if err != nil {
			fmt.Printf("Failed to browse: %v\n", err)
		}
	}()

	// Process entries
	for entry := range entries {
		if entry == nil {
			continue
		}

		ipv6Addrs := extractIPv6Addresses(entry)
		if len(ipv6Addrs) == 0 {
			continue
		}

		for _, ip := range ipv6Addrs {
			device := DeviceInfo{
				Name:     entry.Instance,
				IPv6Addr: ip,
				Services: []string{serviceType},
			}
			devices = append(devices, device)
		}
	}

	// Signal that we're done processing
	close(done)

	return devices, nil
}

// discoverThreadService discovers Thread services
func discoverThreadService(serviceType string) ([]ThreadBorderRouter, error) {
	var routers []ThreadBorderRouter

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Browse for services
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return routers, fmt.Errorf("failed to initialize resolver: %v", err)
	}

	entries := make(chan *zeroconf.ServiceEntry)
	done := make(chan bool)

	go func() {
		defer func() {
			select {
			case <-done:
				// Channel already closed
			default:
				close(entries)
			}
		}()
		err := resolver.Browse(ctx, serviceType, "local.", entries)
		if err != nil {
			fmt.Printf("Failed to browse: %v\n", err)
		}
	}()

	// Process entries
	for entry := range entries {
		if entry == nil {
			continue
		}

		ipv6Addrs := extractIPv6Addresses(entry)
		if len(ipv6Addrs) == 0 {
			continue
		}

		for _, ip := range ipv6Addrs {
			router := ThreadBorderRouter{
				Name:     extractRouterName(entry.Instance),
				IPv6Addr: ip,
				CIDR:     calculateCIDR64(ip),
			}
			routers = append(routers, router)
		}
	}

	// Signal that we're done processing
	close(done)

	return routers, nil
}

// extractIPv6Addresses extracts IPv6 addresses from zeroconf entry
func extractIPv6Addresses(entry *zeroconf.ServiceEntry) []net.IP {
	var ipv6Addrs []net.IP

	// Only use real IPv6 addresses, not IPv4 mapped addresses
	if entry.AddrIPv6 != nil {
		for _, ip := range entry.AddrIPv6 {
			// Check if it's a real IPv6 address (not IPv4 mapped)
			if ip.To4() == nil && ip.To16() != nil {
				ipv6Addrs = append(ipv6Addrs, ip)
			}
		}
	}

	// For Thread networks, we need IPv6 addresses
	// If no IPv6 addresses found, skip this device
	if len(ipv6Addrs) == 0 {
		return ipv6Addrs
	}

	return ipv6Addrs
}

// calculateCIDR64 calculates the /64 CIDR block for an IPv6 address
func calculateCIDR64(ip net.IP) string {
	if ip == nil {
		return ""
	}

	// For IPv4 addresses, return a placeholder
	if ip.To4() != nil {
		return "::/64"
	}

	// For IPv6 addresses, calculate /64 CIDR
	if ip.To16() != nil {
		// Take the first 8 bytes (64 bits) and set the rest to 0
		cidr := make(net.IP, 16)
		copy(cidr, ip[:8])
		return fmt.Sprintf("%s/64", cidr.String())
	}

	return ""
}

// extractRouterName extracts the simple router name from its FQDN
func extractRouterName(fqdn string) string {
	if idx := strings.Index(fqdn, "."); idx != -1 {
		return fqdn[:idx]
	}
	return fqdn
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
	// Parse the CIDR to get the network
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return false
	}

	ip := network.IP

	// Check for non-routable IPv6 address ranges
	// fe80::/10 - Link-local addresses
	if ip[0] == 0xfe && (ip[1]&0xc0) == 0x80 {
		return false
	}

	// ::1/128 - Loopback address
	if ip.Equal(net.ParseIP("::1")) {
		return false
	}

	// ::/128 - Unspecified address
	if ip.Equal(net.ParseIP("::")) {
		return false
	}

	// ff00::/8 - Multicast addresses
	if ip[0] == 0xff {
		return false
	}

	// 2001:db8::/32 - Documentation prefix (should not be routed)
	if len(ip) >= 4 && ip[0] == 0x20 && ip[1] == 0x01 && ip[2] == 0x0d && ip[3] == 0xb8 {
		return false
	}

	// 2001::/32 - Teredo tunneling (usually not routed)
	if len(ip) >= 4 && ip[0] == 0x20 && ip[1] == 0x01 && ip[2] == 0x00 && ip[3] == 0x00 {
		return false
	}

	// 2002::/16 - 6to4 tunneling (deprecated, usually not routed)
	if len(ip) >= 2 && ip[0] == 0x20 && ip[1] == 0x02 {
		return false
	}

	// Note: fdc0::/7 (Unique Local Addresses) are valid for Thread Networks
	// but Thread Border Routers should use public IPv6 addresses

	return true
}

// isRoutableRouterAddress checks if a Thread Border Router IPv6 address is routable
// Thread Border Routers should only use public IPv6 addresses, not link-local or ULA
func isRoutableRouterAddress(ip net.IP) bool {
	if ip == nil {
		return false
	}

	// For IPv4 addresses, return false (we only want IPv6)
	if ip.To4() != nil {
		return false
	}

	// For IPv6 addresses, check for non-routable ranges
	if ip.To16() != nil {
		// fe80::/10 - Link-local addresses
		if ip[0] == 0xfe && (ip[1]&0xc0) == 0x80 {
			return false
		}

		// ::1/128 - Loopback address
		if ip.Equal(net.ParseIP("::1")) {
			return false
		}

		// ::/128 - Unspecified address
		if ip.Equal(net.ParseIP("::")) {
			return false
		}

		// ff00::/8 - Multicast addresses
		if ip[0] == 0xff {
			return false
		}

		// fc00::/7 - Unique Local Addresses (ULA) - Thread Border Routers should use public addresses
		if len(ip) >= 1 && (ip[0]&0xfe) == 0xfc {
			return false
		}

		// 2001:db8::/32 - Documentation prefix
		if len(ip) >= 4 && ip[0] == 0x20 && ip[1] == 0x01 && ip[2] == 0x0d && ip[3] == 0xb8 {
			return false
		}

		// 2001::/32 - Teredo tunneling
		if len(ip) >= 4 && ip[0] == 0x20 && ip[1] == 0x01 && ip[2] == 0x00 && ip[3] == 0x00 {
			return false
		}

		// 2002::/16 - 6to4 tunneling
		if len(ip) >= 2 && ip[0] == 0x20 && ip[1] == 0x02 {
			return false
		}
	}

	return true
}
