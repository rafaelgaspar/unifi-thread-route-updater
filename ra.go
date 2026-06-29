package main

import (
	"fmt"
	"net"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv6"
)

const (
	icmpv6TypeRA        = 134
	ndpOptionPrefixInfo = 3
)

// listenForRouterAdvertisements listens for ICMPv6 Router Advertisements on the
// configured mDNS interface and extracts fd:: (ULA) prefixes from prefix information
// options. These are the Thread mesh prefixes advertised by Thread Border Routers.
// Prefixes change when the Thread network is re-formed, so we track them dynamically.
func listenForRouterAdvertisements(state *DaemonState, done <-chan struct{}) {
	for {
		if err := runRAListener(state, done); err != nil {
			logWarn("RA listener error: %v — retrying in 5s", err)
		}
		select {
		case <-done:
			return
		case <-time.After(5 * time.Second):
		}
	}
}

func runRAListener(state *DaemonState, done <-chan struct{}) error {
	conn, err := icmp.ListenPacket("ip6:ipv6-icmp", "::")
	if err != nil {
		return fmt.Errorf("failed to open ICMPv6 socket: %v", err)
	}
	defer conn.Close()

	pc := conn.IPv6PacketConn()
	if err := pc.SetControlMessage(ipv6.FlagInterface, true); err != nil {
		return fmt.Errorf("failed to set control message: %v", err)
	}

	iface := getMDNSInterface()
	logInfo("Listening for ICMPv6 Router Advertisements on %s", ifaceName(iface))

	buf := make([]byte, 1500)
	for {
		select {
		case <-done:
			return nil
		default:
		}

		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, cm, _, err := pc.ReadFrom(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			return fmt.Errorf("read error: %v", err)
		}

		// Filter by interface if one is configured.
		if iface != nil && cm != nil && cm.IfIndex != iface.Index {
			continue
		}

		// ICMPv6 type is the first byte.
		if n < 1 || buf[0] != icmpv6TypeRA {
			continue
		}

		// RA body starts at byte 4 (after type, code, checksum).
		// Router Advertisement format (RFC 4861):
		//   1 byte  type (134)
		//   1 byte  code (0)
		//   2 bytes checksum
		//   1 byte  current hop limit
		//   1 byte  flags
		//   2 bytes router lifetime
		//   4 bytes reachable time
		//   4 bytes retrans timer
		//   options...
		if n < 16 {
			continue
		}

		prefixes := parseRAPrefixes(buf[:n])
		if len(prefixes) == 0 {
			continue
		}

		state.mu.Lock()
		for _, p := range prefixes {
			if _, known := state.ThreadMeshPrefixes[p]; !known {
				logInfo("Discovered Thread mesh prefix via RA: %s", p)
			}
			state.ThreadMeshPrefixes[p] = time.Now()
		}
		state.mu.Unlock()
	}
}

// parseRAPrefixes parses NDP options from an RA packet and returns fd:: CIDR strings.
// RA packet layout: 16-byte header, then variable-length options.
// Each option: 1 byte type, 1 byte length (in units of 8 bytes), then data.
func parseRAPrefixes(pkt []byte) []string {
	if len(pkt) < 16 {
		return nil
	}

	var prefixes []string
	opts := pkt[16:]

	for len(opts) >= 2 {
		optType := opts[0]
		optLen := int(opts[1]) * 8 // length in 8-byte units
		if optLen == 0 || len(opts) < optLen {
			break
		}

		if optType == ndpOptionPrefixInfo && optLen >= 32 {
			// Prefix Information option (RFC 4861 §6.3.4):
			//   1 byte  type (3)
			//   1 byte  length (4 = 32 bytes)
			//   1 byte  prefix length
			//   1 byte  flags (L, A, R bits)
			//   4 bytes valid lifetime
			//   4 bytes preferred lifetime
			//   4 bytes reserved
			//  16 bytes prefix
			prefixLen := int(opts[2])
			prefix := net.IP(opts[16:32])

			// Only ULA prefixes (fc00::/7 — first byte 0xfc or 0xfd)
			if len(prefix) == 16 && (prefix[0]&0xfe) == 0xfc {
				// Mask the prefix to prefixLen bits
				masked := maskPrefix(prefix, prefixLen)
				cidr := fmt.Sprintf("%s/%d", masked.String(), prefixLen)
				prefixes = append(prefixes, cidr)
			}
		}

		opts = opts[optLen:]
	}
	return prefixes
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

func ifaceName(iface *net.Interface) string {
	if iface == nil {
		return "all interfaces"
	}
	return iface.Name
}

