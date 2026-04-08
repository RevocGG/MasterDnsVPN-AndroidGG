// ==============================================================================
// MasterDnsVPN
// Author: MasterkinG32
// Github: https://github.com/masterking32
// Year: 2026
// ==============================================================================

package config

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadServerConfigWithOverridesAppliesFlagPrecedence(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "server_config.toml")

	if err := os.WriteFile(configPath, []byte(`
PROTOCOL_TYPE = "SOCKS5"
UDP_PORT = 53
DOMAIN = ["config.example.com"]
DATA_ENCRYPTION_METHOD = 1
SUPPORTED_UPLOAD_COMPRESSION_TYPES = [0, 3]
SUPPORTED_DOWNLOAD_COMPRESSION_TYPES = [0, 3]
`), 0o644); err != nil {
		t.Fatalf("WriteFile config failed: %v", err)
	}

	cfg, err := LoadServerConfigWithOverrides(configPath, ServerConfigOverrides{
		Values: map[string]any{
			"UDPPort":                           5300,
			"Domain":                            []string{"flag.example.com", "alt.example.com"},
			"DataEncryptionMethod":              2,
			"SupportedUploadCompressionTypes":   []int{0, 1},
			"SupportedDownloadCompressionTypes": []int{0, 1, 3},
		},
	})
	if err != nil {
		t.Fatalf("LoadServerConfigWithOverrides returned error: %v", err)
	}

	if cfg.UDPPort != 5300 {
		t.Fatalf("unexpected udp port override: got=%d want=%d", cfg.UDPPort, 5300)
	}
	if len(cfg.Domain) != 2 || cfg.Domain[0] != "flag.example.com" || cfg.Domain[1] != "alt.example.com" {
		t.Fatalf("unexpected domain override: %+v", cfg.Domain)
	}
	if cfg.DataEncryptionMethod != 2 {
		t.Fatalf("unexpected data encryption override: got=%d want=%d", cfg.DataEncryptionMethod, 2)
	}
	if len(cfg.SupportedUploadCompressionTypes) != 2 || cfg.SupportedUploadCompressionTypes[0] != 0 || cfg.SupportedUploadCompressionTypes[1] != 1 {
		t.Fatalf("unexpected upload compression override: %+v", cfg.SupportedUploadCompressionTypes)
	}
	if len(cfg.SupportedDownloadCompressionTypes) != 3 {
		t.Fatalf("unexpected download compression override: %+v", cfg.SupportedDownloadCompressionTypes)
	}
}

func TestServerConfigFlagBinderBuildsOverridesForSetFlagsOnly(t *testing.T) {
	fs := flag.NewFlagSet("server", flag.ContinueOnError)
	binder, err := NewServerConfigFlagBinder(fs)
	if err != nil {
		t.Fatalf("NewServerConfigFlagBinder returned error: %v", err)
	}

	if err := fs.Parse([]string{
		"-udp-port=5300",
		"-domain=a.example.com,b.example.com",
		"-use-external-socks5",
		"-supported-upload-compression-types=0,1",
		"-data-encryption-method=2",
	}); err != nil {
		t.Fatalf("flag parse failed: %v", err)
	}

	overrides := binder.Overrides()
	if got, ok := overrides.Values["UDPPort"].(int); !ok || got != 5300 {
		t.Fatalf("unexpected udp port override: %#v", overrides.Values["UDPPort"])
	}
	if got, ok := overrides.Values["UseExternalSOCKS5"].(bool); !ok || !got {
		t.Fatalf("unexpected socks5 override: %#v", overrides.Values["UseExternalSOCKS5"])
	}
	if got, ok := overrides.Values["DataEncryptionMethod"].(int); !ok || got != 2 {
		t.Fatalf("unexpected encryption method override: %#v", overrides.Values["DataEncryptionMethod"])
	}
	gotDomains, ok := overrides.Values["Domain"].([]string)
	if !ok || len(gotDomains) != 2 || gotDomains[0] != "a.example.com" || gotDomains[1] != "b.example.com" {
		t.Fatalf("unexpected domains override: %#v", overrides.Values["Domain"])
	}
	gotCompressions, ok := overrides.Values["SupportedUploadCompressionTypes"].([]int)
	if !ok || len(gotCompressions) != 2 || gotCompressions[0] != 0 || gotCompressions[1] != 1 {
		t.Fatalf("unexpected compression override: %#v", overrides.Values["SupportedUploadCompressionTypes"])
	}
	if _, exists := overrides.Values["UDPHost"]; exists {
		t.Fatalf("did not expect unset flag to appear in overrides: %#v", overrides.Values["UDPHost"])
	}
}

func TestServerConfigEffectiveSizingUsesSmartFloorsAndDerivedCapacities(t *testing.T) {
	cfg := defaultServerConfig()
	cfg.ProtocolType = "SOCKS5"
	cfg.UDPReaders = 1
	cfg.DNSRequestWorkers = 1
	cfg.DeferredSessionWorkers = 1
	cfg.DeferredSessionQueueLimit = 64
	cfg.MaxConcurrentRequests = 512
	cfg.MaxPacketsPerBatch = 1
	cfg.DNSCacheMaxRecords = 100
	cfg.ARQWindowSize = 2000

	if got := cfg.EffectiveUDPReaders(); got < 1 {
		t.Fatalf("expected effective udp readers floor, got=%d", got)
	}
	if got := cfg.EffectiveDNSRequestWorkers(); got < cfg.EffectiveUDPReaders()+1 {
		t.Fatalf("expected dns workers to track reader pressure, got=%d readers=%d", got, cfg.EffectiveUDPReaders())
	}
	if got := cfg.EffectiveDeferredSessionQueueLimit(); got < 256 {
		t.Fatalf("expected deferred queue smart floor, got=%d", got)
	}
	if got := cfg.EffectiveMaxConcurrentRequests(); got < 4096 {
		t.Fatalf("expected max concurrent requests smart floor, got=%d", got)
	}
	if got := cfg.EffectiveMaxPacketsPerBatch(); got < 10 {
		t.Fatalf("expected max packets per batch smart floor, got=%d", got)
	}
	if got := cfg.EffectiveDNSCacheMaxRecords(); got < cfg.EffectiveMaxConcurrentRequests()*2 {
		t.Fatalf("expected dns cache smart floor, got=%d concurrent=%d", got, cfg.EffectiveMaxConcurrentRequests())
	}
	if got := cfg.EffectiveSessionOrphanQueueInitialCap(); got < 32 {
		t.Fatalf("expected derived orphan queue cap, got=%d", got)
	}
	if got := cfg.EffectiveStreamQueueInitialCapacity(); got < 32 {
		t.Fatalf("expected derived stream queue cap, got=%d", got)
	}
	if got := cfg.EffectiveDNSFragmentStoreCapacity(); got < 64 {
		t.Fatalf("expected derived dns fragment store cap, got=%d", got)
	}
	if got := cfg.EffectiveSOCKS5FragmentStoreCapacity(); got < 64 {
		t.Fatalf("expected derived socks5 fragment store cap, got=%d", got)
	}
}
