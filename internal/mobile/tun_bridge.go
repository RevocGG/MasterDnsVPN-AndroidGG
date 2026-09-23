//go:build linux || android

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
	"errors"
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
	// relayBufSize — each recycled relay buffer. 16 KB keeps per-flow memory
	// low (report A4): at the old 64 KB a DNS tunnel's tiny bandwidth meant
	// megabytes accumulated invisibly inside relay + gVisor buffers, making
	// uploads LOOK complete while they were actually stalled (bufferbloat).
	relayBufSize = 16 * 1024

	// tcpMaxInFlight — maximum concurrent TCP connections being proxied.
	// Browsers/chat apps easily open dozens of flows; each flow holds relay
	// buffers plus a full ARQ stream on the DNS tunnel. 4096 allowed SYN
	// storms and ~512 MB of worst-case relay buffers on weak phones.
	tcpMaxInFlight = 96

	// tcpDialTimeout — timeout for dialling the local SOCKS5 proxy (loopback,
	// sub-millisecond when the engine is healthy).
	tcpDialTimeout = 10 * time.Second

	// relayWriteStallTimeout — max time a single relay Write may block without
	// progress before the whole flow is torn down. Without this a wedged peer
	// left goroutines (and orphaned ARQ streams retransmitting to the server)
	// alive until the profile was stopped (report A1).
	relayWriteStallTimeout = 90 * time.Second

	// relayIdleTimeout — close a flow after this much total silence in BOTH
	// directions. Safety net for fully-stalled flows that produce no errors.
	relayIdleTimeout = 10 * time.Minute

	// relayIdleCheckInterval — how often the per-flow idle watchdog ticks.
	relayIdleCheckInterval = 15 * time.Second

	// socks5HandshakeTimeout lives in socks5_client.go (portable, shared
	// with the unit tests that exercise the handshake on any platform).

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

	// Keep receive buffering static (report A3): auto-tuning lets the window
	// balloon far beyond what a DNS tunnel can drain, hiding stalls from the
	// uploading app (bufferbloat) and delaying error propagation on cancel.
	moderate := tcpip.TCPModerateReceiveBufferOption(false)
	s.SetTransportProtocolOption(tcp.ProtocolNumber, &moderate)

	// Sized for the tunnel's real throughput (~tens of KB/s): 64 KB default,
	// 256 KB ceiling. Small windows give the app honest backpressure so a
	// stalled upload surfaces quickly instead of piling into local buffers.
	sendBuf := tcpip.TCPSendBufferSizeRangeOption{Min: 4096, Default: 64 << 10, Max: 256 << 10}
	s.SetTransportProtocolOption(tcp.ProtocolNumber, &sendBuf)

	// Matching receive buffer range.
	recvBuf := tcpip.TCPReceiveBufferSizeRangeOption{Min: 4096, Default: 64 << 10, Max: 256 << 10}
	s.SetTransportProtocolOption(tcp.ProtocolNumber, &recvBuf)

	ep, err := fdbased.New(&fdbased.Options{
		FDs:            []int{tunFd},
		MTU:            uint32(mtu),
		EthernetHeader: false, // TUN (raw IP), not TAP
		// GSOMaxSize, GVisorGSOEnabled and GRO are intentionally NOT set.
		// VpnService on Android does NOT support IFF_VNET_HDR, so gVisor GSO
		// superframes are silently dropped by the kernel — causing near-zero
		// throughput.  RXChecksumOffload is safe because gVisor validates
		// checksums internally before passing to the stack regardless.
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

		// DNS-over-TCP (53) and DoT (853) cannot be served by this bridge: the
		// engine resolves DNS in-process over UDP only, so a CONNECT for the
		// virtual DNS IP (or any :53/:853 TCP flow) would travel to the tunnel
		// server, time out there, and stall app startup for seconds (report C1).
		// RST immediately so clients fall back to UDP/53 (handled in-process).
		if id.LocalPort == 853 || id.LocalPort == 53 {
			r.Complete(true)
			return
		}

		// "Disable IPv6" (default ON): RST IPv6 destinations on the spot. Apps
		// with cached AAAA answers (e.g. YouTube/Cronet) otherwise send every
		// IPv6 CONNECT through the SOCKS/tunnel and only learn of the failure
		// after a full tunnel round-trip with "connect refused code 3". An
		// immediate RST makes them fall back to the IPv4 address right away.
		if id.LocalAddress.Len() == 16 && ipv6Blocked() {
			r.Complete(true)
			return
		}

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
		// Use NewTimer (not time.After) so the timer is stopped immediately
		// when a slot is available or ctx is cancelled — time.After leaves
		// the underlying timer live for the full 2s on every fast-path
		// connection, accumulating thousands of leaked timers under load.
		bpTimer := time.NewTimer(2 * time.Second)
		select {
		case sem <- struct{}{}:
			bpTimer.Stop()
		case <-bpTimer.C:
			bridgeErr("TCP backpressure: dropping %s (>%d concurrent)", dstAddr, tcpMaxInFlight)
			_ = conn.Close()
			return
		case <-ctx.Done():
			bpTimer.Stop()
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

	// Apply the local proxy's credentials (no-op when auth is disabled).
	proxy = socks5CredentialsDialer(proxy)

	if err := socks5ConnectTCP(proxy, host, port); err != nil {
		bridgeErr("TCP SOCKS5 CONNECT %s: %v", dstAddr, err)
		return
	}

	bridgeLog("TCP connected: %s", dstAddr)
	bidirectionalRelay(ctx, src, proxy)
}

// ── DNS proxy (direct in-process call) ─────────────────────────────────────

// proxyDNSDirect reads DNS queries from the gVisor UDP endpoint and processes
// them directly via the engine's ProcessDNSQuery (in-process call, no SOCKS5).
//
// On cache hit the response is returned immediately. On cache miss the query
// is dispatched to the DNS tunnel and the function returns — the browser's
// resolver will retry after its timeout and the second attempt hits cache.
func proxyDNSDirect(ctx context.Context, src net.Conn, dstAddr string) {
	buf := make([]byte, 4096)
	var idleTimeouts int
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
				idleTimeouts++
				if idleTimeouts >= 3 { // 3×5s = 15s idle — exit goroutine
					return
				}
				continue
			}
			return
		}
		idleTimeouts = 0
		if n == 0 {
			continue
		}

		query := make([]byte, n)
		copy(query, buf[:n])

		// Resolve the engine client on every packet so we handle the case
		// where the instance starts up after this goroutine is already running.
		// Calling getAnyClient() once at goroutine start (outside the loop)
		// means a nil return permanently closes the UDP session — all
		// subsequent DNS queries from the same browser socket get no response.
		cl := getAnyClient()
		if cl == nil {
			// Engine not ready yet — drop this packet, browser will retry.
			tunBytesUp.Add(int64(n))
			continue
		}

		// Drop queries when the tunnel session is not yet established.
		// ProcessDNSQuery would dispatch to the tunnel only when SessionReady=true.
		// If we call it while SessionReady=false, it creates a StatusPending cache
		// entry and then silently skips dispatch.  The cache entry then blocks any
		// re-dispatch for the full pendingTimeout window (default 300 s), meaning
		// the poll loop below (30 s) can NEVER succeed — DNS always times out.
		// DNS clients retry every 1–2 s automatically; by the second or third
		// retry the session is ready and the query dispatches normally.
		if !cl.SessionReady() {
			tunBytesUp.Add(int64(n))
			bridgeLog("DNS drop (session not ready) for %s — client will retry", dstAddr)
			continue
		}

		// shared write-once flag + callback — no mutex needed: everything runs
		// in this single goroutine (ProcessDNSQuery calls the callback synchronously).
		var responseWritten bool
		writeResp := func(resp []byte) {
			// Strip AAAA records: the tunnel has no IPv6 egress, so advertising
			// IPv6 addresses makes apps try (and get RST on) dead connections.
			resp = filterDNSAAAA(resp)
			_, _ = src.Write(resp)
			tunBytesDown.Add(int64(len(resp)))
			responseWritten = true
		}

		isHit := cl.ProcessDNSQuery(query, nil, writeResp)
		tunBytesUp.Add(int64(n))

		if isHit {
			bridgeLog("DNS cache hit for %s", dstAddr)
		} else {
			// Cache miss: query dispatched to the tunnel (fire-and-forget).
			// Poll ProcessDNSQuery every 20 ms until HandleDNSQueryRes fills the
			// cache entry (StatusReady), at which point the call hits the cache and
			// invokes writeResp.  Calls while StatusPending do NOT re-dispatch
			// (guarded by pendingTimeout in the cache), so each tick is just a cheap
			// read-locked cache lookup.
			// Use a Ticker (created once) instead of time.After (creates a new
			// timer object on every iteration — ~250 per query miss at 5s/20ms).
			deadline := time.Now().Add(5 * time.Second)
			pollTicker := time.NewTicker(20 * time.Millisecond)
		pollLoop:
			for !responseWritten {
				select {
				case <-ctx.Done():
					break pollLoop
				case <-pollTicker.C:
					if time.Now().After(deadline) {
						break pollLoop
					}
					// Re-fetch client on each tick in case the instance restarted.
					// Also guard against a client that lost its session mid-poll.
					if cl = getAnyClient(); cl != nil && cl.SessionReady() {
						_ = cl.ProcessDNSQuery(query, nil, writeResp)
					}
				}
			}
			pollTicker.Stop()
			if responseWritten {
				bridgeLog("DNS tunnel response for %s", dstAddr)
			} else {
				bridgeLog("DNS timeout (no tunnel response) for %s", dstAddr)
			}
		}
	}
}

