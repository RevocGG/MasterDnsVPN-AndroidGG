// ==============================================================================
// MasterDnsVPN
// Author: MasterkinG32
// Github: https://github.com/masterking32
// Year: 2026
// ==============================================================================
// Package mobile — socks_balancer.go
//
// A lightweight SOCKS5 load-balancer that accepts connections on a single
// listen port and distributes them across multiple upstream SOCKS5 proxies
// using configurable strategies: round-robin, random, least-connections,
// or failover.
// ==============================================================================
package mobile

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// ── Buffer pool for relay copies ───────────────────────────────────────────────

const balancerRelayBufSize = 64 * 1024

var balancerBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, balancerRelayBufSize)
		return &b
	},
}

func getBalancerBuf() []byte  { return *(balancerBufPool.Get().(*[]byte)) }
func putBalancerBuf(b []byte) { balancerBufPool.Put(&b) }

// BalancerStrategy mirrors internal/client/balancer.go constants.
const (
	BalancerDefault       = 0 // round-robin
	BalancerRandom        = 1
	BalancerRoundRobin    = 2
	BalancerLeastConn     = 3
	BalancerLowestLatency = 4 // use least-conn as approximation
)

type upstreamEntry struct {
	addr       string
	activeConn int64
}

type socksBalancer struct {
	mu        sync.RWMutex
	upstreams []*upstreamEntry
	strategy  int
	rrIndex   uint64
	listener  net.Listener
	cancel    context.CancelFunc
	done      chan struct{}
}

var (
	balancerMu       sync.Mutex
	activeBalancer   *socksBalancer
)

// StartSocksBalancer starts a SOCKS5 load-balancer listening on listenAddr
// that distributes connections across the given upstream addresses.
//
//   - listenAddr:  "host:port" to listen on, e.g. "127.0.0.1:10800"
//   - upstreamCSV: comma-separated upstream "host:port" addresses
//   - strategy:    one of BalancerDefault..BalancerLowestLatency
//
// Returns the actual listen address (useful when port is 0).
func StartSocksBalancer(listenAddr string, upstreamAddrs []string, strategy int) (string, error) {
	balancerMu.Lock()
	defer balancerMu.Unlock()

	if activeBalancer != nil {
		return "", errors.New("socks balancer already running")
	}

	if len(upstreamAddrs) == 0 {
		return "", errors.New("no upstream addresses")
	}

	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return "", fmt.Errorf("socks balancer listen: %w", err)
	}

	entries := make([]*upstreamEntry, len(upstreamAddrs))
	for i, a := range upstreamAddrs {
		entries[i] = &upstreamEntry{addr: a}
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	b := &socksBalancer{
		upstreams: entries,
		strategy:  strategy,
		listener:  ln,
		cancel:    cancel,
		done:      done,
	}
	activeBalancer = b

	go func() {
		defer close(done)
		b.acceptLoop(ctx)
	}()

	return ln.Addr().String(), nil
}

// StopSocksBalancer stops the load-balancer if running.
func StopSocksBalancer() {
	balancerMu.Lock()
	b := activeBalancer
	activeBalancer = nil
	balancerMu.Unlock()

	if b == nil {
		return
	}
	b.cancel()
	b.listener.Close()
	<-b.done
}

// IsSocksBalancerRunning returns true if the balancer is active.
func IsSocksBalancerRunning() bool {
	balancerMu.Lock()
	defer balancerMu.Unlock()
	return activeBalancer != nil
}

// UpdateBalancerUpstreams hot-swaps the upstream list (e.g. when a profile
// goes down and needs to be removed).
func UpdateBalancerUpstreams(addrs []string) {
	balancerMu.Lock()
	b := activeBalancer
	balancerMu.Unlock()
	if b == nil {
		return
	}
	entries := make([]*upstreamEntry, len(addrs))
	for i, a := range addrs {
		entries[i] = &upstreamEntry{addr: a}
	}
	b.mu.Lock()
	b.upstreams = entries
	b.mu.Unlock()
}

