// Package mobile exposes the MasterDnsVPN client for use via gomobile bind.
package mobile

import (
	"fmt"
	"strings"

	"masterdnsvpn-go/internal/mobile"
)

// MobileLogCallback receives log entries from the Go tunnel runtime.
type MobileLogCallback interface {
OnLog(level int, timestamp string, message string)
}

// MobileProtectCallback is called for each UDP socket fd that the tunnel opens.
// The Android VpnService.protect(fd) must be called in the implementation.
type MobileProtectCallback interface {
Protect(fd int32) bool
}

// MobileConfig holds all configuration needed to start a tunnel instance.
type MobileConfig struct {
// Tunnel data dir  writable directory for config/resolver files (e.g. getFilesDir())
ProfileDir      string
// Resolvers inline text (same format as client_resolvers.simple)
ResolversInline string
// Identity
Domains              string
DataEncryptionMethod int
EncryptionKey        string
// Proxy listener
ProtocolType string
ListenIP     string
ListenPort   int
SOCKS5Auth   bool
SOCKS5User   string
SOCKS5Pass   string
// Local DNS
LocalDNSEnabled         bool
LocalDNSIP              string
LocalDNSPort            int
LocalDNSCacheMaxRecords int
LocalDNSCacheTTLSec     float64
LocalDNSPendingTimeout  float64
LocalDNSCachePersist    bool
LocalDNSCacheFlushSec   float64
// Balancing
ResolverBalancingStrategy int
PacketDuplicationCount    int
SetupDuplicationCount     int
// Resolver health
StreamFailoverResendThreshold int
StreamFailoverCooldownSec     float64
RecheckInactiveEnabled        bool
RecheckInactiveIntervalSec    float64
RecheckServerIntervalSec      float64
RecheckBatchSize              int
AutoDisableTimeoutServers     bool
AutoDisableTimeoutWindowSec   float64
AutoDisableMinObservations    int
AutoDisableCheckIntervalSec   float64
// Encoding / Compression
BaseEncodeData          bool
UploadCompressionType   int
DownloadCompressionType int
CompressionMinSize      int
// MTU
MinUploadMTU    int
MinDownloadMTU  int
MaxUploadMTU    int
MaxDownloadMTU  int
MTUTestRetries  int
MTUTestTimeout  float64
MTUTestParallel int
// Workers & Timeouts
TunnelReaderWorkers           int
TunnelWriterWorkers           int
TunnelProcessWorkers          int
TunnelPacketTimeoutSec        float64
DispatcherIdlePollIntervalSec float64
// Ping
PingAggressiveIntervalSec float64
PingLazyIntervalSec       float64
PingCooldownIntervalSec   float64
PingColdIntervalSec       float64
PingWarmThresholdSec      float64
PingCoolThresholdSec      float64
PingColdThresholdSec      float64
// Channels / Buffers
TXChannelSize                    int
RXChannelSize                    int
ResolverUDPPoolSize              int
StreamQueueInitCap               int
OrphanQueueInitCap               int
DNSFragmentStoreCap              int
DNSFragmentTimeoutSec            float64
SOCKSUDPAssocReadTimeoutSec      float64
ClientTerminalStreamRetentionSec float64
ClientCancelledSetupRetentionSec float64
// Session retry
SessionInitRetryBaseSec         float64
SessionInitRetryStepSec         float64
SessionInitRetryLinearAfter     int
SessionInitRetryMaxSec          float64
SessionInitBusyRetryIntervalSec float64
// MTU file
SaveMTUServersToFile      bool
MTUServersFileName        string
MTUServersFileFormat      string
MTUUsingSeparatorText     string
MTURemovedServerLogFormat string
MTUAddedServerLogFormat   string
// Misc
LogLevel           string
MaxPacketsPerBatch int
// ARQ
ARQWindowSize                int
ARQInitialRTOSec             float64
ARQMaxRTOSec                 float64
ARQControlInitialRTOSec      float64
ARQControlMaxRTOSec          float64
ARQMaxControlRetries         int
ARQInactivityTimeoutSec      float64
ARQDataPacketTTLSec          float64
ARQControlPacketTTLSec       float64
ARQMaxDataRetries            int
ARQDataNackMaxGap                int
ARQDataNackInitialDelaySec       float64
ARQDataNackRepeatSec             float64
ARQTerminalDrainTimeoutSec       float64
ARQTerminalAckWaitTimeoutSec float64
}

