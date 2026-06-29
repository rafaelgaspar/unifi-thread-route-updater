//go:build linux

package main

import (
	"fmt"
	"net"
	"os"
	"syscall"
)

// openICMPv6Socket opens a raw ICMPv6 socket bound to iface via SO_BINDTODEVICE
// so the kernel delivers multicast packets (like RAs) that it would otherwise
// only handle internally.
func openICMPv6Socket(iface *net.Interface) (net.PacketConn, error) {
	fd, err := syscall.Socket(syscall.AF_INET6, syscall.SOCK_RAW, syscall.IPPROTO_ICMPV6)
	if err != nil {
		return nil, fmt.Errorf("failed to open ICMPv6 socket: %v", err)
	}

	if iface != nil {
		if err := syscall.SetsockoptString(fd, syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, iface.Name); err != nil {
			syscall.Close(fd) //nolint:errcheck
			return nil, fmt.Errorf("SO_BINDTODEVICE failed: %v", err)
		}
		logDebug("Bound raw socket to %s via SO_BINDTODEVICE", iface.Name)
	}

	f := os.NewFile(uintptr(fd), "icmpv6")
	conn, err := net.FilePacketConn(f)
	f.Close() //nolint:errcheck
	if err != nil {
		return nil, fmt.Errorf("FilePacketConn failed: %v", err)
	}
	return conn, nil
}
