// ==============================================================================
// MasterDnsVPN
// Author: MasterkinG32
// Github: https://github.com/masterking32
// Year: 2026
// ==============================================================================

package client

import (
	"context"
	"errors"
	"time"

	Enums "masterdnsvpn-go/internal/enums"
	VpnProto "masterdnsvpn-go/internal/vpnproto"
)

var ErrSessionDropped = errors.New("session dropped by server")

const (
	mtuRetryInterval             = 5 * time.Second
	sessionInitRetryInterval     = time.Second
	sessionInitBusyRetryInterval = time.Minute
)

func (c *Client) RunConnectionManager(ctx context.Context, ready chan<- struct{}) error {
	if c == nil {
		return nil
	}

	readySent := false
initLoop:
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}

		if !c.HasSuccessfulMTUChecks() || c.Balancer().ValidCount() == 0 {
			c.resetSessionInitState()
			if err := c.RunInitialMTUTests(ctx); err != nil {
				if c.log != nil {
					c.log.Warnf("\u23F3 <yellow>No valid servers found, retrying MTU tests in <cyan>%d</cyan> seconds</yellow>", int(mtuRetryInterval/time.Second))
				}
				if !sleepWithContext(ctx, mtuRetryInterval) {
					return nil
				}
				continue
			}

			if c.log != nil {
				c.log.Infof(
					"\U0001F4CF <green>Initial MTU Sync Completed, Upload: <cyan>%d</cyan> Download: <cyan>%d</cyan></green>",
					c.SyncedUploadMTU(),
					c.SyncedDownloadMTU(),
				)
			}
		}

		if busyUntil := c.sessionInitBusyUntil(); !busyUntil.IsZero() {
			if remaining := time.Until(busyUntil); remaining > 0 {
				if !sleepWithContext(ctx, remaining) {
					return nil
				}
				continue
			}
			c.clearSessionInitBusyUntil()
		}

		initFailures := 0
		for !c.SessionReady() {
			if err := ctx.Err(); err != nil {
				return nil
			}

			err := c.InitializeSession(1)
			switch {
			case err == nil:
				if c.log != nil {
					c.log.Infof(
						"\U0001F91D <green>Session Established ID: <cyan>%d</cyan> Cookie: <cyan>%d</cyan></green>",
						c.SessionID(),
						c.SessionCookie(),
					)
				}
				if !readySent {
					closeReadyChannel(ready)
					readySent = true
				}
				return nil
			case errors.Is(err, ErrSessionInitBusy):
				if c.log != nil {
					c.log.Warnf("\U0001F6AB <yellow>Server session capacity is full, retrying session init in <cyan>%d</cyan> seconds</yellow>", int(sessionInitBusyRetryInterval/time.Second))
				}
				if !sleepWithContext(ctx, sessionInitBusyRetryInterval) {
					return nil
				}
			case errors.Is(err, ErrNoValidConnections):
				c.MarkMTUChecksStale()
				c.resetSessionInitState()
				continue initLoop
			default:
				initFailures++
				delay := time.Duration(0)
				if initFailures > 10 {
					delay = 5 * time.Second
				}
				if delay > 0 && !sleepWithContext(ctx, delay) {
					return nil
				}
			}
		}
	}
}

func (c *Client) handleServerDropPacket(packet VpnProto.Packet) error {
	if c == nil || packet.PacketType != Enums.PACKET_ERROR_DROP {
		return nil
	}
	if !c.shouldReconnectForDrop(packet) {
		return nil
	}

	if c.log != nil {
		c.log.Errorf(
			"🪂 <red>Session Dropped By Server: <cyan>%d</cyan> (Restarted, Invalid Or Closed Session)</red>",
			packet.SessionID,
		)
	}
	c.requestSessionReset()
	return ErrSessionDropped
}

func (c *Client) shouldReconnectForDrop(packet VpnProto.Packet) bool {
	if c == nil || packet.PacketType != Enums.PACKET_ERROR_DROP {
		return false
	}
	if c.sessionResetPending.Load() {
		return false
	}
	if c.sessionReady {
		return packet.SessionID == c.sessionID
	}
	return c.sessionID != 0 && packet.SessionID == c.sessionID
}

func (c *Client) requestSessionReset() {
	if c == nil || !c.sessionResetPending.CompareAndSwap(false, true) {
		return
	}
	c.resetSessionInitState()
	c.ResetRuntimeState(true)
	select {
	case c.sessionResetSignal <- struct{}{}:
	default:
	}
}

func (c *Client) clearSessionResetPending() {
	if c == nil {
		return
	}
	c.sessionResetPending.Store(false)
}

func (c *Client) waitForSessionReset(ctx context.Context) bool {
	if c == nil {
		return false
	}
	select {
	case <-ctx.Done():
		return false
	case <-c.sessionResetSignal:
		return true
	}
}

func (c *Client) WaitForSessionReset(ctx context.Context) bool {
	return c.waitForSessionReset(ctx)
}

func sleepWithContext(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		delay = time.Millisecond
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func closeReadyChannel(ready chan<- struct{}) {
	if ready == nil {
		return
	}
	close(ready)
}