func (b *socksBalancer) acceptLoop(ctx context.Context) {
	for {
		conn, err := b.listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			continue
		}
		go b.handleConn(ctx, conn)
	}
}

func (b *socksBalancer) handleConn(ctx context.Context, client net.Conn) {
	defer client.Close()

	// Read the SOCKS5 handshake from the client, then replay it to the
	// chosen upstream. This way the upstream Go SOCKS5 server sees a fresh
	// SOCKS5 connection.

	// 1. Read client greeting
	greeting := make([]byte, 258)
	client.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, err := client.Read(greeting)
	if err != nil || n < 3 || greeting[0] != 0x05 {
		return
	}
	greetingData := greeting[:n]

	// Pick upstream
	upstream := b.pickUpstream()
	if upstream == nil {
		return
	}
	atomic.AddInt64(&upstream.activeConn, 1)
	defer atomic.AddInt64(&upstream.activeConn, -1)

	// Connect to upstream
	upConn, err := net.DialTimeout("tcp", upstream.addr, 5*time.Second)
	if err != nil {
		// Try next upstream on failure
		upstream2 := b.pickFallback(upstream.addr)
		if upstream2 == nil {
			return
		}
		atomic.AddInt64(&upstream2.activeConn, 1)
		defer atomic.AddInt64(&upstream2.activeConn, -1)
		upConn, err = net.DialTimeout("tcp", upstream2.addr, 5*time.Second)
		if err != nil {
			return
		}
	}
	defer upConn.Close()

	// Replay greeting to upstream
	if _, err := upConn.Write(greetingData); err != nil {
		return
	}

	// From here, just relay everything bidirectionally.
	// The SOCKS5 auth + CONNECT negotiation happens between client and
	// upstream transparently.
	client.SetReadDeadline(time.Time{})
	relay(ctx, client, upConn)
}

func (b *socksBalancer) pickUpstream() *upstreamEntry {
	b.mu.RLock()
	ups := b.upstreams
	b.mu.RUnlock()

	if len(ups) == 0 {
		return nil
	}
	if len(ups) == 1 {
		return ups[0]
	}

	switch b.strategy {
	case BalancerRandom, BalancerDefault:
		return ups[rand.Intn(len(ups))]

	case BalancerRoundRobin:
		idx := atomic.AddUint64(&b.rrIndex, 1)
		return ups[idx%uint64(len(ups))]

	case BalancerLeastConn, BalancerLowestLatency:
		best := ups[0]
		bestConn := atomic.LoadInt64(&best.activeConn)
		for _, u := range ups[1:] {
			c := atomic.LoadInt64(&u.activeConn)
			if c < bestConn {
				best = u
				bestConn = c
			}
		}
		return best

	default:
		return ups[rand.Intn(len(ups))]
	}
}

func (b *socksBalancer) pickFallback(exclude string) *upstreamEntry {
	b.mu.RLock()
	ups := b.upstreams
	b.mu.RUnlock()

	for _, u := range ups {
		if u.addr != exclude {
			return u
		}
	}
	return nil
}

func relay(ctx context.Context, a, b net.Conn) {
	buf1 := getBalancerBuf()
	buf2 := getBalancerBuf()

	var wg sync.WaitGroup
	wg.Add(2)

	cp := func(dst, src net.Conn, buf []byte) {
		defer wg.Done()
		defer putBalancerBuf(buf)
		io.CopyBuffer(dst, src, buf)
		// Half-close so the other side sees EOF cleanly.
		if tc, ok := dst.(*net.TCPConn); ok {
			tc.CloseWrite()
		} else {
			dst.Close()
		}
	}

	go cp(a, b, buf1) // upstream → client (download)
	go cp(b, a, buf2) // client → upstream (upload)

	// Wait for BOTH directions — only then close connections (via defer in
	// handleConn). Closing early after one direction truncates in-flight data.
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()

	select {
	case <-done:
	case <-ctx.Done():
		a.Close()
		b.Close()
		<-done
	}
}
