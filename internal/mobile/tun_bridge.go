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
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

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
	// relayBufSize — each recycled relay buffer is 32 KB, matching typical
	// CDN chunk sizes and HTTP/2 DATA frame sizes.
	relayBufSize = 32 * 1024

	// tcpMaxInFlight — maximum concurrent half-open TCP connections tracked
	// by the gVisor TCP forwarder.
	tcpMaxInFlight = 1024

	// tcpBackpressureTimeout — how long the TCP forwarder waits for a free
	// concurrency slot before refusing a new connection.
	tcpBackpressureTimeout = 200 * time.Millisecond

	// tcpPoolMaxIdle — maximum idle pre-dialled TCP connections in pool.
	tcpPoolMaxIdle = 32

	// tcpPoolWarmSize — connections pre-dialled at bridge startup.
	tcpPoolWarmSize = 6

	// tcpPoolIdleTTL — discard pool connections idle longer than this.
	tcpPoolIdleTTL = 90 * time.Second

	// tcpDialTimeout — timeout for a fresh SOCKS5 TCP dial.
	tcpDialTimeout = 3 * time.Second

	// udpReadTimeout is the per-iteration deadline on the UDP relay socket
	// (allows ctx.Done detection without blocking forever).
	udpReadTimeout = 5 * time.Second

	// udpBufSize — max UDP datagram size.
	udpBufSize = 65535
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

// ── SOCKS5 TCP connection pool ─────────────────────────────────────────────────
//
// Stores raw TCP connections to the SOCKS5 server ready for a CONNECT request.
// Connections are discarded after tcpPoolIdleTTL to avoid using stale sockets.

type pooledTCPConn struct {
	net.Conn
	idleSince time.Time
}

type socks5Pool struct {
	addr string
	mu   sync.Mutex
	idle []*pooledTCPConn
}

func newSocks5Pool(addr string) *socks5Pool {
	return &socks5Pool{addr: addr, idle: make([]*pooledTCPConn, 0, tcpPoolMaxIdle)}
}

// warmUp pre-dials n connections in background goroutines.
func (p *socks5Pool) warmUp(ctx context.Context) {
	for i := 0; i < tcpPoolWarmSize; i++ {
		go func() {
			c, err := net.DialTimeout("tcp", p.addr, tcpDialTimeout)
			if err != nil || ctx.Err() != nil {
				if c != nil {
					_ = c.Close()
				}
				return
			}
			p.put(c)
		}()
	}
}

// get returns a healthy idle connection or dials a new one.
func (p *socks5Pool) get() (net.Conn, error) {
	p.mu.Lock()
	for len(p.idle) > 0 {
		pc := p.idle[len(p.idle)-1]
		p.idle = p.idle[:len(p.idle)-1]
		p.mu.Unlock()
		if time.Since(pc.idleSince) <= tcpPoolIdleTTL {
			return pc.Conn, nil
		}
		_ = pc.Close()
		p.mu.Lock()
	}
	p.mu.Unlock()
	return net.DialTimeout("tcp", p.addr, tcpDialTimeout)
}

// put returns a fresh pre-CONNECT connection to the idle pool.
// Do NOT call put on connections that have already performed a SOCKS5 CONNECT.
func (p *socks5Pool) put(c net.Conn) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.idle) >= tcpPoolMaxIdle {
		_ = c.Close()
		return
	}
	p.idle = append(p.idle, &pooledTCPConn{Conn: c, idleSince: time.Now()})
}

// refillOne pre-dials one connection in the background if the pool is low.
func (p *socks5Pool) refillOne(ctx context.Context) {
	p.mu.Lock()
	sz := len(p.idle)
	p.mu.Unlock()
	if sz >= tcpPoolWarmSize || ctx.Err() != nil {
		return
	}
	go func() {
		c, err := net.DialTimeout("tcp", p.addr, tcpDialTimeout)
		if err != nil || ctx.Err() != nil {
			if c != nil {
				_ = c.Close()
			}
			return
		}
		p.put(c)
	}()
}

func (p *socks5Pool) closeAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, pc := range p.idle {
		_ = pc.Close()
	}
	p.idle = p.idle[:0]
}

