//go:build !linux && !android

// Package mobile — tun_api_stub.go
//
// Stub exported API for platforms where the tun2socks bridge is unavailable.
// ==============================================================================
package mobile

// StartTunBridge is a no-op on non-Android/Linux platforms.
func StartTunBridge(_ string, _ int32, _ int32, _ string) error {
	return errTunNotSupported
}

// StopTunBridge is a no-op on non-Android/Linux platforms.
func StopTunBridge() {}

// IsTunBridgeRunning always returns false on non-Android/Linux platforms.
func IsTunBridgeRunning() bool { return false }

// SetTunDisableIPv6 stores the setting so tests on any platform can exercise it.
func SetTunDisableIPv6(disabled bool) { SetDisableIPv6(disabled) }

// GetTunDisableIPv6 reports the current "Disable IPv6" state.
func GetTunDisableIPv6() bool { return GetDisableIPv6() }
