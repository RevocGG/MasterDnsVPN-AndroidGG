//go:build !linux && !android && !darwin && !freebsd && !netbsd && !openbsd && !dragonfly

package udpserver

import (
	"errors"
	"net"
)

var errReusePortUnsupported = errors.New("reuseport unsupported")

func listenUDPReusePort(addr *net.UDPAddr) (*net.UDPConn, error) {
	return nil, errReusePortUnsupported
}
