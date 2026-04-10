//go:build linux || android

// ==============================================================================
// MasterDnsVPN
// Author: MasterkinG32
// Github: https://github.com/masterking32
// Year: 2026
// ==============================================================================
// Package mobile — tun_bridge.go
//
// Implements a tun2socks bridge using gVisor netstack.
// Reads raw IP packets from a TUN file descriptor (provided by Android's
// VpnService.establish()), sets up a virtual TCP/IP stack, and forwards
// every TCP/UDP flow through the MasterDnsVPN SOCKS5 proxy.
// ==============================================================================
package mobile

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"masterdnsvpn-go/internal/client"

	"github.com/sagernet/gvisor/pkg/tcpip"
	"github.com/sagernet/gvisor/pkg/tcpip/adapters/gonet"
	"github.com/sagernet/gvisor/pkg/tcpip/header"
	"github.com/sagernet/gvisor/pkg/tcpip/link/fdbased"
	"github.com/sagernet/gvisor/pkg/tcpip/network/ipv4"
	"github.com/sagernet/gvisor/pkg/tcpip/network/ipv6"
	"github.com/sagernet/gvisor/pkg/tcpip/stack"
	"github.com/sagernet/gvisor/pkg/tcpip/transport/tcp"
	"github.com/sagernet/gvisor/pkg/tcpip/transport/udp"
	"github.com/sagernet/gvisor/pkg/waiter"
)

const tunNICID tcpip.NICID = 1

// Bandwidth counters — updated atomically from relay goroutines.
var (
	tunBytesUp   atomic.Int64
	tunBytesDown atomic.Int64
)

// GetTunBandwidth returns (upload, download) bytes since bridge start.
func GetTunBandwidth() (int64, int64) {
	return tunBytesUp.Load(), tunBytesDown.Load()
}

// ── Tuning constants ───────────────────────────────────────────────────────────

const (
	// relayBufSize — each recycled relay buffer is 64 KB, matching typical
	// CDN chunk sizes, HTTP/2 DATA frame sizes, and video streaming segments.
	relayBufSize = 64 * 1024

	// tcpMaxInFlight — maximum concurrent TCP connections being proxied.
	tcpMaxInFlight = 4096

	// tcpDialTimeout — timeout for dialling the local SOCKS5 proxy.
	tcpDialTimeout = 5 * time.Second

	// socks5HandshakeTimeout — deadline for the entire SOCKS5 handshake.
	socks5HandshakeTimeout = 30 * time.Second

	// udpReadTimeout is the per-iteration deadline on the UDP relay socket
	// (allows ctx.Done detection without blocking forever).
	udpReadTimeout = 5 * time.Second

)

// ── Buffer pool ────────────────────────────────────────────────────────────────

var relayBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, relayBufSize)
		return &b
	},
}

func getRelayBuf() []byte  { return *(relayBufPool.Get().(*[]byte)) }
func putRelayBuf(b []byte) { relayBufPool.Put(&b) }

// ── Bridge logging ─────────────────────────────────────────────────────────────
//
// bridgeLog delivers messages both to Android logcat (via stderr) and to the
// app's log callback so they show up in the in-app log viewer.

func bridgeLog(format string, args ...any) {
	msg := fmt.Sprintf("[TUN-BRIDGE] "+format, args...)
	log.Print(msg)
	deliverLogEntry(LogEntry{
		Level:     LogLevelInfo,
		Timestamp: time.Now().Format("2006-01-02 15:04:05"),
		Message:   msg,
	})
}

func bridgeErr(format string, args ...any) {
	msg := fmt.Sprintf("[TUN-BRIDGE] "+format, args...)
	log.Print(msg)
	deliverLogEntry(LogEntry{
		Level:     LogLevelError,
		Timestamp: time.Now().Format("2006-01-02 15:04:05"),
		Message:   msg,
	})
}

