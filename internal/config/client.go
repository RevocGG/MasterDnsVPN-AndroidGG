// ==============================================================================
// MasterDnsVPN
// Author: MasterkinG32
// Github: https://github.com/masterking32
// Year: 2026
// ==============================================================================

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"masterdnsvpn-go/internal/compression"
)

type ClientConfig struct {
	ConfigDir                             string            `toml:"-"`
	ConfigPath                            string            `toml:"-"`
	ProtocolType                          string            `toml:"PROTOCOL_TYPE"`
	Domains                               []string          `toml:"DOMAINS"`
	ListenIP                              string            `toml:"LISTEN_IP"`
	ListenPort                            int               `toml:"LISTEN_PORT"`
	LocalSOCKS5Enabled                    bool              `toml:"LOCAL_SOCKS5_ENABLED"`
	LocalSOCKS5IP                         string            `toml:"LOCAL_SOCKS5_IP"`
	LocalSOCKS5Port                       int               `toml:"LOCAL_SOCKS5_PORT"`
	LocalSOCKS5HandshakeSec               float64           `toml:"LOCAL_SOCKS5_HANDSHAKE_TIMEOUT_SECONDS"`
	SOCKS5Auth                            bool              `toml:"SOCKS5_AUTH"`
	SOCKS5User                            string            `toml:"SOCKS5_USER"`
	SOCKS5Pass                            string            `toml:"SOCKS5_PASS"`
	LocalDNSEnabled                       bool              `toml:"LOCAL_DNS_ENABLED"`
	LocalDNSIP                            string            `toml:"LOCAL_DNS_IP"`
	LocalDNSPort                          int               `toml:"LOCAL_DNS_PORT"`
	LocalDNSWorkers                       int               `toml:"LOCAL_DNS_WORKERS"`
	LocalDNSQueueSize                     int               `toml:"LOCAL_DNS_QUEUE_SIZE"`
	LocalDNSCacheMaxRecords               int               `toml:"LOCAL_DNS_CACHE_MAX_RECORDS"`
	LocalDNSCacheTTLSeconds               float64           `toml:"LOCAL_DNS_CACHE_TTL_SECONDS"`
	LocalDNSPendingTimeoutSec             float64           `toml:"LOCAL_DNS_PENDING_TIMEOUT_SECONDS"`
	LocalDNSFragmentTimeoutSec            float64           `toml:"LOCAL_DNS_FRAGMENT_ASSEMBLY_TIMEOUT_SECONDS"`
	LocalDNSCachePersist                  bool              `toml:"LOCAL_DNS_CACHE_PERSIST_TO_FILE"`
	LocalDNSCacheFlushSec                 float64           `toml:"LOCAL_DNS_CACHE_FLUSH_INTERVAL_SECONDS"`
	ResolverBalancingStrategy             int               `toml:"RESOLVER_BALANCING_STRATEGY"`
	AutoDisableTimeoutServers             bool              `toml:"AUTO_DISABLE_TIMEOUT_SERVERS"`
	AutoDisableTimeoutWindow              float64           `toml:"AUTO_DISABLE_TIMEOUT_WINDOW_SECONDS"`
	AutoDisableMinObservations            int               `toml:"AUTO_DISABLE_TIMEOUT_MIN_OBSERVATIONS"`
	AutoDisableCheckInterval              float64           `toml:"AUTO_DISABLE_CHECK_INTERVAL_SECONDS"`
	RecheckInactiveEnabled                bool              `toml:"RECHECK_INACTIVE_SERVERS_ENABLED"`
	RecheckInactiveInterval               float64           `toml:"RECHECK_INACTIVE_INTERVAL_SECONDS"`
	RecheckServerInterval                 float64           `toml:"RECHECK_SERVER_INTERVAL_SECONDS"`
	RecheckBatchSize                      int               `toml:"RECHECK_BATCH_SIZE"`
	PacketDuplicationCount                int               `toml:"PACKET_DUPLICATION_COUNT"`
	SetupPacketDuplicationCount           int               `toml:"SETUP_PACKET_DUPLICATION_COUNT"`
	StreamResolverFailoverResendThreshold int               `toml:"STREAM_RESOLVER_FAILOVER_RESEND_THRESHOLD"`
	StreamResolverFailoverCooldownSec     float64           `toml:"STREAM_RESOLVER_FAILOVER_COOLDOWN"`
	MaxPacketsPerBatch                    int               `toml:"MAX_PACKETS_PER_BATCH"`
	StreamTXWindow                        int               `toml:"STREAM_TX_WINDOW"`
	StreamTXQueueLimit                    int               `toml:"STREAM_TX_QUEUE_LIMIT"`
	StreamTXMaxRetries                    int               `toml:"STREAM_TX_MAX_RETRIES"`
	StreamTXTTLSeconds                    float64           `toml:"STREAM_TX_TTL_SECONDS"`
	BaseEncodeData                        bool              `toml:"BASE_ENCODE_DATA"`
	UploadCompressionType                 int               `toml:"UPLOAD_COMPRESSION_TYPE"`
	DownloadCompressionType               int               `toml:"DOWNLOAD_COMPRESSION_TYPE"`
	CompressionMinSize                    int               `toml:"COMPRESSION_MIN_SIZE"`
	DataEncryptionMethod                  int               `toml:"DATA_ENCRYPTION_METHOD"`
	EncryptionKey                         string            `toml:"ENCRYPTION_KEY"`
	MinUploadMTU                          int               `toml:"MIN_UPLOAD_MTU"`
	MinDownloadMTU                        int               `toml:"MIN_DOWNLOAD_MTU"`
	MaxUploadMTU                          int               `toml:"MAX_UPLOAD_MTU"`
	MaxDownloadMTU                        int               `toml:"MAX_DOWNLOAD_MTU"`
	MTUTestRetries                        int               `toml:"MTU_TEST_RETRIES"`
	MTUTestTimeout                        float64           `toml:"MTU_TEST_TIMEOUT"`
	MTUTestParallelism                    int               `toml:"MTU_TEST_PARALLELISM"`
	SaveMTUServersToFile                  bool              `toml:"SAVE_MTU_SERVERS_TO_FILE"`
	MTUServersFileName                    string            `toml:"MTU_SERVERS_FILE_NAME"`
	MTUServersFileFormat                  string            `toml:"MTU_SERVERS_FILE_FORMAT"`
	MTUUsingSeparatorText                 string            `toml:"MTU_USING_SECTION_SEPARATOR_TEXT"`
	MTURemovedServerLogFormat             string            `toml:"MTU_REMOVED_SERVER_LOG_FORMAT"`
	MTUAddedServerLogFormat               string            `toml:"MTU_ADDED_SERVER_LOG_FORMAT"`
	LogLevel                              string            `toml:"LOG_LEVEL"`
	Resolvers                             []ResolverAddress `toml:"-"`
	ResolverMap                           map[string]int    `toml:"-"`
}

