//go:build !linux

package main

import (
	"fmt"
	"net"

	"golang.org/x/net/icmp"
)

// openICMPv6Socket opens a raw ICMPv6 socket on non-Linux platforms.
// SO_BINDTODEVICE is Linux-only; other platforms use icmp.ListenPacket directly.
func openICMPv6Socket(iface *net.Interface) (net.PacketConn, error) {
	conn, err := icmp.ListenPacket("ip6:ipv6-icmp", "::")
	if err != nil {
		return nil, fmt.Errorf("failed to open ICMPv6 socket: %v", err)
	}
	return conn, nil
}