// Stats reports current tunnel state.
type Stats struct {
IsRunning          bool
SessionReady       bool
ResolverCount      int
ValidResolverCount int
ListenAddr         string
}

// NewDefaultConfig returns a MobileConfig pre-filled with safe defaults.
func NewDefaultConfig() *MobileConfig {
d := mobile.NewDefaultMobileClientConfig()
c := &MobileConfig{}
copyDefaultsToMobileConfig(d, c)
return c
}

// SetLogCallback registers a callback for all Go log output.
func SetLogCallback(cb MobileLogCallback) {
if cb == nil {
mobile.SetLogCallback(nil)
return
}
mobile.SetLogCallback(func(e mobile.LogEntry) {
cb.OnLog(e.Level, e.Timestamp, e.Message)
})
}

// SetProtectCallback registers the VPN socket protect hook.
func SetProtectCallback(cb MobileProtectCallback) {
if cb == nil {
mobile.SetProtectCallback(nil)
return
}
mobile.SetProtectCallback(func(fd int32) bool {
return cb.Protect(fd)
})
}

// StartInstance starts a tunnel for the given instance key.
func StartInstance(key string, cfg *MobileConfig) error {
ic := toInternalConfig(cfg)
return mobile.StartInstance(key, cfg.ProfileDir, ic, cfg.ResolversInline)
}

// StopInstance stops a running tunnel.
func StopInstance(key string) {
mobile.StopInstance(key)
}

// StopAll stops every running tunnel.
func StopAll() {
mobile.StopAllInstances()
}

// IsRunning returns true if the given instance is active.
func IsRunning(key string) bool {
return mobile.IsInstanceRunning(key)
}

// GetStats returns current stats for an instance.
func GetStats(key string) *Stats {
s := mobile.GetInstanceStats(key)
return &Stats{
IsRunning:          s.IsRunning,
SessionReady:       s.SessionReady,
ResolverCount:      s.ResolverCount,
ValidResolverCount: s.ValidResolverCount,
ListenAddr:         s.ListenAddr,
}
}

// GetLastError returns the last error message for an instance, or empty string.
func GetLastError(key string) string {
err := mobile.GetInstanceLastError(key)
if err == nil {
return ""
}
return err.Error()
}

// StartTunBridge starts the tun2socks bridge that forwards all TUN traffic
// through the SOCKS5 proxy already running via StartInstance.
//
// Must be called AFTER StartInstance so the proxy port is ready.
// tunFd is the raw fd from Android's ParcelFileDescriptor.getFd();
// mtu is the MTU of the TUN interface (1500 by default);
// listenAddr is "host:port" of the SOCKS5 proxy (e.g. "127.0.0.1:1080").
func StartTunBridge(tunFd int32, mtu int32, listenAddr string) error {
return mobile.StartTunBridge(tunFd, mtu, listenAddr)
}

// StopTunBridge tears down the tun2socks bridge.
// Safe to call if the bridge was never started.
func StopTunBridge() {
mobile.StopTunBridge()
}

// IsTunBridgeRunning returns true if the tun2socks bridge is currently active.
func IsTunBridgeRunning() bool {
return mobile.IsTunBridgeRunning()
}

// BandwidthStats holds upload/download byte counters for the TUN bridge.
type BandwidthStats struct {
UploadBytes   int64
DownloadBytes int64
}

// GetBandwidth returns current TUN bridge bandwidth counters.
func GetBandwidth() *BandwidthStats {
up, down := mobile.GetTunBandwidth()
return &BandwidthStats{UploadBytes: up, DownloadBytes: down}
}