func defaultClientConfig() ClientConfig {
	return ClientConfig{
		ProtocolType:                          "SOCKS5",
		Domains:                               nil,
		ListenIP:                              "127.0.0.1",
		ListenPort:                            1080,
		LocalSOCKS5Enabled:                    false,
		LocalSOCKS5IP:                         "127.0.0.1",
		LocalSOCKS5Port:                       1080,
		LocalSOCKS5HandshakeSec:               10.0,
		SOCKS5Auth:                            false,
		SOCKS5User:                            "",
		SOCKS5Pass:                            "",
		LocalDNSEnabled:                       false,
		LocalDNSIP:                            "127.0.0.1",
		LocalDNSPort:                          5353,
		LocalDNSWorkers:                       2,
		LocalDNSQueueSize:                     512,
		LocalDNSCacheMaxRecords:               2000,
		LocalDNSCacheTTLSeconds:               3600.0,
		LocalDNSPendingTimeoutSec:             600.0,
		LocalDNSFragmentTimeoutSec:            300.0,
		LocalDNSCachePersist:                  true,
		LocalDNSCacheFlushSec:                 60.0,
		ResolverBalancingStrategy:             0,
		AutoDisableTimeoutServers:             true,
		AutoDisableTimeoutWindow:              300.0,
		AutoDisableMinObservations:            3,
		AutoDisableCheckInterval:              1.0,
		RecheckInactiveEnabled:                true,
		RecheckInactiveInterval:               1800.0,
		RecheckServerInterval:                 3.0,
		RecheckBatchSize:                      5,
		PacketDuplicationCount:                1,
		SetupPacketDuplicationCount:           2,
		StreamResolverFailoverResendThreshold: 2,
		StreamResolverFailoverCooldownSec:     1.0,
		MaxPacketsPerBatch:                    5,
		StreamTXWindow:                        4,
		StreamTXQueueLimit:                    128,
		StreamTXMaxRetries:                    24,
		StreamTXTTLSeconds:                    120.0,
		BaseEncodeData:                        false,
		UploadCompressionType:                 compression.TypeOff,
		DownloadCompressionType:               compression.TypeOff,
		CompressionMinSize:                    compression.DefaultMinSize,
		DataEncryptionMethod:                  1,
		EncryptionKey:                         "",
		MinUploadMTU:                          70,
		MinDownloadMTU:                        150,
		MaxUploadMTU:                          150,
		MaxDownloadMTU:                        200,
		MTUTestRetries:                        2,
		MTUTestTimeout:                        2.0,
		MTUTestParallelism:                    6,
		SaveMTUServersToFile:                  false,
		MTUServersFileName:                    "masterdnsvpn_success_test_{time}.log",
		MTUServersFileFormat:                  "{IP} - UP: {UP_MTU} DOWN: {DOWN-MTU}",
		MTUUsingSeparatorText:                 "",
		MTURemovedServerLogFormat:             "",
		MTUAddedServerLogFormat:               "",
		LogLevel:                              "INFO",
	}
}

