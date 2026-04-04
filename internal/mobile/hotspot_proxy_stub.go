//go:build !linux && !android

package mobile

// GetHotspotPacPort returns 0 on non-Android/Linux platforms.
func GetHotspotPacPort() int32 { return 0 }

// SetHotspotPacSocksAddr is a no-op on non-Android/Linux platforms.
func SetHotspotPacSocksAddr(_ string) {}
