//go:build linux || android

// Package mobile — tun_api.go
//
// Exported gomobile-compatible functions that Android calls to manage the
// tun2socks bridge lifecycle.
//
// NOTE: StartTunBridge's signature intentionally keeps the original
// (tunFd, mtu, listenAddr) parameters for backwards compatibility with the
// Kotlin bridge. The local SOCKS5 credentials are resolved automatically from
// the single running instance registered via StartInstance, so both the
// single-profile and meta (balancer) flows pick up SOCKS5_USER/SOCKS5_PASS
// without any Kotlin-side change.
// ==============================================================================
package mobile

import (
	"context"
	"errors"
	"fmt"
)

// StartTunBridge starts the tun2socks bridge.
// It must be called AFTER StartInstance so the SOCKS5 proxy is already
// listening on listenAddr.
//
// The bridge reads the local SOCKS5 credentials from the running instance
// registered under instanceID (set via MobileConfig.SOCKS5Auth/User/Pass),
// so the bridge can authenticate against the proxy in TUN mode when the
// profile enables SOCKS5 authentication.
//
// Parameters:
//
//	instanceID – instance key used in StartInstance (e.g. the profile id).
//	tunFd      – raw int file descriptor from Android's
//	             ParcelFileDescriptor.getFd() (VpnService.establish()).
//	mtu        – MTU of the TUN interface (typically 1500).
//	listenAddr – "host:port" of the SOCKS5 proxy started by StartInstance
//	             (e.g. "127.0.0.1:1080").
func StartTunBridge(instanceID string, tunFd int32, mtu int32, listenAddr string) error {
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

	// Resolve the local SOCKS5 credentials from the running instance so the
	// bridge's dialer can perform USER_PASS authentication when required.
	tunSocksAuth, tunSocksUser, tunSocksPass = instanceSocksCredentials(instanceID)
	if tunSocksAuth {
		bridgeLog("TUN bridge using SOCKS5 auth for user %q on %s", tunSocksUser, listenAddr)
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
	// Credentials (tunSocks*) are kept so a restart via StartTunBridge
	// refreshes them from the running instance.
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

// SetTunDisableIPv6 toggles the bridge-wide "Disable IPv6" behaviour
// (DNS AAAA filtering + fast RST of IPv6 TCP flows). The setting is global
// for the bridge (all profiles share one TUN stack) and defaults to true —
// the DNS tunnel has no IPv6 egress, so blocking IPv6 avoids
// "socks5 connect refused code 3" timeouts in apps like YouTube.
func SetTunDisableIPv6(disabled bool) { SetDisableIPv6(disabled) }

// GetTunDisableIPv6 reports the current "Disable IPv6" state.
func GetTunDisableIPv6() bool { return GetDisableIPv6() }

// logTunError emits a log entry through the registered log callback (if any).
func logTunError(msg string) {
	deliverLogEntry(LogEntry{
		Level:     LogLevelError,
		Timestamp: "tun_bridge",
		Message:   msg,
	})
}