// runTunBridge creates a gVisor TCP/IP stack over the given TUN file descriptor
// and proxies every TCP/UDP connection through the SOCKS5 server at socksAddr.
// This function blocks until ctx is cancelled.
func runTunBridge(ctx context.Context, tunFd int, mtu int, socksAddr string) error {
	tunBytesUp.Store(0)
	tunBytesDown.Store(0)

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
	// 256 KB sustains ~512 Kbps for ~4 s without stalling; 8 MB ceiling lets
	// auto-tuning grow further on faster connections.
	sendBuf := tcpip.TCPSendBufferSizeRangeOption{Min: 4096, Default: 262144, Max: 8388608}
	s.SetTransportProtocolOption(tcp.ProtocolNumber, &sendBuf)

	// Matching receive buffer range.
	recvBuf := tcpip.TCPReceiveBufferSizeRangeOption{Min: 4096, Default: 262144, Max: 8388608}
	s.SetTransportProtocolOption(tcp.ProtocolNumber, &recvBuf)

	ep, err := fdbased.New(&fdbased.Options{
		FDs:            []int{tunFd},
		MTU:            uint32(mtu),
		EthernetHeader: false, // TUN (raw IP), not TAP
		// TXChecksumOffload and RXChecksumOffload are intentionally NOT set.
		// TUN is a pure software device with no hardware checksum offload.
		// Setting TXChecksumOffload=true would cause gVisor to skip computing
		// TCP/UDP checksums — resulting in every outgoing packet being dropped
		// by the kernel due to invalid checksums (near-zero throughput).
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

	// ── TCP connection pool ───────────────────────────────────────────────────
	tcpPool := newSocks5Pool(socksAddr)
	defer tcpPool.closeAll()

	// Pre-dial before the first app request arrives.
	tcpPool.warmUp(ctx)

	// ── TCP forwarder ──────────────────────────────────────────────────────────
	sem := make(chan struct{}, tcpMaxInFlight)

	tcpFwd := tcp.NewForwarder(s, 0, tcpMaxInFlight, func(r *tcp.ForwarderRequest) {
		id := r.ID()
		dstAddr := fmt.Sprintf("%s:%d", id.LocalAddress.String(), id.LocalPort)

		var wq waiter.Queue
		ep, tcpErr := r.CreateEndpoint(&wq)
		if tcpErr != nil {
			r.Complete(true)
			return
		}
		r.Complete(false)
		conn := gonet.NewTCPConn(&wq, ep)

		// Backpressure: block briefly instead of silently dropping.
		// Under normal load the channel has a free slot instantly.
		select {
		case sem <- struct{}{}:
		case <-time.After(tcpBackpressureTimeout):
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
			proxyTCPWithPool(ctx, conn, dstAddr, tcpPool)
		}()
	})
	s.SetTransportProtocolHandler(tcp.ProtocolNumber, tcpFwd.HandlePacket)

	// ── UDP forwarder ──────────────────────────────────────────────────────────
	udpFwd := udp.NewForwarder(s, func(r *udp.ForwarderRequest) bool {
		id := r.ID()
		dstAddr := fmt.Sprintf("%s:%d", id.LocalAddress.String(), id.LocalPort)

		var wq waiter.Queue
		ep, udpErr := r.CreateEndpoint(&wq)
		if udpErr != nil {
			return false
		}

		conn := gonet.NewUDPConn(&wq, ep)
		go func() {
			defer conn.Close()
			if ctx.Err() != nil {
				return
			}
			proxyUDPViaSocks5(ctx, conn, dstAddr, socksAddr)
		}()
		return true
	})
	s.SetTransportProtocolHandler(udp.ProtocolNumber, udpFwd.HandlePacket)

	<-ctx.Done()
	s.Close()
	return ctx.Err()
}

// ── TCP proxy via pool ─────────────────────────────────────────────────────────

// proxyTCPWithPool obtains a pre-dialled connection from the pool, performs
// SOCKS5 CONNECT, then relays.  If the pooled connection is stale (rare) it
// retries once with a fresh dial so the app never sees the failure.
func proxyTCPWithPool(ctx context.Context, src net.Conn, dstAddr string, pool *socks5Pool) {
	host, portStr, err := net.SplitHostPort(dstAddr)
	if err != nil {
		return
	}
	port, err := parsePort(portStr)
	if err != nil {
		return
	}

	var proxy net.Conn
	// Up to 2 attempts: pooled first, fresh dial on retry.
	for attempt := 0; attempt < 2; attempt++ {
		proxy, err = pool.get()
		if err != nil {
			return
		}
		if err = socks5ConnectTCP(proxy, host, port); err == nil {
			break
		}
		_ = proxy.Close()
		proxy = nil
	}
	if proxy == nil {
		return
	}
	defer proxy.Close()

	// Trigger a background refill so the next flow finds a ready connection.
	pool.refillOne(ctx)

	bidirectionalRelay(src, proxy)
}

// ── UDP proxy via SOCKS5 UDP ASSOCIATE ────────────────────────────────────────

// proxyUDPViaSocks5 opens a fresh SOCKS5 UDP ASSOCIATE session per UDP flow
// and relays datagrams between the TUN endpoint and the SOCKS5 relay socket.
//
// A fresh session per flow is intentional: the MasterDnsVPN SOCKS5 server only
// handles DNS (port 53) and closes the UDP ASSOCIATE immediately on any DNS
// cache miss.  Pooling sessions would return stale sessions whose server-side
// socket is already closed, silently dropping every DNS query.
func proxyUDPViaSocks5(ctx context.Context, src net.Conn, dstAddr, socksAddr string) {
	host, portStr, err := net.SplitHostPort(dstAddr)
	if err != nil {
		return
	}
	port, err := parsePort(portStr)
	if err != nil {
		return
	}

	ctrlConn, err := net.DialTimeout("tcp", socksAddr, tcpDialTimeout)
	if err != nil {
		return
	}
	defer ctrlConn.Close()

	relayAddrStr, err := socks5UDPAssociate(ctrlConn)
	if err != nil {
		return
	}
	relayAddr, err := net.ResolveUDPAddr("udp", relayAddrStr)
	if err != nil {
		return
	}
	localUDP, err := net.ListenUDP("udp", &net.UDPAddr{})
	if err != nil {
		return
	}
	defer localUDP.Close()

	// TUN → SOCKS5 relay goroutine.
	go func() {
		buf := make([]byte, udpBufSize)
		for {
			n, readErr := src.Read(buf)
			if readErr != nil {
				return
			}
			wrapped := buildSocks5UDPFrame(host, port, buf[:n])
			_, _ = localUDP.WriteToUDP(wrapped, relayAddr)
		}
	}()

	// SOCKS5 → TUN relay (this goroutine).
	buf := make([]byte, udpBufSize)
	for {
		if ctx.Err() != nil {
			return
		}
		_ = localUDP.SetReadDeadline(time.Now().Add(udpReadTimeout))
		n, _, readErr := localUDP.ReadFromUDP(buf)
		if readErr != nil {
			if netErr, ok := readErr.(net.Error); ok && netErr.Timeout() {
				continue
			}
			return
		}
		payload, parseErr := parseSocks5UDPFrame(buf[:n])
		if parseErr != nil {
			continue
		}
		_, _ = src.Write(payload)
	}
}

// ── SOCKS5 helpers ─────────────────────────────────────────────────────────────

// socks5ConnectTCP performs SOCKS5 method negotiation (no-auth) then CONNECT.
func socks5ConnectTCP(conn net.Conn, host string, port uint16) error {
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	defer func() { _ = conn.SetDeadline(time.Time{}) }()

	// Method negotiation — request no-auth (0x00)
	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		return err
	}
	authResp := make([]byte, 2)
	if _, err := io.ReadFull(conn, authResp); err != nil {
		return err
	}
	if authResp[0] != 0x05 || authResp[1] != 0x00 {
		return fmt.Errorf("socks5: unexpected auth response %v", authResp)
	}

	// CONNECT request
	req := buildSocks5ConnectRequest(host, port)
	if _, err := conn.Write(req); err != nil {
		return err
	}

	// Response header: VER REP RSV ATYP
	hdr := make([]byte, 4)
	if _, err := io.ReadFull(conn, hdr); err != nil {
		return err
	}
	if hdr[1] != 0x00 {
		return fmt.Errorf("socks5: CONNECT refused, code=%d", hdr[1])
	}
	// Drain bound address
	if err := drainSocks5Addr(conn, hdr[3]); err != nil {
		return err
	}
	return nil
}

// socks5UDPAssociate sends SOCKS5 UDP ASSOCIATE and returns the relay UDP address.
func socks5UDPAssociate(conn net.Conn) (string, error) {
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	defer func() { _ = conn.SetDeadline(time.Time{}) }()

	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		return "", err
	}
	authResp := make([]byte, 2)
	if _, err := io.ReadFull(conn, authResp); err != nil {
		return "", err
	}
	if authResp[0] != 0x05 || authResp[1] != 0x00 {
		return "", fmt.Errorf("socks5: unexpected auth response %v", authResp)
	}

	// UDP ASSOCIATE with zero bind addr/port
	req := []byte{0x05, 0x03, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	if _, err := conn.Write(req); err != nil {
		return "", err
	}

	hdr := make([]byte, 4)
	if _, err := io.ReadFull(conn, hdr); err != nil {
		return "", err
	}
	if hdr[1] != 0x00 {
		return "", fmt.Errorf("socks5: UDP ASSOCIATE failed, code=%d", hdr[1])
	}

	switch hdr[3] {
	case 0x01: // IPv4
		buf := make([]byte, 6)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return "", err
		}
		return fmt.Sprintf("%s:%d", net.IP(buf[:4]).String(), binary.BigEndian.Uint16(buf[4:6])), nil
	case 0x04: // IPv6
		buf := make([]byte, 18)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return "", err
		}
		return fmt.Sprintf("[%s]:%d", net.IP(buf[:16]).String(), binary.BigEndian.Uint16(buf[16:18])), nil
	default:
		return "", fmt.Errorf("socks5: unexpected atyp=%d in UDP ASSOCIATE reply", hdr[3])
	}
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

