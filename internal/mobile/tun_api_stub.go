//go:build !linux && !android

// ==============================================================================
// MasterDnsVPN
// Author: MasterkinG32
// Github: https://github.com/masterking32
// Year: 2026
// ==============================================================================
// Package mobile — tun_api_stub.go
//
// Stub exported API for platforms where the tun2socks bridge is unavailable.
// ==============================================================================
package mobile

// StartTunBridge is a no-op on non-Android/Linux platforms.
func StartTunBridge(_ int32, _ int32, _ string) error {
	return errTunNotSupported
}

// StopTunBridge is a no-op on non-Android/Linux platforms.
func StopTunBridge() {}

// IsTunBridgeRunning always returns false on non-Android/Linux platforms.
func IsTunBridgeRunning() bool { return false }