func LoadClientConfig(filename string) (ClientConfig, error) {
	cfg := defaultClientConfig()
	path, err := filepath.Abs(filename)
	if err != nil {
		return cfg, err
	}

	if _, err := os.Stat(path); err != nil {
		return cfg, fmt.Errorf("config file not found: %s", path)
	}

	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return cfg, fmt.Errorf("parse TOML failed for %s: %w", path, err)
	}

	cfg.ConfigPath = path
	cfg.ConfigDir = filepath.Dir(path)
	cfg.ProtocolType = strings.ToUpper(strings.TrimSpace(cfg.ProtocolType))
	cfg.LogLevel = strings.TrimSpace(cfg.LogLevel)
	if cfg.LogLevel == "" {
		cfg.LogLevel = "INFO"
	}

	switch cfg.ProtocolType {
	case "", "SOCKS5":
		cfg.ProtocolType = "SOCKS5"
	case "TCP":
	default:
		return cfg, fmt.Errorf("invalid PROTOCOL_TYPE: %q", cfg.ProtocolType)
	}

	if cfg.DataEncryptionMethod < 0 || cfg.DataEncryptionMethod > 5 {
		return cfg, fmt.Errorf("invalid DATA_ENCRYPTION_METHOD: %d", cfg.DataEncryptionMethod)
	}

	cfg.ListenIP = defaultString(strings.TrimSpace(cfg.ListenIP), "127.0.0.1")

	if cfg.ListenPort < 0 || cfg.ListenPort > 65535 {
		return cfg, fmt.Errorf("invalid LISTEN_PORT: %d", cfg.ListenPort)
	}

	cfg.LocalSOCKS5IP = defaultString(strings.TrimSpace(cfg.LocalSOCKS5IP), "127.0.0.1")

	if cfg.LocalSOCKS5Port < 0 || cfg.LocalSOCKS5Port > 65535 {
		return cfg, fmt.Errorf("invalid LOCAL_SOCKS5_PORT: %d", cfg.LocalSOCKS5Port)
	}

	switch cfg.ProtocolType {
	case "SOCKS5":
		if !cfg.LocalSOCKS5Enabled {
			cfg.LocalSOCKS5IP = cfg.ListenIP
			cfg.LocalSOCKS5Port = cfg.ListenPort
		}
	case "TCP":
		cfg.LocalSOCKS5Enabled = false
	}

	cfg.LocalSOCKS5HandshakeSec = defaultFloatAtMostZero(cfg.LocalSOCKS5HandshakeSec, 10.0)

	if len(cfg.SOCKS5User) > 255 {
		return cfg, fmt.Errorf("SOCKS5_USER cannot exceed 255 bytes")
	}

	if len(cfg.SOCKS5Pass) > 255 {
		return cfg, fmt.Errorf("SOCKS5_PASS cannot exceed 255 bytes")
	}

	if cfg.SOCKS5Auth && (cfg.SOCKS5User == "" || cfg.SOCKS5Pass == "") {
		return cfg, fmt.Errorf("SOCKS5_AUTH requires both SOCKS5_USER and SOCKS5_PASS")
	}

	cfg.LocalDNSIP = defaultString(strings.TrimSpace(cfg.LocalDNSIP), "127.0.0.1")

	if cfg.LocalDNSPort < 0 || cfg.LocalDNSPort > 65535 {
		return cfg, fmt.Errorf("invalid LOCAL_DNS_PORT: %d", cfg.LocalDNSPort)
	}

	cfg.LocalDNSWorkers = defaultIntBelow(cfg.LocalDNSWorkers, 1, 1)
	cfg.LocalDNSQueueSize = defaultIntBelow(cfg.LocalDNSQueueSize, 1, 512)
	cfg.LocalDNSCacheMaxRecords = defaultIntBelow(cfg.LocalDNSCacheMaxRecords, 1, 2000)
	cfg.LocalDNSCacheTTLSeconds = defaultFloatAtMostZero(cfg.LocalDNSCacheTTLSeconds, 3600.0)
	cfg.LocalDNSPendingTimeoutSec = defaultFloatAtMostZero(cfg.LocalDNSPendingTimeoutSec, 600.0)
	cfg.LocalDNSFragmentTimeoutSec = defaultFloatAtMostZero(cfg.LocalDNSFragmentTimeoutSec, 300.0)
	cfg.LocalDNSCacheFlushSec = defaultFloatAtMostZero(cfg.LocalDNSCacheFlushSec, 60.0)
	cfg.MaxPacketsPerBatch = defaultIntBelow(cfg.MaxPacketsPerBatch, 1, 5)
	cfg.StreamTXWindow = clampInt(defaultIntBelow(cfg.StreamTXWindow, 1, 4), 1, 32)
	cfg.StreamTXQueueLimit = clampInt(defaultIntBelow(cfg.StreamTXQueueLimit, 1, 128), 1, 4096)
	cfg.StreamTXMaxRetries = clampInt(defaultIntBelow(cfg.StreamTXMaxRetries, 1, 24), 1, 512)
	cfg.StreamTXTTLSeconds = defaultFloatAtMostZero(cfg.StreamTXTTLSeconds, 120.0)

	if cfg.UploadCompressionType < compression.TypeOff || cfg.UploadCompressionType > compression.TypeZLIB {
		return cfg, fmt.Errorf("invalid UPLOAD_COMPRESSION_TYPE: %d", cfg.UploadCompressionType)
	}

	if cfg.DownloadCompressionType < compression.TypeOff || cfg.DownloadCompressionType > compression.TypeZLIB {
		return cfg, fmt.Errorf("invalid DOWNLOAD_COMPRESSION_TYPE: %d", cfg.DownloadCompressionType)
	}

	cfg.CompressionMinSize = defaultIntBelow(cfg.CompressionMinSize, 1, compression.DefaultMinSize)

	if cfg.ResolverBalancingStrategy < 0 || cfg.ResolverBalancingStrategy > 4 {
		return cfg, fmt.Errorf("invalid RESOLVER_BALANCING_STRATEGY: %d", cfg.ResolverBalancingStrategy)
	}

	cfg.AutoDisableTimeoutWindow = defaultFloatAtMostZero(cfg.AutoDisableTimeoutWindow, 300.0)
	cfg.AutoDisableMinObservations = defaultIntBelow(cfg.AutoDisableMinObservations, 1, 3)
	cfg.AutoDisableCheckInterval = defaultFloatBelow(cfg.AutoDisableCheckInterval, 0.5, 1.0)
	cfg.RecheckInactiveInterval = defaultFloatBelow(cfg.RecheckInactiveInterval, 60.0, 1800.0)
	cfg.RecheckServerInterval = defaultFloatBelow(cfg.RecheckServerInterval, 1.0, 3.0)
	cfg.RecheckBatchSize = clampInt(defaultIntBelow(cfg.RecheckBatchSize, 1, 1), 1, 64)
	cfg.PacketDuplicationCount = clampInt(defaultIntBelow(cfg.PacketDuplicationCount, 1, 1), 1, 8)
	cfg.SetupPacketDuplicationCount = clampInt(defaultIntBelow(cfg.SetupPacketDuplicationCount, 1, max(2, cfg.PacketDuplicationCount)), cfg.PacketDuplicationCount, 8)
	cfg.StreamResolverFailoverResendThreshold = clampInt(defaultIntBelow(cfg.StreamResolverFailoverResendThreshold, 1, 2), 1, 64)
	cfg.StreamResolverFailoverCooldownSec = defaultFloatBelow(cfg.StreamResolverFailoverCooldownSec, 0.1, 1.0)

	if cfg.MinUploadMTU < 0 || cfg.MinDownloadMTU < 0 || cfg.MaxUploadMTU < 0 || cfg.MaxDownloadMTU < 0 {
		return cfg, fmt.Errorf("mtu values cannot be negative")
	}

	if cfg.MaxUploadMTU > 0 && cfg.MinUploadMTU > cfg.MaxUploadMTU {
		return cfg, fmt.Errorf("MIN_UPLOAD_MTU cannot be greater than MAX_UPLOAD_MTU")
	}

	if cfg.MaxDownloadMTU > 0 && cfg.MinDownloadMTU > cfg.MaxDownloadMTU {
		return cfg, fmt.Errorf("MIN_DOWNLOAD_MTU cannot be greater than MAX_DOWNLOAD_MTU")
	}

	cfg.MTUTestRetries = defaultIntBelow(cfg.MTUTestRetries, 1, 1)
	cfg.MTUTestTimeout = defaultFloatAtMostZero(cfg.MTUTestTimeout, 1.0)
	cfg.MTUTestParallelism = defaultIntBelow(cfg.MTUTestParallelism, 1, 1)
	cfg.MTUServersFileName = strings.TrimSpace(cfg.MTUServersFileName)
	cfg.MTUServersFileFormat = strings.TrimSpace(cfg.MTUServersFileFormat)
	cfg.MTUUsingSeparatorText = strings.TrimSpace(cfg.MTUUsingSeparatorText)
	cfg.MTURemovedServerLogFormat = strings.TrimSpace(cfg.MTURemovedServerLogFormat)
	cfg.MTUAddedServerLogFormat = strings.TrimSpace(cfg.MTUAddedServerLogFormat)

	cfg.EncryptionKey = strings.TrimSpace(cfg.EncryptionKey)
	if cfg.EncryptionKey == "" {
		return cfg, fmt.Errorf("ENCRYPTION_KEY is required in client config")
	}

	cfg.Domains = normalizeClientDomains(cfg.Domains)
	if len(cfg.Domains) == 0 {
		return cfg, fmt.Errorf("DOMAINS must contain at least one domain")
	}

	resolvers, resolverMap, err := LoadClientResolvers(cfg.ResolversPath())
	if err != nil {
		return cfg, err
	}
	cfg.Resolvers = resolvers
	cfg.ResolverMap = resolverMap
	return cfg, nil
}

