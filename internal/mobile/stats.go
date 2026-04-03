// MasterDnsVPN  Android Mobile Adapter
// File: internal/mobile/stats.go
package mobile

// MobileStats contains a snapshot of tunnel runtime metrics.
type MobileStats struct {
IsRunning          bool
SessionReady       bool
ResolverCount      int
ValidResolverCount int
ListenAddr         string
ProfileDir         string
}
