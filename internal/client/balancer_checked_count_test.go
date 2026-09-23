package client

import (
	"testing"
	"time"
)

func TestCheckedCountProgressesWithRejectedProbes(t *testing.T) {
	b := NewBalancer(0, nil)
	b.SetConnections([]*Connection{
		{Domain: "d1", Resolver: "10.0.0.1", ResolverPort: 53, Key: "k1"},
		{Domain: "d1", Resolver: "10.0.0.2", ResolverPort: 53, Key: "k2"},
		{Domain: "d1", Resolver: "10.0.0.3", ResolverPort: 53, Key: "k3"},
	})
	if got := b.CheckedCount(); got != 0 {
		t.Fatalf("fresh balancer: got %d want 0", got)
	}
	// One rejected probe (invalid, resolveTime zero — buildMTUDecision zeroes it)
	b.ApplyMTUProbeResult("k1", 0, 0, 0, 0, false)
	// One accepted probe (valid)
	b.ApplyMTUProbeResult("k2", 60, 60, 600, 30*time.Millisecond, true)
	if got := b.CheckedCount(); got != 2 {
		t.Fatalf("after 1 reject + 1 accept: got %d want 2", got)
	}
	// Second rejected probe — now invalid with prior MTU data
	b.ApplyMTUProbeResult("k1", 0, 0, 0, 0, false)
	if got := b.CheckedCount(); got != 2 {
		t.Fatalf("re-probe same conn: got %d want 2", got)
	}
}