func (c ClientConfig) ResolversPath() string {
	return filepath.Join(c.ConfigDir, "client_resolvers.txt")
}

func (c ClientConfig) LocalDNSCachePath() string {
	return filepath.Join(c.ConfigDir, "local_dns_cache.json")
}

func normalizeClientDomains(domains []string) []string {
	if len(domains) == 0 {
		return nil
	}

	unique := make(map[string]struct{}, len(domains))
	for _, domain := range domains {
		normalized := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")
		if normalized == "" || normalized == "." {
			continue
		}
		unique[normalized] = struct{}{}
	}

	if len(unique) == 0 {
		return nil
	}

	normalized := make([]string, 0, len(unique))
	for domain := range unique {
		normalized = append(normalized, domain)
	}

	sort.Slice(normalized, func(i, j int) bool {
		if len(normalized[i]) == len(normalized[j]) {
			return normalized[i] < normalized[j]
		}
		return len(normalized[i]) > len(normalized[j])
	})

	return normalized
}

func defaultString(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func defaultIntBelow(value int, minValue int, fallback int) int {
	if value < minValue {
		return fallback
	}
	return value
}

func clampInt(value int, minValue int, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func defaultFloatAtMostZero(value float64, fallback float64) float64 {
	if value <= 0 {
		return fallback
	}
	return value
}

func defaultFloatBelow(value float64, minValue float64, fallback float64) float64 {
	if value < minValue {
		return fallback
	}
	return value
}
