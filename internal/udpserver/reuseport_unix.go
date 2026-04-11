//go:build linux || android || darwin || freebsd || netbsd || openbsd || dragonfly

package udpserver

import (
	"context"
	"net"
	"syscall"

	"golang.org/x/sys/unix"
)

func listenUDPReusePort(addr *net.UDPAddr) (*net.UDPConn, error) {
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			var ctrlErr error
			if err := c.Control(func(fd uintptr) {
				if err := unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1); err != nil {
					ctrlErr = err
					return
				}
				if err := unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1); err != nil {
					ctrlErr = err
					return
				}
			}); err != nil {
				return err
			}
			return ctrlErr
		},
	}

	pc, err := lc.ListenPacket(context.Background(), "udp", addr.String())
	if err != nil {
		return nil, err
	}
	return pc.(*net.UDPConn), nil
}
