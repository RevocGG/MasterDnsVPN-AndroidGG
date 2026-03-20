// ==============================================================================
// MasterDnsVPN
// Author: MasterkinG32
// Github: https://github.com/masterking32
// Year: 2026
// ==============================================================================

package client

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"masterdnsvpn-go/internal/config"
	DnsEnums "masterdnsvpn-go/internal/enums"
	VpnProto "masterdnsvpn-go/internal/vpnproto"
)

func TestBinarySearchMTUSkipsDuplicateChecks(t *testing.T) {
	c := New(config.ClientConfig{
		MTUTestRetries: 1,
	}, nil, nil)

	calls := make(map[int]int)
	best := c.binarySearchMTU(context.Background(), "test", 30, 100, func(value int, _ bool) (bool, error) {
		calls[value]++
		return value <= 73, nil
	})

	if best != 73 {
		t.Fatalf("unexpected binary search result: got=%d want=%d", best, 73)
	}
	for value, count := range calls {
		if count != 1 {
			t.Fatalf("mtu candidate %d checked more than once: %d", value, count)
		}
	}
}

func TestEffectiveDownloadMTUProbeSizeAddsHeaderReserve(t *testing.T) {
	want := 150 + max(0, VpnProto.MaxHeaderRawSize()-VpnProto.HeaderRawSize(DnsEnums.PACKET_MTU_DOWN_RES))
	if got := effectiveDownloadMTUProbeSize(150); got != want {
		t.Fatalf("unexpected effective download mtu size: got=%d want=%d", got, want)
	}
}

func TestComputeSafeUploadMTU(t *testing.T) {
	tests := []struct {
		name           string
		uploadMTU      int
		cryptoOverhead int
		want           int
	}{
		{name: "zero", uploadMTU: 0, cryptoOverhead: 28, want: 0},
		{name: "no_overhead", uploadMTU: 150, cryptoOverhead: 0, want: 150},
		{name: "subtract_overhead", uploadMTU: 150, cryptoOverhead: 28, want: 122},
		{name: "floor_to_sixty_four", uploadMTU: 70, cryptoOverhead: 28, want: 64},
		{name: "clamp_to_upload", uploadMTU: 40, cryptoOverhead: 0, want: 40},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := computeSafeUploadMTU(tc.uploadMTU, tc.cryptoOverhead); got != tc.want {
				t.Fatalf("unexpected safe upload mtu: got=%d want=%d", got, tc.want)
			}
		})
	}
}

func TestMTUSuccessOutputFileLogging(t *testing.T) {
	tempDir := t.TempDir()
	c := New(config.ClientConfig{
		ConfigDir:                 tempDir,
		SaveMTUServersToFile:      true,
		MTUServersFileName:        "mtu_{time}.txt",
		MTUServersFileFormat:      "{IP} - UP: {UP_MTU} DOWN: {DOWN-MTU}",
		MTUUsingSeparatorText:     "---- Active MTU Testing Results ----",
		MTURemovedServerLogFormat: "IP {IP} removed from list at {TIME} due to {CAUSE}",
		MTUAddedServerLogFormat:   "Server {IP} re-added at {TIME} (UP MTU: {UP_MTU}, DOWN MTU: {DOWN_MTU})",
	}, nil, nil)

	now := time.Date(2026, time.March, 20, 12, 34, 56, 0, time.UTC)
	c.now = func() time.Time { return now }

	conn := &Connection{
		ResolverLabel:    "1.1.1.1:53",
		Domain:           "v.example.com",
		UploadMTUBytes:   145,
		UploadMTUChars:   188,
		DownloadMTUBytes: 190,
	}

	outputPath := c.prepareMTUSuccessOutputFile()
	if outputPath == "" {
		t.Fatal("expected mtu output path to be created")
	}

	c.appendMTUSuccessLine(conn)
	c.appendMTUUsageSeparatorOnce()
	c.appendMTURemovedServerLine(conn, "timeout")
	c.appendMTUAddedServerLine(conn)

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read mtu output file: %v", err)
	}

	text := string(data)
	expectedSnippets := []string{
		"1.1.1.1:53 - UP: 145 DOWN: 190",
		"---- Active MTU Testing Results ----",
		"IP 1.1.1.1:53 removed from list at 2026-03-20 12:34:56 due to timeout",
		"Server 1.1.1.1:53 re-added at 2026-03-20 12:34:56 (UP MTU: 145, DOWN MTU: 190)",
	}
	for _, snippet := range expectedSnippets {
		if !strings.Contains(text, snippet) {
			t.Fatalf("expected output to contain %q, got:\n%s", snippet, text)
		}
	}
}
