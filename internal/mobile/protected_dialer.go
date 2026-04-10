//go:build linux || android

package mobile

import (
	"fmt"
	"net"
)

// protectedDialUDP works like net.DialUDP but calls ProtectFD on the raw
// socket fd so Android VpnService.protect() excludes it from the TUN route.
func protectedDialUDP(network string, laddr, raddr *net.UDPAddr) (*net.UDPConn, error) {
	conn, err := net.DialUDP(network, laddr, raddr)
	if err != nil {
		return nil, err
	}
	if err := protectUDPConn(conn); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

// protectedListenUDP works like net.ListenUDP but calls ProtectFD on the raw
// socket fd so Android VpnService.protect() excludes it from the TUN route.
func protectedListenUDP(network string, laddr *net.UDPAddr) (*net.UDPConn, error) {
	conn, err := net.ListenUDP(network, laddr)
	if err != nil {
		return nil, err
	}
	if err := protectUDPConn(conn); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

// protectUDPConn extracts the raw fd from a net.UDPConn and calls ProtectFD.
func protectUDPConn(conn *net.UDPConn) error {
	raw, err := conn.SyscallConn()
	if err != nil {
		return fmt.Errorf("protect: SyscallConn: %w", err)
	}
	var protectErr error
	err = raw.Control(func(fd uintptr) {
		if !ProtectFD(int32(fd)) {
			protectErr = fmt.Errorf("VpnService.protect(%d) returned false", fd)
		}
	})
	if err != nil {
		return fmt.Errorf("protect: Control: %w", err)
	}
	return protectErr
}
