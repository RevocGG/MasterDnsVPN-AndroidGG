// ==============================================================================
// MasterDnsVPN
// Author: MasterkinG32
// Github: https://github.com/masterking32
// Year: 2026
// ==============================================================================

package client

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	Enums "masterdnsvpn-go/internal/enums"
)

const (
	pingAggressiveInterval = 300 * time.Millisecond
	pingLazyInterval       = 1 * time.Second
	pingCoolDownInterval   = 3 * time.Second
	pingColdInterval       = 30 * time.Second
	pingWarmThreshold      = 5 * time.Second
	pingCoolThreshold      = 10 * time.Second
	pingColdThreshold      = 20 * time.Second
	pingPongFreshWindow    = 2 * time.Second
)

type PingManager struct {
	client                *Client
	lastPingSentAt        atomic.Int64
	lastPongReceivedAt    atomic.Int64
	lastNonPingSentAt     atomic.Int64
	lastNonPongReceivedAt atomic.Int64

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	wakeCh chan struct{}
}

func newPingManager(client *Client) *PingManager {
	now := time.Now().UnixNano()
	p := &PingManager{
		client: client,
		wakeCh: make(chan struct{}, 1),
	}
	p.lastPingSentAt.Store(now)
	p.lastPongReceivedAt.Store(now)
	p.lastNonPingSentAt.Store(now)
	p.lastNonPongReceivedAt.Store(now)
	return p
}

// Start starts the autonomous ping loop.
func (p *PingManager) Start(parentCtx context.Context) {
	p.Stop() // Ensure old one is stopped

	p.ctx, p.cancel = context.WithCancel(parentCtx)
	p.wg.Add(1)
	go p.pingLoop()
}

// Stop stops the ping loop.
func (p *PingManager) Stop() {
	if p.cancel != nil {
		p.cancel()
		p.wg.Wait()
		p.cancel = nil
	}
}

func (p *PingManager) NotifyPacket(packetType uint8, isInbound bool) {
	if p == nil {
		return
	}
	now := time.Now().UnixNano()
	isPing := packetType == Enums.PACKET_PING
	isPong := packetType == Enums.PACKET_PONG || packetType == Enums.PACKET_STREAM_DATA_ACK || packetType == Enums.PACKET_STREAM_FIN_ACK || packetType == Enums.PACKET_STREAM_RST_ACK

	if isInbound {
		if isPong {
			p.lastPongReceivedAt.Store(now)
		} else {
			p.lastNonPongReceivedAt.Store(now)
			p.wake()
		}
	} else {
		if isPing {
			p.lastPingSentAt.Store(now)
		} else {
			p.lastNonPingSentAt.Store(now)
			p.wake()
		}
	}
}

func (p *PingManager) wake() {
	select {
	case p.wakeCh <- struct{}{}:
	default:
	}
}

func (p *PingManager) nextInterval(now time.Time) time.Duration {
	lastNonPingSent := time.Unix(0, p.lastNonPingSentAt.Load())
	lastNonPongRecv := time.Unix(0, p.lastNonPongReceivedAt.Load())

	// Condition: Aggressive if ANY non-ping/pong activity in last 5 seconds
	if now.Sub(lastNonPingSent) < pingWarmThreshold || now.Sub(lastNonPongRecv) < pingWarmThreshold {
		return pingAggressiveInterval
	}

	idleTimeSinceSent := now.Sub(lastNonPingSent)
	idleTimeSinceRecv := now.Sub(lastNonPongRecv)
	minIdle := idleTimeSinceSent
	if idleTimeSinceRecv < minIdle {
		minIdle = idleTimeSinceRecv
	}

	switch {
	case minIdle < pingCoolThreshold:
		return pingLazyInterval
	case minIdle < pingCoolThreshold*2:
		return pingCoolDownInterval
	default:
		return pingColdInterval
	}
}

func (p *PingManager) pingLoop() {
	defer p.wg.Done()

	p.client.log.Debugf("\U0001F3D3 <cyan>Ping Manager loop started</cyan>")
	timer := time.NewTimer(pingAggressiveInterval)
	defer timer.Stop()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-p.wakeCh:
		case <-timer.C:
		}

		now := time.Now()
		interval := p.nextInterval(now)
		lastPing := time.Unix(0, p.lastPingSentAt.Load())

		if now.Sub(lastPing) >= interval {
			if p.client.SessionReady() {
				payload, err := buildClientPingPayload()
				if err == nil {
					// Use Stream 0 for pings
					p.client.streamsMu.RLock()
					s0 := p.client.active_streams[0]
					p.client.streamsMu.RUnlock()

					if s0 != nil {
						s0.PushTXPacket(5, Enums.PACKET_PING, 0, 0, 0, payload)
					}
				}
			}
		}

		checkInterval := interval / 2
		if checkInterval < 100*time.Millisecond {
			checkInterval = 100 * time.Millisecond
		}
		if checkInterval > 1*time.Second {
			checkInterval = 1 * time.Second
		}

		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(checkInterval)
	}
}

func buildClientPingPayload() ([]byte, error) {
	payload := []byte{'P', 'O', ':'}
	randomPart, err := randomBytes(4)
	if err != nil {
		return nil, err
	}
	return append(payload, randomPart...), nil
}