// runTunBridge creates a gVisor TCP/IP stack over the given TUN file descriptor
// and proxies every TCP/UDP connection through the SOCKS5 server at socksAddr.
// This function blocks until ctx is cancelled.
func runTunBridge(ctx context.Context, tunFd int, mtu int, socksAddr string) error {
	tunBytesUp.Store(0)
	tunBytesDown.Store(0)

	bridgeLog("starting bridge: tunFd=%d mtu=%d socksAddr=%s", tunFd, mtu, socksAddr)

	// Install protected UDP dial/listen hooks so every outbound UDP socket the
	// engine opens to DNS resolvers is excluded from the TUN route via
	// VpnService.protect(fd).  Without this the engine's resolver traffic loops
	// back through gVisor -> SOCKS5 -> engine -> infinite loop.
	origDial := client.DialUDPFunc
	origListen := client.ListenUDPFunc
	client.DialUDPFunc = protectedDialUDP
	client.ListenUDPFunc = protectedListenUDP
	defer func() {
		client.DialUDPFunc = origDial
		client.ListenUDPFunc = origListen
	}()

	s := stack.New(stack.Options{
		NetworkProtocols: []stack.NetworkProtocolFactory{
			ipv4.NewProtocol,
			ipv6.NewProtocol,
		},
		TransportProtocols: []stack.TransportProtocolFactory{
			tcp.NewProtocol,
			udp.NewProtocol,
		},
	})

	// Disable Nagle's algorithm — flush writes immediately for low-latency
	// streaming and interactive apps instead of waiting to coalesce segments.
	delay := tcpip.TCPDelayEnabled(false)
	s.SetTransportProtocolOption(tcp.ProtocolNumber, &delay)

	// Enable Selective Acknowledgement (SACK) for faster loss recovery on
	// lossy mobile connections.
	sack := tcpip.TCPSACKEnabled(true)
	s.SetTransportProtocolOption(tcp.ProtocolNumber, &sack)

	// Auto-tune receive buffer — gVisor will grow the receive window based on
	// observed bandwidth, mirroring Linux's tcp_moderate_rcvbuf behaviour.
	moderate := tcpip.TCPModerateReceiveBufferOption(true)
	s.SetTransportProtocolOption(tcp.ProtocolNumber, &moderate)

	// Larger send buffer: 256 KB default, 8 MB ceiling.
	sendBuf := tcpip.TCPSendBufferSizeRangeOption{Min: 4096, Default: 262144, Max: 8388608}
	s.SetTransportProtocolOption(tcp.ProtocolNumber, &sendBuf)

	// Matching receive buffer range.
	recvBuf := tcpip.TCPReceiveBufferSizeRangeOption{Min: 4096, Default: 262144, Max: 8388608}
	s.SetTransportProtocolOption(tcp.ProtocolNumber, &recvBuf)

	ep, err := fdbased.New(&fdbased.Options{
		FDs:            []int{tunFd},
		MTU:            uint32(mtu),
		EthernetHeader: false, // TUN (raw IP), not TAP
		GSOMaxSize:       65535,
		GVisorGSOEnabled: true,
		GRO:              true,
		RXChecksumOffload: true,
	})
	if err != nil {
		return fmt.Errorf("tun_bridge: fdbased.New: %w", err)
	}

	if tcpErr := s.CreateNIC(tunNICID, ep); tcpErr != nil {
		return fmt.Errorf("tun_bridge: CreateNIC: %v", tcpErr)
	}

	// Promiscuous + spoofing: accept packets for any destination IP.
	if tcpErr := s.SetPromiscuousMode(tunNICID, true); tcpErr != nil {
		return fmt.Errorf("tun_bridge: SetPromiscuousMode: %v", tcpErr)
	}
	if tcpErr := s.SetSpoofing(tunNICID, true); tcpErr != nil {
		return fmt.Errorf("tun_bridge: SetSpoofing: %v", tcpErr)
	}

	s.SetRouteTable([]tcpip.Route{
		{Destination: header.IPv4EmptySubnet, NIC: tunNICID},
		{Destination: header.IPv6EmptySubnet, NIC: tunNICID},
	})

	bridgeLog("gVisor stack created, setting up forwarders")

	// ── TCP forwarder ──────────────────────────────────────────────────────────
	// Simplified: no connection pool. Each TCP flow dials the loopback SOCKS5
	// proxy directly. Loopback dials are sub-millisecond so pooling adds
	// complexity (race conditions, stale connections) without measurable gain.
	sem := make(chan struct{}, tcpMaxInFlight)

	tcpFwd := tcp.NewForwarder(s, 0, tcpMaxInFlight, func(r *tcp.ForwarderRequest) {
		id := r.ID()
		dstAddr := net.JoinHostPort(id.LocalAddress.String(), fmt.Sprintf("%d", id.LocalPort))

		var wq waiter.Queue
		ep, tcpErr := r.CreateEndpoint(&wq)
		if tcpErr != nil {
			bridgeErr("TCP CreateEndpoint failed for %s: %v", dstAddr, tcpErr)
			r.Complete(true) // send RST
			return
		}
		r.Complete(false)
		conn := gonet.NewTCPConn(&wq, ep)

		// Backpressure: wait up to 2s for a slot. Log if dropping.
		select {
		case sem <- struct{}{}:
		case <-time.After(2 * time.Second):
			bridgeErr("TCP backpressure: dropping %s (>%d concurrent)", dstAddr, tcpMaxInFlight)
			_ = conn.Close()
			return
		case <-ctx.Done():
			_ = conn.Close()
			return
		}

		go func() {
			defer func() { <-sem }()
			defer conn.Close()
			if ctx.Err() != nil {
				return
			}
			proxyTCPDirect(ctx, conn, dstAddr, socksAddr)
		}()
	})
	s.SetTransportProtocolHandler(tcp.ProtocolNumber, tcpFwd.HandlePacket)

	// ── UDP forwarder ──────────────────────────────────────────────────────────
	// DNS (port 53) is handled by calling the engine's ProcessDNSQuery directly
	// (in-process, no SOCKS5 overhead). All other UDP returns false → ICMP
	// Port Unreachable → browsers fall back to TCP immediately.
	udpFwd := udp.NewForwarder(s, func(r *udp.ForwarderRequest) bool {
		id := r.ID()
		dstPort := id.LocalPort

		if dstPort != 53 {
			return false
		}

		dstAddr := net.JoinHostPort(id.LocalAddress.String(), fmt.Sprintf("%d", dstPort))

		var wq waiter.Queue
		ep, udpErr := r.CreateEndpoint(&wq)
		if udpErr != nil {
			bridgeErr("UDP CreateEndpoint failed for %s: %v", dstAddr, udpErr)
			return false
		}

		conn := gonet.NewUDPConn(&wq, ep)
		go func() {
			defer conn.Close()
			if ctx.Err() != nil {
				return
			}
			proxyDNSDirect(ctx, conn, dstAddr)
		}()
		return true
	})
	s.SetTransportProtocolHandler(udp.ProtocolNumber, udpFwd.HandlePacket)

	bridgeLog("forwarders ready, bridge is active")

	<-ctx.Done()
	bridgeLog("bridge shutting down")
	s.Close()
	return ctx.Err()
}

