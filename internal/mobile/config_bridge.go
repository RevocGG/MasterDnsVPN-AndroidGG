package mobile

import (
"fmt"
"os"
"path/filepath"
"strings"
)

type MobileClientConfig struct {
Domains             string
DataEncryptionMethod int
EncryptionKey       string
ProtocolType string
ListenIP     string
ListenPort   int
SOCKS5Auth   bool
SOCKS5User   string
SOCKS5Pass   string
LocalDNSEnabled           bool
LocalDNSIP                string
LocalDNSPort              int
LocalDNSCacheMaxRecords   int
LocalDNSCacheTTLSeconds   float64
LocalDNSPendingTimeoutSec float64
LocalDNSCachePersist      bool
LocalDNSCacheFlushSec     float64
ResolverBalancingStrategy             int
PacketDuplicationCount                int
SetupPacketDuplicationCount           int
StreamResolverFailoverResendThreshold int
StreamResolverFailoverCooldownSec     float64
RecheckInactiveServersEnabled   bool
RecheckInactiveIntervalSeconds  float64
RecheckServerIntervalSeconds    float64
RecheckBatchSize                int
AutoDisableTimeoutServers       bool
AutoDisableTimeoutWindowSeconds float64
AutoDisableMinObservations      int
AutoDisableCheckIntervalSeconds float64
BaseEncodeData        bool
UploadCompressionType int
DownloadCompressionType int
CompressionMinSize    int
MinUploadMTU      int
MinDownloadMTU    int
MaxUploadMTU      int
MaxDownloadMTU    int
MTUTestRetries    int
MTUTestTimeout    float64
MTUTestParallelism int
RxTxWorkers                       int
TunnelProcessWorkers              int
TunnelPacketTimeoutSec            float64
DispatcherIdlePollIntervalSeconds float64
PingAggressiveIntervalSeconds float64
PingLazyIntervalSeconds       float64
PingCooldownIntervalSeconds   float64
PingColdIntervalSeconds       float64
PingWarmThresholdSeconds      float64
PingCoolThresholdSeconds      float64
PingColdThresholdSeconds      float64
TXChannelSize                        int
RXChannelSize                        int
ResolverUDPConnectionPoolSize        int
StreamQueueInitialCapacity           int
OrphanQueueInitialCapacity           int
DNSResponseFragmentStoreCap          int
DNSResponseFragmentTimeoutSeconds    float64
SOCKSUDPAssociateReadTimeoutSeconds  float64
ClientTerminalStreamRetentionSeconds float64
ClientCancelledSetupRetentionSeconds float64
SessionInitRetryBaseSeconds          float64
SessionInitRetryStepSeconds          float64
SessionInitRetryLinearAfter          int
SessionInitRetryMaxSeconds           float64
SessionInitBusyRetryIntervalSeconds  float64
SessionInitRacingCount               int
SaveMTUServersToFile  bool
MTUServersFileName    string
MTUServersFileFormat  string
MTUUsingSeparatorText string
MTURemovedServerLogFormat string
MTUAddedServerLogFormat   string
LogLevel string
MaxPacketsPerBatch            int
ARQWindowSize                 int
ARQInitialRTOSeconds          float64
ARQMaxRTOSeconds              float64
ARQControlInitialRTOSeconds   float64
ARQControlMaxRTOSeconds       float64
ARQMaxControlRetries          int
ARQInactivityTimeoutSeconds   float64
ARQDataPacketTTLSeconds       float64
ARQControlPacketTTLSeconds    float64
ARQMaxDataRetries             int
ARQDataNackMaxGap             int
ARQDataNackInitialDelaySeconds float64
ARQDataNackRepeatSeconds      float64
ARQTerminalDrainTimeoutSec    float64
ARQTerminalAckWaitTimeoutSec  float64
}

