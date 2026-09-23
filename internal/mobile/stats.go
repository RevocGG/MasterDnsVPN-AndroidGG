// MasterDnsVPN  Android Mobile Adapter
// File: internal/mobile/stats.go
package mobile

// MobileStats contains a snapshot of tunnel runtime metrics.
type MobileStats struct {
	IsRunning          bool
	SessionReady       bool
	ResolverCount      int
	ValidResolverCount int
	// CheckedResolverCount is the number of connections whose initial MTU/health
	// probe has FINISHED (valid OR rejected). During the initial scan it grows
	// from 0 to ResolverCount, which lets the UI draw a true 0→100% scan
	// progress bar (progress = checked/total, not valid/total).
	CheckedResolverCount int
	ListenAddr           string
	ProfileDir           string
}
