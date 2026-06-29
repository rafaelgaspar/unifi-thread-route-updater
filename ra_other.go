//go:build !linux

package main

import (
	"fmt"
	"net"
)

// openICMPv6Socket opens a raw ICMPv6 socket on non-Linux platforms.
// Uses net.ListenPacket which returns *net.IPConn, satisfying ipv6.NewPacketConn's
// internal interface requirements (unlike icmp.ListenPacket's *icmp.PacketConn).
func openICMPv6Socket(iface *net.Interface) (net.PacketConn, error) {
	conn, err := net.ListenPacket("ip6:ipv6-icmp", "::")
	if err != nil {
		return nil, fmt.Errorf("failed to open ICMPv6 socket: %v", err)
	}
	return conn, nil
}