func NewDefaultMobileClientConfig() MobileClientConfig {
return MobileClientConfig{
Domains: "", DataEncryptionMethod: 1, EncryptionKey: "",
ProtocolType: "SOCKS5", ListenIP: "127.0.0.1", ListenPort: 18000,
SOCKS5Auth: false, SOCKS5User: "master_dns_vpn", SOCKS5Pass: "master_dns_vpn",
LocalDNSEnabled: false, LocalDNSIP: "127.0.0.1", LocalDNSPort: 5353,
LocalDNSCacheMaxRecords: 5000, LocalDNSCacheTTLSeconds: 28800.0,
LocalDNSPendingTimeoutSec: 300.0, LocalDNSCachePersist: true, LocalDNSCacheFlushSec: 60.0,
ResolverBalancingStrategy: 0, PacketDuplicationCount: 5, SetupPacketDuplicationCount: 5,
StreamResolverFailoverResendThreshold: 2, StreamResolverFailoverCooldownSec: 1.0,
RecheckInactiveServersEnabled: true, RecheckInactiveIntervalSeconds: 1800.0,
RecheckServerIntervalSeconds: 3.0, RecheckBatchSize: 5,
AutoDisableTimeoutServers: true, AutoDisableTimeoutWindowSeconds: 180.0,
AutoDisableMinObservations: 6, AutoDisableCheckIntervalSeconds: 3.0,
BaseEncodeData: false, UploadCompressionType: 0, DownloadCompressionType: 0, CompressionMinSize: 100,
MinUploadMTU: 40, MinDownloadMTU: 100, MaxUploadMTU: 64, MaxDownloadMTU: 140,
MTUTestRetries: 2, MTUTestTimeout: 4.0, MTUTestParallelism: 16,
RxTxWorkers: 6, TunnelProcessWorkers: 4,
TunnelPacketTimeoutSec: 8.0, DispatcherIdlePollIntervalSeconds: 0.020,
PingAggressiveIntervalSeconds: 0.300, PingLazyIntervalSeconds: 1.0,
PingCooldownIntervalSeconds: 3.0, PingColdIntervalSeconds: 30.0,
PingWarmThresholdSeconds: 5.0, PingCoolThresholdSeconds: 10.0, PingColdThresholdSeconds: 20.0,
TXChannelSize: 4096, RXChannelSize: 4096, ResolverUDPConnectionPoolSize: 64,
StreamQueueInitialCapacity: 128, OrphanQueueInitialCapacity: 32,
DNSResponseFragmentStoreCap: 256, DNSResponseFragmentTimeoutSeconds: 10.0,
SOCKSUDPAssociateReadTimeoutSeconds: 30.0,
ClientTerminalStreamRetentionSeconds: 45.0, ClientCancelledSetupRetentionSeconds: 120.0,
SessionInitRetryBaseSeconds: 1.0, SessionInitRetryStepSeconds: 1.0,
SessionInitRetryLinearAfter: 5, SessionInitRetryMaxSeconds: 60.0,
SessionInitBusyRetryIntervalSeconds: 60.0,
SessionInitRacingCount: 3,
SaveMTUServersToFile: false,
MTUServersFileName: "masterdnsvpn_success_{time}.log",
MTUServersFileFormat: "{IP}", MTUUsingSeparatorText: "",
MTURemovedServerLogFormat: "Resolver {IP} removed at {TIME} due to {CAUSE}",
MTUAddedServerLogFormat: "Resolver {IP} added back at {TIME} (UP {UP_MTU}, DOWN {DOWN_MTU})",
LogLevel: "INFO", MaxPacketsPerBatch: 8,
ARQWindowSize: 2000, ARQInitialRTOSeconds: 1.0, ARQMaxRTOSeconds: 8.0,
ARQControlInitialRTOSeconds: 1.0, ARQControlMaxRTOSeconds: 8.0,
ARQMaxControlRetries: 80, ARQInactivityTimeoutSeconds: 1800.0,
ARQDataPacketTTLSeconds: 1800.0, ARQControlPacketTTLSeconds: 900.0,
ARQMaxDataRetries: 800, ARQDataNackMaxGap: 0, ARQDataNackInitialDelaySeconds: 0.4, ARQDataNackRepeatSeconds: 2.0,
ARQTerminalDrainTimeoutSec: 90.0, ARQTerminalAckWaitTimeoutSec: 60.0,
}
}

