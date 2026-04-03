//go:build !linux && !android

// ==============================================================================
// MasterDnsVPN
// Author: MasterkinG32
// Github: https://github.com/masterking32
// Year: 2026
// ==============================================================================
// Package mobile — tun_bridge_stub.go
//
// Stub implementations for platforms where gVisor fdbased is not available.
// The tun2socks bridge only runs on Android/Linux.
// ==============================================================================
package mobile

import (
	"context"
	"errors"
)

var errTunNotSupported = errors.New("tun bridge is only supported on Android/Linux")

func runTunBridge(_ context.Context, _ int, _ int, _ string) error {
	return errTunNotSupported
}

// GetTunBandwidth returns (0, 0) on non-Android/Linux platforms.
func GetTunBandwidth() (int64, int64) { return 0, 0 }

// StartHotspotProxy is a no-op on non-Android/Linux platforms.
func StartHotspotProxy(_ int32, _ string) (string, error) {
	return "", errors.New("hotspot proxy is only supported on Android/Linux")
}

// StopHotspotProxy is a no-op on non-Android/Linux platforms.
func StopHotspotProxy() {}

// IsHotspotProxyRunning always returns false on non-Android/Linux platforms.
func IsHotspotProxyRunning() bool { return false }
