package client

import "net"

// DialUDPFunc / ListenUDPFunc are replaceable hooks for creating UDP sockets.
// By default they call the standard net functions.  When a VPN TUN interface is
// active, the mobile layer replaces them with versions that call
// VpnService.protect(fd) on the raw socket so Android excludes the fd from the
// TUN route, preventing an infinite routing loop.
var (
	DialUDPFunc   = net.DialUDP
	ListenUDPFunc = net.ListenUDP
)