// ── TCP proxy (direct dial per flow) ───────────────────────────────────────────

// proxyTCPDirect dials the SOCKS5 proxy, performs CONNECT handshake, then
// relays bidirectionally. Each flow gets a fresh connection — loopback is fast.
func proxyTCPDirect(ctx context.Context, src net.Conn, dstAddr string, socksAddr string) {
	host, portStr, err := net.SplitHostPort(dstAddr)
	if err != nil {
		bridgeErr("TCP parse %s: %v", dstAddr, err)
		return
	}
	port, err := parsePort(portStr)
	if err != nil {
		bridgeErr("TCP port %s: %v", dstAddr, err)
		return
	}

	dialCtx, dialCancel := context.WithTimeout(ctx, tcpDialTimeout)
	defer dialCancel()
	// Use plain dial for loopback — VpnService.protect() is NOT needed for
	// 127.0.0.1 (loopback traffic never enters the TUN interface) and calling
	// protect on loopback sockets can cause routing issues on some devices.
	d := net.Dialer{}
	proxy, err := d.DialContext(dialCtx, "tcp", socksAddr)
	if err != nil {
		bridgeErr("TCP dial SOCKS5 for %s: %v", dstAddr, err)
		return
	}
	defer proxy.Close()

	if err := socks5ConnectTCP(proxy, host, port); err != nil {
		bridgeErr("TCP SOCKS5 CONNECT %s: %v", dstAddr, err)
		return
	}

	bridgeLog("TCP connected: %s", dstAddr)
	bidirectionalRelay(src, proxy)
}

// ── DNS proxy (direct in-process call) ─────────────────────────────────────

// proxyDNSDirect reads DNS queries from the gVisor UDP endpoint and processes
// them directly via the engine's ProcessDNSQuery (in-process call, no SOCKS5).
//
// On cache hit the response is returned immediately. On cache miss the query
// is dispatched to the DNS tunnel and the function returns — the browser's
// resolver will retry after its timeout and the second attempt hits cache.
func proxyDNSDirect(ctx context.Context, src net.Conn, dstAddr string) {
	cl := getAnyClient()
	if cl == nil {
		bridgeErr("DNS no engine client available for %s", dstAddr)
		return
	}

	buf := make([]byte, 4096)
	for {
		if ctx.Err() != nil {
			return
		}
		// Set a read deadline so we detect context cancellation periodically.
		if tc, ok := src.(interface{ SetReadDeadline(time.Time) error }); ok {
			_ = tc.SetReadDeadline(time.Now().Add(udpReadTimeout))
		}

		n, readErr := src.Read(buf)
		if readErr != nil {
			if netErr, ok := readErr.(net.Error); ok && netErr.Timeout() {
				continue
			}
			return
		}
		if n == 0 {
			continue
		}

		query := make([]byte, n)
		copy(query, buf[:n])

		isHit := cl.ProcessDNSQuery(query, nil, func(resp []byte) {
			_, _ = src.Write(resp)
			tunBytesDown.Add(int64(len(resp)))
		})

		tunBytesUp.Add(int64(n))

		if isHit {
			bridgeLog("DNS cache hit for %s", dstAddr)
		} else {
			bridgeLog("DNS dispatched to tunnel for %s", dstAddr)
		}
	}
}

// ── SOCKS5 helpers ─────────────────────────────────────────────────────────────