// ── SOCKS5 dialer wiring and the DNS AAAA filter live in tun_state.go
// (portable, unit-tested on every platform) ───────

// ── Relay ──────────────────────────────────────────────────────────────────────

// halfCloser is implemented by connections that support TCP half-close.
// Both *net.TCPConn and *gonet.TCPConn implement this interface.
type halfCloser interface {
	CloseWrite() error
}

// bidirectionalRelay copies data between a (TUN/gVisor side) and b (SOCKS5
// proxy side) until the flow finishes or is aborted.
//
//	a -> b  =  upload   (device -> internet)
//	b -> a  =  download (internet -> device)
//
// Uses pooled 16 KB buffers to bound per-flow memory.
//
// Abort semantics (report A1 — previously a cancelled/stalled upload left the
// SOCKS connection and its ARQ stream retransmitting to the server until the
// whole profile was stopped):
//   - ctx cancelled (bridge bounce / VPN stop) → both sides closed at once.
//   - Write stalled longer than relayWriteStallTimeout → both sides closed.
//   - non-EOF read error (RST / reset / closed) → BOTH sides closed, so an
//     app-side abort propagates to the tunnel instead of half-closing and
//     waiting for the server to close its half forever.
//   - clean EOF → graceful half-close of the peer's write side only, keeping
//     correct HTTP/1.1 request-response completion.
//   - total silence in both directions for relayIdleTimeout → both closed
//     (watchdog for flows that stall without producing any error).
func bidirectionalRelay(ctx context.Context, a, b net.Conn) {
	var lastActivity atomic.Int64
	lastActivity.Store(time.Now().UnixNano())

	closeBoth := func() {
		_ = a.Close()
		_ = b.Close()
	}

	// Bridge teardown (bounce/stop) must tear down every live relay so no
	// orphaned SOCKS streams survive the bridge.
	stopWatch := context.AfterFunc(ctx, closeBoth)
	defer stopWatch()

	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(relayIdleCheckInterval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if time.Since(time.Unix(0, lastActivity.Load())) > relayIdleTimeout {
					closeBoth()
					return
				}
			}
		}
	}()

	var wg sync.WaitGroup
	wg.Add(2)
	copyHalf := func(dst, src net.Conn, counter *atomic.Int64) {
		defer wg.Done()
		buf := getRelayBuf()
		defer putRelayBuf(buf)

		for {
			n, readErr := src.Read(buf)
			if n > 0 {
				_ = dst.SetWriteDeadline(time.Now().Add(relayWriteStallTimeout))
				wrote, writeErr := dst.Write(buf[:n])
				_ = dst.SetWriteDeadline(time.Time{})
				if wrote > 0 {
					counter.Add(int64(wrote))
				}
				lastActivity.Store(time.Now().UnixNano())
				if writeErr != nil {
					// Stalled write or peer reset: abort the whole flow.
					closeBoth()
					return
				}
			}
			if readErr != nil {
				if errors.Is(readErr, io.EOF) {
					// Clean half-close: this direction finished normally.
					if hc, ok := dst.(halfCloser); ok {
						_ = hc.CloseWrite()
					} else {
						_ = dst.Close()
					}
					return
				}
				// Reset/timeout/closed: tear down BOTH directions now. The old
				// behaviour only stopped this half, leaving the SOCKS side (and
				// the server's stream) alive — the stuck-upload bug.
				closeBoth()
				return
			}
		}
	}

	go copyHalf(b, a, &tunBytesUp)   // upload:   TUN -> proxy
	go copyHalf(a, b, &tunBytesDown) // download: proxy -> TUN
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
