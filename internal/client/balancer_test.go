package client

import (
	"testing"
	"time"
)

func TestBalancerLeastLossFallsBackToRoundRobinWithoutStats(t *testing.T) {
	b := NewBalancer(BalancingLeastLoss, nil)
	connections := []*Connection{
		{Key: "a", IsValid: true},
		{Key: "b", IsValid: true},
		{Key: "c", IsValid: true},
	}
	b.SetConnections(connections)
	_ = b.SetConnectionValidity("a", true)
	_ = b.SetConnectionValidity("b", true)
	_ = b.SetConnectionValidity("c", true)

	first, ok := b.GetBestConnection()
	if !ok {
		t.Fatal("expected first connection")
	}
	second, ok := b.GetBestConnection()
	if !ok {
		t.Fatal("expected second connection")
	}
	third, ok := b.GetBestConnection()
	if !ok {
		t.Fatal("expected third connection")
	}

	if first.Key != "a" || second.Key != "b" || third.Key != "c" {
		t.Fatalf("expected round-robin a,b,c before stats, got %q,%q,%q", first.Key, second.Key, third.Key)
	}
}

func TestBalancerLowestLatencyUsesRuntimeStats(t *testing.T) {
	b := NewBalancer(BalancingLowestLatency, nil)
	connections := []*Connection{
		{Key: "a", IsValid: true},
		{Key: "b", IsValid: true},
	}
	b.SetConnections(connections)
	_ = b.SetConnectionValidity("a", true)
	_ = b.SetConnectionValidity("b", true)

	for i := 0; i < 6; i++ {
		b.ReportSend("a")
		b.ReportSuccess("a", 8*time.Millisecond)
		b.ReportSend("b")
		b.ReportSuccess("b", 2*time.Millisecond)
	}

	best, ok := b.GetBestConnection()
	if !ok {
		t.Fatal("expected best connection")
	}
	if best.Key != "b" {
		t.Fatalf("expected lower-latency resolver b, got %q", best.Key)
	}
}

func TestBalancerStatsHalfLifeAlsoAppliesOnSend(t *testing.T) {
	b := NewBalancer(BalancingLeastLoss, nil)
	connections := []*Connection{
		{Key: "a", IsValid: true},
	}
	b.SetConnections(connections)

	for i := 0; i < connectionStatsHalfLifeThreshold+1; i++ {
		b.ReportSend("a")
	}

	stats := b.statsForKey("a")
	if stats == nil {
		t.Fatal("expected stats for resolver a")
	}

	sent, acked, _, sum, count := stats.snapshot()
	if sent != (connectionStatsHalfLifeThreshold+1)/2 {
		t.Fatalf("expected send-triggered half-life to bound sent, got sent=%d acked=%d sum=%d count=%d", sent, acked, sum, count)
	}
	if acked != 0 || sum != 0 || count != 0 {
		t.Fatalf("expected send-triggered half-life to preserve zero success stats, got acked=%d sum=%d count=%d", acked, sum, count)
	}
}

func TestBalancerStatsHalfLifePreservesRelativeSuccessSignal(t *testing.T) {
	b := NewBalancer(BalancingLeastLoss, nil)
	connections := []*Connection{
		{Key: "a", IsValid: true},
	}
	b.SetConnections(connections)

	for i := 0; i < 800; i++ {
		b.ReportSend("a")
	}
	for i := 0; i < 400; i++ {
		b.ReportSuccess("a", 5*time.Millisecond)
	}
	for i := 0; i < 401; i++ {
		b.ReportSend("a")
	}

	stats := b.statsForKey("a")
	if stats == nil {
		t.Fatal("expected stats for resolver a")
	}

	sent, acked, _, sum, count := stats.snapshot()
	if sent != 700 || acked != 200 || count != 200 {
		t.Fatalf("expected balanced half-life after crossing threshold, got sent=%d acked=%d count=%d", sent, acked, count)
	}
	if sum != uint64(time.Millisecond/time.Microsecond)*5*200 {
		t.Fatalf("expected RTT signal to decay proportionally, got sum=%d", sum)
	}
}

func TestBalancerSetConnectionsCopiesSourceDomain(t *testing.T) {
	b := NewBalancer(BalancingRoundRobinDefault, nil)
	connections := []*Connection{
		{Key: "a", IsValid: true, Domain: "a.example.com"},
	}
	b.SetConnections(connections)

	connections[0].Domain = "mutated.example.com"

	got, ok := b.GetConnectionByKey("a")
	if !ok {
		t.Fatal("expected resolver a in balancer snapshot")
	}
	if got.Domain != "a.example.com" {
		t.Fatalf("expected balancer to keep copied domain after source mutation, got %q", got.Domain)
	}
}

func TestBalancerSetConnectionValidityDoesNotPullSourceMutation(t *testing.T) {
	b := NewBalancer(BalancingRoundRobinDefault, nil)
	connections := []*Connection{
		{Key: "a", IsValid: false, UploadMTUBytes: 140, DownloadMTUBytes: 220},
	}
	b.SetConnections(connections)

	connections[0].UploadMTUBytes = 90
	connections[0].DownloadMTUBytes = 180

	if !b.SetConnectionValidity("a", true) {
		t.Fatal("expected SetConnectionValidity to succeed")
	}

	got, ok := b.GetConnectionByKey("a")
	if !ok {
		t.Fatal("expected resolver a in snapshot")
	}
	if !got.IsValid {
		t.Fatal("expected resolver a to become valid")
	}
	if got.UploadMTUBytes != 0 || got.DownloadMTUBytes != 0 {
		t.Fatalf("expected balancer state to stay independent from source mutation, got up=%d down=%d", got.UploadMTUBytes, got.DownloadMTUBytes)
	}
}

func TestBalancerSetConnectionMTUUpdatesBalancerOnly(t *testing.T) {
	b := NewBalancer(BalancingRoundRobinDefault, nil)
	connections := []*Connection{
		{Key: "a", IsValid: true, UploadMTUBytes: 120, UploadMTUChars: 180, DownloadMTUBytes: 220},
	}
	b.SetConnections(connections)

	if !b.SetConnectionMTU("a", 90, 135, 180) {
		t.Fatal("expected SetConnectionMTU to succeed")
	}

	if connections[0].UploadMTUBytes != 120 || connections[0].UploadMTUChars != 180 || connections[0].DownloadMTUBytes != 220 {
		t.Fatalf("expected source MTUs to remain unchanged, got up=%d chars=%d down=%d", connections[0].UploadMTUBytes, connections[0].UploadMTUChars, connections[0].DownloadMTUBytes)
	}

	got, ok := b.GetConnectionByKey("a")
	if !ok {
		t.Fatal("expected resolver a in snapshot")
	}
	if got.UploadMTUBytes != 90 || got.UploadMTUChars != 135 || got.DownloadMTUBytes != 180 {
		t.Fatalf("expected snapshot MTUs to update, got up=%d chars=%d down=%d", got.UploadMTUBytes, got.UploadMTUChars, got.DownloadMTUBytes)
	}
}