// socks5ConnectTCP performs SOCKS5 method negotiation (no-auth) then CONNECT.
func socks5ConnectTCP(conn net.Conn, host string, port uint16) error {
	_ = conn.SetDeadline(time.Now().Add(socks5HandshakeTimeout))
	defer func() { _ = conn.SetDeadline(time.Time{}) }()

	// Method negotiation — request no-auth (0x00)
	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		return fmt.Errorf("auth write: %w", err)
	}
	authResp := make([]byte, 2)
	if _, err := io.ReadFull(conn, authResp); err != nil {
		return fmt.Errorf("auth read: %w", err)
	}
	if authResp[0] != 0x05 || authResp[1] != 0x00 {
		return fmt.Errorf("unexpected auth response %v", authResp)
	}

	// CONNECT request
	req := buildSocks5ConnectRequest(host, port)
	if _, err := conn.Write(req); err != nil {
		return fmt.Errorf("connect write: %w", err)
	}

	// Response header: VER REP RSV ATYP
	hdr := make([]byte, 4)
	if _, err := io.ReadFull(conn, hdr); err != nil {
		return fmt.Errorf("connect read: %w", err)
	}
	if hdr[1] != 0x00 {
		return fmt.Errorf("CONNECT refused, code=%d", hdr[1])
	}
	// Drain bound address
	if err := drainSocks5Addr(conn, hdr[3]); err != nil {
		return fmt.Errorf("drain addr: %w", err)
	}
	return nil
}

// buildSocks5ConnectRequest builds a SOCKS5 CONNECT request for host:port.
func buildSocks5ConnectRequest(host string, port uint16) []byte {
	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			req := make([]byte, 10)
			req[0], req[1], req[2], req[3] = 0x05, 0x01, 0x00, 0x01
			copy(req[4:8], ip4)
			binary.BigEndian.PutUint16(req[8:10], port)
			return req
		}
		ip6 := ip.To16()
		req := make([]byte, 22)
		req[0], req[1], req[2], req[3] = 0x05, 0x01, 0x00, 0x04
		copy(req[4:20], ip6)
		binary.BigEndian.PutUint16(req[20:22], port)
		return req
	}
	// Domain name
	h := []byte(host)
	req := make([]byte, 7+len(h))
	req[0], req[1], req[2], req[3] = 0x05, 0x01, 0x00, 0x03
	req[4] = byte(len(h))
	copy(req[5:], h)
	binary.BigEndian.PutUint16(req[5+len(h):], port)
	return req
}

// drainSocks5Addr reads and discards the bound address from a SOCKS5 reply.
func drainSocks5Addr(conn net.Conn, atyp byte) error {
	var size int
	switch atyp {
	case 0x01:
		size = 4 + 2
	case 0x03:
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			return err
		}
		size = int(lenBuf[0]) + 2
	case 0x04:
		size = 16 + 2
	default:
		return fmt.Errorf("socks5: unknown atyp=%d", atyp)
	}
	_, err := io.ReadFull(conn, make([]byte, size))
	return err
}

// ── Relay ──────────────────────────────────────────────────────────────────────

// halfCloser is implemented by connections that support TCP half-close.
// Both *net.TCPConn and *gonet.TCPConn implement this interface.
type halfCloser interface {
	CloseWrite() error
}

// bidirectionalRelay copies data between a (TUN/gVisor side) and b (SOCKS5
// proxy side) until either connection closes.
//
//	a -> b  =  upload   (device -> internet)
//	b -> a  =  download (internet -> device)
//
// Uses pooled 64 KB buffers to avoid per-goroutine allocations.
// Sends TCP FIN in each direction independently (half-close) so that
// HTTP/1.1 request-response patterns complete cleanly.
func bidirectionalRelay(a, b net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	copyHalf := func(dst, src net.Conn, counter *atomic.Int64) {
		defer wg.Done()
		buf := getRelayBuf()
		defer putRelayBuf(buf)

		for {
			n, readErr := src.Read(buf)
			if n > 0 {
				wrote, writeErr := dst.Write(buf[:n])
				if wrote > 0 {
					counter.Add(int64(wrote))
				}
				if writeErr != nil {
					break
				}
			}
			if readErr != nil {
				break
			}
		}
		if hc, ok := dst.(halfCloser); ok {
			_ = hc.CloseWrite()
		} else {
			_ = dst.Close()
		}
	}

	go copyHalf(b, a, &tunBytesUp)  // upload:   TUN -> proxy
	copyHalf(a, b, &tunBytesDown)   // download: proxy -> TUN
	wg.Wait()
}

// ── Utilities ──────────────────────────────────────────────────────────────────

func parsePort(portStr string) (uint16, error) {
	p, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return 0, fmt.Errorf("tun_bridge: invalid port %q: %w", portStr, err)
	}
	return uint16(p), nil
}
