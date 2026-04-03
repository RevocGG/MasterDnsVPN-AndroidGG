//go:build linux || android

// ==============================================================================
// MasterDnsVPN
// Author: MasterkinG32
// Github: https://github.com/masterking32
// Year: 2026
// ==============================================================================
// Package mobile — tun_api.go
//
// Exported gomobile-compatible functions that Android calls to manage the
// tun2socks bridge lifecycle.
// ==============================================================================
package mobile

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var (
	tunMu     sync.Mutex
	tunCancel context.CancelFunc
	tunDone   chan struct{}
)

// StartTunBridge starts the tun2socks bridge.
// It must be called AFTER StartInstance so the SOCKS5 proxy is already
// listening on listenAddr.
//
// Parameters:
//
//	tunFd      – raw int file descriptor from Android's
//	             ParcelFileDescriptor.getFd() (VpnService.establish()).
//	mtu        – MTU of the TUN interface (typically 1500).
//	listenAddr – "host:port" of the SOCKS5 proxy started by StartInstance
//	             (e.g. "127.0.0.1:1080").
func StartTunBridge(tunFd int32, mtu int32, listenAddr string) error {
	tunMu.Lock()

	if tunCancel != nil {
		tunMu.Unlock()
		return errors.New("tun bridge already running")
	}

	if listenAddr == "" {
		tunMu.Unlock()
		return errors.New("listenAddr must not be empty")
	}
	if tunFd < 0 {
		tunMu.Unlock()
		return fmt.Errorf("invalid tunFd: %d", tunFd)
	}
	tunMTU := int(mtu)
	if tunMTU <= 0 {
		tunMTU = 1500
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var runErr error
	tunCancel = cancel
	tunDone = done
	tunMu.Unlock() // Release lock before blocking — StopTunBridge needs it

	go func() {
		defer func() {
			// Clear bridge state before signalling completion.
			tunMu.Lock()
			tunCancel = nil
			tunDone = nil
			tunMu.Unlock()
			close(done)
		}()
		runErr = runTunBridge(ctx, int(tunFd), tunMTU, listenAddr)
		if runErr != nil && !errors.Is(runErr, context.Canceled) {
			logTunError(fmt.Sprintf("tun bridge exited: %v", runErr))
		}
	}()

	<-done // Block until the bridge goroutine finishes.

	// context.Canceled is a normal stop triggered by StopTunBridge — not an error.
	if errors.Is(runErr, context.Canceled) {
		return nil
	}
	return runErr
}

// StopTunBridge stops the tun2socks bridge and waits for it to exit.
func StopTunBridge() {
	tunMu.Lock()
	cancel := tunCancel
	done := tunDone
	// Do NOT clear tunCancel/tunDone here — the goroutine owns that cleanup.
	tunMu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

// IsTunBridgeRunning returns true if the bridge goroutine is currently active.
func IsTunBridgeRunning() bool {
	tunMu.Lock()
	defer tunMu.Unlock()
	return tunCancel != nil
}

// logTunError emits a log entry through the registered log callback (if any).
func logTunError(msg string) {
	deliverLogEntry(LogEntry{
		Level:     LogLevelError,
		Timestamp: "tun_bridge",
		Message:   msg,
	})
}
