//go:build !windows

package netutil

import (
	"context"
	"net"
	"syscall"
)

func listenDualStack(address string) (net.Listener, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return net.Listen("tcp", address)
	}
	if ip := net.ParseIP(host); ip != nil && ip.To4() != nil {
		return net.Listen("tcp4", address)
	}
	if host != "::" {
		return net.Listen("tcp", address)
	}
	var config net.ListenConfig
	config.Control = func(_, _ string, raw syscall.RawConn) error {
		var socketErr error
		if err := raw.Control(func(fd uintptr) {
			socketErr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IPV6, syscall.IPV6_V6ONLY, 0)
		}); err != nil {
			return err
		}
		return socketErr
	}
	return config.Listen(context.Background(), "tcp", address)
}