// StartSocksBalancer starts a SOCKS5 load-balancer on listenAddr that
// distributes connections across upstreamCSV (comma-separated "host:port").
// Returns the actual listen address.
func StartSocksBalancer(listenAddr string, upstreamCSV string, strategy int) (string, error) {
	parts := splitCSV(upstreamCSV)
	if len(parts) == 0 {
		return "", fmt.Errorf("no upstream addresses in %q", upstreamCSV)
	}
	return mobile.StartSocksBalancer(listenAddr, parts, strategy)
}

// StopSocksBalancer stops the SOCKS5 load-balancer.
func StopSocksBalancer() {
	mobile.StopSocksBalancer()
}

// IsSocksBalancerRunning returns true if the balancer is active.
func IsSocksBalancerRunning() bool {
	return mobile.IsSocksBalancerRunning()
}

// UpdateBalancerUpstreams hot-swaps the upstream list.
func UpdateBalancerUpstreams(upstreamCSV string) {
	parts := splitCSV(upstreamCSV)
	mobile.UpdateBalancerUpstreams(parts)
}

// ------------------------------------------------------------------
// Hotspot proxy
// ------------------------------------------------------------------

// StartHotspotProxy starts a transparent TCP relay on 0.0.0.0:listenPort
// that forwards all SOCKS5 traffic to the local SOCKS5 proxy at
// upstreamSocksAddr (e.g. "127.0.0.1:1080").
//
// Devices connected to the Android Wi-Fi hotspot can then configure
// SOCKS5 proxy <hotspot-IP>:<listenPort> to tunnel through MasterDnsVPN.
//
// Returns the listening address ("0.0.0.0:PORT") or an error string.
func StartHotspotProxy(listenPort int32, upstreamSocksAddr string) (string, error) {
	return mobile.StartHotspotProxy(listenPort, upstreamSocksAddr)
}

// StopHotspotProxy stops the hotspot relay.
func StopHotspotProxy() {
	mobile.StopHotspotProxy()
}

// IsHotspotProxyRunning returns true if the hotspot relay is active.
func IsHotspotProxyRunning() bool {
	return mobile.IsHotspotProxyRunning()
}

// GetHotspotPacPort returns the port the PAC file HTTP server is listening on.
// The PAC URL is: http://<hotspot-ip>:<port>/proxy.pac
func GetHotspotPacPort() int32 {
	return mobile.GetHotspotPacPort()
}