func WriteConfigFiles(cfg MobileClientConfig, resolversText string, profileDir string) error {
if err := os.MkdirAll(profileDir, 0o750); err != nil {
return fmt.Errorf("failed to create profile directory: %w", err)
}
if err := writeConfigTOML(cfg, filepath.Join(profileDir, "client_config.toml")); err != nil {
return fmt.Errorf("failed to write config: %w", err)
}
if err := writeResolversFile(resolversText, filepath.Join(profileDir, "client_resolvers.txt")); err != nil {
return fmt.Errorf("failed to write resolvers: %w", err)
}
return nil
}

func ConfigFilePath(profileDir string) string {
return filepath.Join(profileDir, "client_config.toml")
}

func writeResolversFile(resolversText string, path string) error {
if strings.TrimSpace(resolversText) == "" {
resolversText = "# No resolvers configured\n"
}
return os.WriteFile(path, []byte(resolversText), 0o640)
}

func writeConfigTOML(cfg MobileClientConfig, path string) error {
f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
if err != nil { return err }
defer f.Close()
var domainsArr []string
for _, d := range strings.Split(cfg.Domains, ",") {
d = strings.TrimSpace(d)
if d != "" { domainsArr = append(domainsArr, `"`+strings.ReplaceAll(d, `"`, ``)+`"`) }
}
domainsToml := "[" + strings.Join(domainsArr, ", ") + "]"
tpl := "DOMAINS = %s\nDATA_ENCRYPTION_METHOD = %d\nENCRYPTION_KEY = %q\n" +
"PROTOCOL_TYPE = %q\nLISTEN_IP = %q\nLISTEN_PORT = %d\n" +
"SOCKS5_AUTH = %t\nSOCKS5_USER = %q\nSOCKS5_PASS = %q\n" +
"LOCAL_DNS_ENABLED = %t\nLOCAL_DNS_IP = %q\nLOCAL_DNS_PORT = %d\n" +
"LOCAL_DNS_CACHE_MAX_RECORDS = %d\nLOCAL_DNS_CACHE_TTL_SECONDS = %g\n" +
"LOCAL_DNS_PENDING_TIMEOUT_SECONDS = %g\nLOCAL_DNS_CACHE_PERSIST_TO_FILE = %t\n" +
"LOCAL_DNS_CACHE_FLUSH_INTERVAL_SECONDS = %g\n" +
"RESOLVER_BALANCING_STRATEGY = %d\nPACKET_DUPLICATION_COUNT = %d\n" +
"SETUP_PACKET_DUPLICATION_COUNT = %d\n" +
"STREAM_RESOLVER_FAILOVER_RESEND_THRESHOLD = %d\nSTREAM_RESOLVER_FAILOVER_COOLDOWN = %g\n" +
"RECHECK_INACTIVE_SERVERS_ENABLED = %t\nRECHECK_INACTIVE_INTERVAL_SECONDS = %g\n" +
"RECHECK_SERVER_INTERVAL_SECONDS = %g\nRECHECK_BATCH_SIZE = %d\n" +
"AUTO_DISABLE_TIMEOUT_SERVERS = %t\nAUTO_DISABLE_TIMEOUT_WINDOW_SECONDS = %g\n" +
"AUTO_DISABLE_MIN_OBSERVATIONS = %d\nAUTO_DISABLE_CHECK_INTERVAL_SECONDS = %g\n" +
"BASE_ENCODE_DATA = %t\nUPLOAD_COMPRESSION_TYPE = %d\nDOWNLOAD_COMPRESSION_TYPE = %d\n" +
"COMPRESSION_MIN_SIZE = %d\n" +
"MIN_UPLOAD_MTU = %d\nMIN_DOWNLOAD_MTU = %d\nMAX_UPLOAD_MTU = %d\nMAX_DOWNLOAD_MTU = %d\n" +
"MTU_TEST_RETRIES = %d\nMTU_TEST_TIMEOUT = %g\nMTU_TEST_PARALLELISM = %d\n" +
"RX_TX_WORKERS = %d\nTUNNEL_PROCESS_WORKERS = %d\n" +
"TUNNEL_PACKET_TIMEOUT_SECONDS = %g\nDISPATCHER_IDLE_POLL_INTERVAL_SECONDS = %g\n" +
"PING_AGGRESSIVE_INTERVAL_SECONDS = %g\nPING_LAZY_INTERVAL_SECONDS = %g\n" +
"PING_COOLDOWN_INTERVAL_SECONDS = %g\nPING_COLD_INTERVAL_SECONDS = %g\n" +
"PING_WARM_THRESHOLD_SECONDS = %g\nPING_COOL_THRESHOLD_SECONDS = %g\nPING_COLD_THRESHOLD_SECONDS = %g\n" +
"TX_CHANNEL_SIZE = %d\nRX_CHANNEL_SIZE = %d\nRESOLVER_UDP_CONNECTION_POOL_SIZE = %d\n" +
"STREAM_QUEUE_INITIAL_CAPACITY = %d\nORPHAN_QUEUE_INITIAL_CAPACITY = %d\n" +
"DNS_RESPONSE_FRAGMENT_STORE_CAPACITY = %d\nDNS_RESPONSE_FRAGMENT_TIMEOUT_SECONDS = %g\n" +
"SOCKS_UDP_ASSOCIATE_READ_TIMEOUT_SECONDS = %g\n" +
"CLIENT_TERMINAL_STREAM_RETENTION_SECONDS = %g\nCLIENT_CANCELLED_SETUP_RETENTION_SECONDS = %g\n" +
"SESSION_INIT_RETRY_BASE_SECONDS = %g\nSESSION_INIT_RETRY_STEP_SECONDS = %g\n" +
"SESSION_INIT_RETRY_LINEAR_AFTER = %d\nSESSION_INIT_RETRY_MAX_SECONDS = %g\n" +
"SESSION_INIT_BUSY_RETRY_INTERVAL_SECONDS = %g\n" +
"SESSION_INIT_RACING_COUNT = %d\n" +
"SAVE_MTU_SERVERS_TO_FILE = %t\nMTU_SERVERS_FILE_NAME = %q\nMTU_SERVERS_FILE_FORMAT = %q\n" +
"MTU_USING_SECTION_SEPARATOR_TEXT = %q\nMTU_REMOVED_SERVER_LOG_FORMAT = %q\nMTU_ADDED_SERVER_LOG_FORMAT = %q\n" +
"LOG_LEVEL = %q\n" +
"MAX_PACKETS_PER_BATCH = %d\nARQ_WINDOW_SIZE = %d\n" +
"ARQ_INITIAL_RTO_SECONDS = %g\nARQ_MAX_RTO_SECONDS = %g\n" +
"ARQ_CONTROL_INITIAL_RTO_SECONDS = %g\nARQ_CONTROL_MAX_RTO_SECONDS = %g\n" +
"ARQ_MAX_CONTROL_RETRIES = %d\nARQ_INACTIVITY_TIMEOUT_SECONDS = %g\n" +
"ARQ_DATA_PACKET_TTL_SECONDS = %g\nARQ_CONTROL_PACKET_TTL_SECONDS = %g\n" +
"ARQ_MAX_DATA_RETRIES = %d\nARQ_DATA_NACK_MAX_GAP = %d\nARQ_DATA_NACK_INITIAL_DELAY_SECONDS = %g\nARQ_DATA_NACK_REPEAT_SECONDS = %g\n" +
"ARQ_TERMINAL_DRAIN_TIMEOUT_SECONDS = %g\nARQ_TERMINAL_ACK_WAIT_TIMEOUT_SECONDS = %g\n"
_, err = fmt.Fprintf(f, tpl,
domainsToml, cfg.DataEncryptionMethod, cfg.EncryptionKey,
cfg.ProtocolType, cfg.ListenIP, cfg.ListenPort, cfg.SOCKS5Auth, cfg.SOCKS5User, cfg.SOCKS5Pass,
cfg.LocalDNSEnabled, cfg.LocalDNSIP, cfg.LocalDNSPort, cfg.LocalDNSCacheMaxRecords,
cfg.LocalDNSCacheTTLSeconds, cfg.LocalDNSPendingTimeoutSec, cfg.LocalDNSCachePersist, cfg.LocalDNSCacheFlushSec,
cfg.ResolverBalancingStrategy, cfg.PacketDuplicationCount, cfg.SetupPacketDuplicationCount,
cfg.StreamResolverFailoverResendThreshold, cfg.StreamResolverFailoverCooldownSec,
cfg.RecheckInactiveServersEnabled, cfg.RecheckInactiveIntervalSeconds,
cfg.RecheckServerIntervalSeconds, cfg.RecheckBatchSize,
cfg.AutoDisableTimeoutServers, cfg.AutoDisableTimeoutWindowSeconds,
cfg.AutoDisableMinObservations, cfg.AutoDisableCheckIntervalSeconds,
cfg.BaseEncodeData, cfg.UploadCompressionType, cfg.DownloadCompressionType, cfg.CompressionMinSize,
cfg.MinUploadMTU, cfg.MinDownloadMTU, cfg.MaxUploadMTU, cfg.MaxDownloadMTU,
cfg.MTUTestRetries, cfg.MTUTestTimeout, cfg.MTUTestParallelism,
cfg.RxTxWorkers, cfg.TunnelProcessWorkers,
cfg.TunnelPacketTimeoutSec, cfg.DispatcherIdlePollIntervalSeconds,
cfg.PingAggressiveIntervalSeconds, cfg.PingLazyIntervalSeconds,
cfg.PingCooldownIntervalSeconds, cfg.PingColdIntervalSeconds,
cfg.PingWarmThresholdSeconds, cfg.PingCoolThresholdSeconds, cfg.PingColdThresholdSeconds,
cfg.TXChannelSize, cfg.RXChannelSize, cfg.ResolverUDPConnectionPoolSize,
cfg.StreamQueueInitialCapacity, cfg.OrphanQueueInitialCapacity,
cfg.DNSResponseFragmentStoreCap, cfg.DNSResponseFragmentTimeoutSeconds,
cfg.SOCKSUDPAssociateReadTimeoutSeconds, cfg.ClientTerminalStreamRetentionSeconds,
cfg.ClientCancelledSetupRetentionSeconds,
cfg.SessionInitRetryBaseSeconds, cfg.SessionInitRetryStepSeconds,
cfg.SessionInitRetryLinearAfter, cfg.SessionInitRetryMaxSeconds, cfg.SessionInitBusyRetryIntervalSeconds,
cfg.SessionInitRacingCount,
cfg.SaveMTUServersToFile, cfg.MTUServersFileName, cfg.MTUServersFileFormat,
cfg.MTUUsingSeparatorText, cfg.MTURemovedServerLogFormat, cfg.MTUAddedServerLogFormat,
cfg.LogLevel, cfg.MaxPacketsPerBatch, cfg.ARQWindowSize,
cfg.ARQInitialRTOSeconds, cfg.ARQMaxRTOSeconds, cfg.ARQControlInitialRTOSeconds, cfg.ARQControlMaxRTOSeconds,
cfg.ARQMaxControlRetries, cfg.ARQInactivityTimeoutSeconds,
cfg.ARQDataPacketTTLSeconds, cfg.ARQControlPacketTTLSeconds,
cfg.ARQMaxDataRetries, cfg.ARQDataNackMaxGap, cfg.ARQDataNackInitialDelaySeconds, cfg.ARQDataNackRepeatSeconds,
cfg.ARQTerminalDrainTimeoutSec, cfg.ARQTerminalAckWaitTimeoutSec,
)
return err
}