// buildSocks5UDPFrame wraps payload in a SOCKS5 UDP header for the given destination.
func buildSocks5UDPFrame(host string, port uint16, payload []byte) []byte {
	var addrPart []byte
	var atyp byte
	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			atyp = 0x01
			addrPart = ip4
		} else {
			atyp = 0x04
			addrPart = ip.To16()
		}
	} else {
		atyp = 0x03
		addrPart = append([]byte{byte(len(host))}, []byte(host)...)
	}
	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, port)

	frame := make([]byte, 0, 4+len(addrPart)+2+len(payload))
	frame = append(frame, 0x00, 0x00, 0x00, atyp) // RSV RSV FRAG ATYP
	frame = append(frame, addrPart...)
	frame = append(frame, portBytes...)
	frame = append(frame, payload...)
	return frame
}

// parseSocks5UDPFrame strips the SOCKS5 UDP header and returns the payload.
func parseSocks5UDPFrame(data []byte) ([]byte, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("tun_bridge: UDP frame too short")
	}
	var skip int
	switch data[3] {
	case 0x01:
		skip = 4 + 4 + 2
	case 0x03:
		if len(data) < 5 {
			return nil, fmt.Errorf("tun_bridge: UDP frame too short for domain")
		}
		skip = 4 + 1 + int(data[4]) + 2
	case 0x04:
		skip = 4 + 16 + 2
	default:
		return nil, fmt.Errorf("tun_bridge: unknown atyp=%d in UDP frame", data[3])
	}
	if len(data) < skip {
		return nil, fmt.Errorf("tun_bridge: UDP frame header truncated")
	}
	return data[skip:], nil
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
//	a → b  =  upload   (device → internet)
//	b → a  =  download (internet → device)
//
// Uses pooled 32 KB buffers to avoid per-goroutine allocations.
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

	go copyHalf(b, a, &tunBytesUp)  // upload:   TUN → proxy
	copyHalf(a, b, &tunBytesDown)   // download: proxy → TUN
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