// SetHotspotPacSocksAddr sets the SOCKS5 address embedded in the PAC file.
// addr format: "ip:port", e.g. "192.168.43.1:8090"
func SetHotspotPacSocksAddr(addr string) {
	mobile.SetHotspotPacSocksAddr(addr)
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// toInternalConfig maps MobileConfig -> mobile.MobileClientConfig.
func toInternalConfig(c *MobileConfig) mobile.MobileClientConfig {
return mobile.MobileClientConfig{
Domains:              c.Domains,
DataEncryptionMethod: c.DataEncryptionMethod,
EncryptionKey:        c.EncryptionKey,
ProtocolType:         c.ProtocolType,
ListenIP:             c.ListenIP,
ListenPort:           c.ListenPort,
SOCKS5Auth:           c.SOCKS5Auth,
SOCKS5User:           c.SOCKS5User,
SOCKS5Pass:           c.SOCKS5Pass,
LocalDNSEnabled:          c.LocalDNSEnabled,
LocalDNSIP:               c.LocalDNSIP,
LocalDNSPort:             c.LocalDNSPort,
LocalDNSCacheMaxRecords:  c.LocalDNSCacheMaxRecords,
LocalDNSCacheTTLSeconds:  c.LocalDNSCacheTTLSec,
LocalDNSPendingTimeoutSec: c.LocalDNSPendingTimeout,
LocalDNSCachePersist:     c.LocalDNSCachePersist,
LocalDNSCacheFlushSec:    c.LocalDNSCacheFlushSec,
ResolverBalancingStrategy:             c.ResolverBalancingStrategy,
PacketDuplicationCount:                c.PacketDuplicationCount,
SetupPacketDuplicationCount:           c.SetupDuplicationCount,
StreamResolverFailoverResendThreshold: c.StreamFailoverResendThreshold,
StreamResolverFailoverCooldownSec:     c.StreamFailoverCooldownSec,
RecheckInactiveServersEnabled:         c.RecheckInactiveEnabled,
RecheckInactiveIntervalSeconds:        c.RecheckInactiveIntervalSec,
RecheckServerIntervalSeconds:          c.RecheckServerIntervalSec,
RecheckBatchSize:                      c.RecheckBatchSize,
AutoDisableTimeoutServers:             c.AutoDisableTimeoutServers,
AutoDisableTimeoutWindowSeconds:       c.AutoDisableTimeoutWindowSec,
AutoDisableMinObservations:            c.AutoDisableMinObservations,
AutoDisableCheckIntervalSeconds:       c.AutoDisableCheckIntervalSec,
BaseEncodeData:          c.BaseEncodeData,
UploadCompressionType:   c.UploadCompressionType,
DownloadCompressionType: c.DownloadCompressionType,
CompressionMinSize:      c.CompressionMinSize,
MinUploadMTU:       c.MinUploadMTU,
MinDownloadMTU:     c.MinDownloadMTU,
MaxUploadMTU:       c.MaxUploadMTU,
MaxDownloadMTU:     c.MaxDownloadMTU,
MTUTestRetries:     c.MTUTestRetries,
MTUTestTimeout:     c.MTUTestTimeout,
MTUTestParallelism: c.MTUTestParallel,
TunnelReaderWorkers:               c.TunnelReaderWorkers,
TunnelWriterWorkers:               c.TunnelWriterWorkers,
TunnelProcessWorkers:              c.TunnelProcessWorkers,
TunnelPacketTimeoutSec:            c.TunnelPacketTimeoutSec,
DispatcherIdlePollIntervalSeconds: c.DispatcherIdlePollIntervalSec,
PingAggressiveIntervalSeconds: c.PingAggressiveIntervalSec,
PingLazyIntervalSeconds:       c.PingLazyIntervalSec,
PingCooldownIntervalSeconds:   c.PingCooldownIntervalSec,
PingColdIntervalSeconds:       c.PingColdIntervalSec,
PingWarmThresholdSeconds:      c.PingWarmThresholdSec,
PingCoolThresholdSeconds:      c.PingCoolThresholdSec,
PingColdThresholdSeconds:      c.PingColdThresholdSec,
TXChannelSize:                 c.TXChannelSize,
RXChannelSize:                 c.RXChannelSize,
ResolverUDPConnectionPoolSize: c.ResolverUDPPoolSize,
StreamQueueInitialCapacity:    c.StreamQueueInitCap,
OrphanQueueInitialCapacity:    c.OrphanQueueInitCap,
DNSResponseFragmentStoreCap:       c.DNSFragmentStoreCap,
DNSResponseFragmentTimeoutSeconds: c.DNSFragmentTimeoutSec,
SOCKSUDPAssociateReadTimeoutSeconds:  c.SOCKSUDPAssocReadTimeoutSec,
ClientTerminalStreamRetentionSeconds: c.ClientTerminalStreamRetentionSec,
ClientCancelledSetupRetentionSeconds: c.ClientCancelledSetupRetentionSec,
SessionInitRetryBaseSeconds:         c.SessionInitRetryBaseSec,
SessionInitRetryStepSeconds:         c.SessionInitRetryStepSec,
SessionInitRetryLinearAfter:         c.SessionInitRetryLinearAfter,
SessionInitRetryMaxSeconds:          c.SessionInitRetryMaxSec,
SessionInitBusyRetryIntervalSeconds: c.SessionInitBusyRetryIntervalSec,
SaveMTUServersToFile:      c.SaveMTUServersToFile,
MTUServersFileName:        c.MTUServersFileName,
MTUServersFileFormat:      c.MTUServersFileFormat,
MTUUsingSeparatorText:     c.MTUUsingSeparatorText,
MTURemovedServerLogFormat: c.MTURemovedServerLogFormat,
MTUAddedServerLogFormat:   c.MTUAddedServerLogFormat,
LogLevel:           c.LogLevel,
MaxPacketsPerBatch: c.MaxPacketsPerBatch,
ARQWindowSize:               c.ARQWindowSize,
ARQInitialRTOSeconds:        c.ARQInitialRTOSec,
ARQMaxRTOSeconds:            c.ARQMaxRTOSec,
ARQControlInitialRTOSeconds: c.ARQControlInitialRTOSec,
ARQControlMaxRTOSeconds:     c.ARQControlMaxRTOSec,
ARQMaxControlRetries:        c.ARQMaxControlRetries,
ARQInactivityTimeoutSeconds: c.ARQInactivityTimeoutSec,
ARQDataPacketTTLSeconds:     c.ARQDataPacketTTLSec,
ARQControlPacketTTLSeconds:  c.ARQControlPacketTTLSec,
ARQMaxDataRetries:           c.ARQMaxDataRetries,
ARQDataNackMaxGap:                    c.ARQDataNackMaxGap,
ARQDataNackInitialDelaySeconds:       c.ARQDataNackInitialDelaySec,
ARQDataNackRepeatSeconds:             c.ARQDataNackRepeatSec,
ARQTerminalDrainTimeoutSec:  c.ARQTerminalDrainTimeoutSec,
ARQTerminalAckWaitTimeoutSec: c.ARQTerminalAckWaitTimeoutSec,
}
}

func copyDefaultsToMobileConfig(d mobile.MobileClientConfig, c *MobileConfig) {
c.Domains              = d.Domains
c.DataEncryptionMethod = d.DataEncryptionMethod
c.EncryptionKey        = d.EncryptionKey
c.ProtocolType         = d.ProtocolType
c.ListenIP             = d.ListenIP
c.ListenPort           = d.ListenPort
c.SOCKS5Auth           = d.SOCKS5Auth
c.SOCKS5User           = d.SOCKS5User
c.SOCKS5Pass           = d.SOCKS5Pass
c.LocalDNSEnabled         = d.LocalDNSEnabled
c.LocalDNSIP              = d.LocalDNSIP
c.LocalDNSPort            = d.LocalDNSPort
c.LocalDNSCacheMaxRecords = d.LocalDNSCacheMaxRecords
c.LocalDNSCacheTTLSec     = d.LocalDNSCacheTTLSeconds
c.LocalDNSPendingTimeout  = d.LocalDNSPendingTimeoutSec
c.LocalDNSCachePersist    = d.LocalDNSCachePersist
c.LocalDNSCacheFlushSec   = d.LocalDNSCacheFlushSec
c.ResolverBalancingStrategy    = d.ResolverBalancingStrategy
c.PacketDuplicationCount       = d.PacketDuplicationCount
c.SetupDuplicationCount        = d.SetupPacketDuplicationCount
c.StreamFailoverResendThreshold = d.StreamResolverFailoverResendThreshold
c.StreamFailoverCooldownSec     = d.StreamResolverFailoverCooldownSec
c.RecheckInactiveEnabled       = d.RecheckInactiveServersEnabled
c.RecheckInactiveIntervalSec   = d.RecheckInactiveIntervalSeconds
c.RecheckServerIntervalSec     = d.RecheckServerIntervalSeconds
c.RecheckBatchSize             = d.RecheckBatchSize
c.AutoDisableTimeoutServers    = d.AutoDisableTimeoutServers
c.AutoDisableTimeoutWindowSec  = d.AutoDisableTimeoutWindowSeconds
c.AutoDisableMinObservations   = d.AutoDisableMinObservations
c.AutoDisableCheckIntervalSec  = d.AutoDisableCheckIntervalSeconds
c.BaseEncodeData          = d.BaseEncodeData
c.UploadCompressionType   = d.UploadCompressionType
c.DownloadCompressionType = d.DownloadCompressionType
c.CompressionMinSize      = d.CompressionMinSize
c.MinUploadMTU    = d.MinUploadMTU
c.MinDownloadMTU  = d.MinDownloadMTU
c.MaxUploadMTU    = d.MaxUploadMTU
c.MaxDownloadMTU  = d.MaxDownloadMTU
c.MTUTestRetries  = d.MTUTestRetries
c.MTUTestTimeout  = d.MTUTestTimeout
c.MTUTestParallel = d.MTUTestParallelism
c.TunnelReaderWorkers  = d.TunnelReaderWorkers
c.TunnelWriterWorkers  = d.TunnelWriterWorkers
c.TunnelProcessWorkers = d.TunnelProcessWorkers
c.TunnelPacketTimeoutSec        = d.TunnelPacketTimeoutSec
c.DispatcherIdlePollIntervalSec = d.DispatcherIdlePollIntervalSeconds
c.PingAggressiveIntervalSec = d.PingAggressiveIntervalSeconds
c.PingLazyIntervalSec       = d.PingLazyIntervalSeconds
c.PingCooldownIntervalSec   = d.PingCooldownIntervalSeconds
c.PingColdIntervalSec       = d.PingColdIntervalSeconds
c.PingWarmThresholdSec      = d.PingWarmThresholdSeconds
c.PingCoolThresholdSec      = d.PingCoolThresholdSeconds
c.PingColdThresholdSec      = d.PingColdThresholdSeconds
c.TXChannelSize                   = d.TXChannelSize
c.RXChannelSize                   = d.RXChannelSize
c.ResolverUDPPoolSize             = d.ResolverUDPConnectionPoolSize
c.StreamQueueInitCap              = d.StreamQueueInitialCapacity
c.OrphanQueueInitCap              = d.OrphanQueueInitialCapacity
c.DNSFragmentStoreCap             = d.DNSResponseFragmentStoreCap
c.DNSFragmentTimeoutSec           = d.DNSResponseFragmentTimeoutSeconds
c.SOCKSUDPAssocReadTimeoutSec     = d.SOCKSUDPAssociateReadTimeoutSeconds
c.ClientTerminalStreamRetentionSec = d.ClientTerminalStreamRetentionSeconds
c.ClientCancelledSetupRetentionSec = d.ClientCancelledSetupRetentionSeconds
c.SessionInitRetryBaseSec         = d.SessionInitRetryBaseSeconds
c.SessionInitRetryStepSec         = d.SessionInitRetryStepSeconds
c.SessionInitRetryLinearAfter     = d.SessionInitRetryLinearAfter
c.SessionInitRetryMaxSec          = d.SessionInitRetryMaxSeconds
c.SessionInitBusyRetryIntervalSec = d.SessionInitBusyRetryIntervalSeconds
c.SaveMTUServersToFile      = d.SaveMTUServersToFile
c.MTUServersFileName        = d.MTUServersFileName
c.MTUServersFileFormat      = d.MTUServersFileFormat
c.MTUUsingSeparatorText     = d.MTUUsingSeparatorText
c.MTURemovedServerLogFormat = d.MTURemovedServerLogFormat
c.MTUAddedServerLogFormat   = d.MTUAddedServerLogFormat
c.LogLevel           = d.LogLevel
c.MaxPacketsPerBatch = d.MaxPacketsPerBatch
c.ARQWindowSize               = d.ARQWindowSize
c.ARQInitialRTOSec            = d.ARQInitialRTOSeconds
c.ARQMaxRTOSec                = d.ARQMaxRTOSeconds
c.ARQControlInitialRTOSec     = d.ARQControlInitialRTOSeconds
c.ARQControlMaxRTOSec         = d.ARQControlMaxRTOSeconds
c.ARQMaxControlRetries        = d.ARQMaxControlRetries
c.ARQInactivityTimeoutSec     = d.ARQInactivityTimeoutSeconds
c.ARQDataPacketTTLSec         = d.ARQDataPacketTTLSeconds
c.ARQControlPacketTTLSec      = d.ARQControlPacketTTLSeconds
c.ARQMaxDataRetries           = d.ARQMaxDataRetries
c.ARQDataNackMaxGap              = d.ARQDataNackMaxGap
c.ARQDataNackInitialDelaySec     = d.ARQDataNackInitialDelaySeconds
c.ARQDataNackRepeatSec           = d.ARQDataNackRepeatSeconds
c.ARQTerminalDrainTimeoutSec  = d.ARQTerminalDrainTimeoutSec
c.ARQTerminalAckWaitTimeoutSec = d.ARQTerminalAckWaitTimeoutSec
}
