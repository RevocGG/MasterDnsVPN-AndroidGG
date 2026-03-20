// ==============================================================================
// MasterDnsVPN
// Author: MasterkinG32
// Github: https://github.com/masterking32
// Year: 2026
// ==============================================================================

package client

import (
	"context"
	"testing"
	"time"

	"masterdnsvpn-go/internal/config"
)

func TestResolverHealthAutoDisablesTimeoutOnlyConnection(t *testing.T) {
	cfg := config.ClientConfig{
		AutoDisableTimeoutServers:  true,
		AutoDisableTimeoutWindow:   10.0,
		AutoDisableMinObservations: 3,
		AutoDisableCheckInterval:   1.0,
	}
	c := New(cfg, nil, nil)
	c.connections = []Connection{
		{Key: "a", ResolverLabel: "1.1.1.1:53", IsValid: true},
		{Key: "b", ResolverLabel: "2.2.2.2:53", IsValid: true},
	}
	c.connectionsByKey = map[string]int{"a": 0, "b": 1}
	c.initResolverRecheckMeta()
	c.rebuildBalancer()

	base := time.Date(2026, 3, 19, 12, 0, 0, 0, time.UTC)
	current := base
	c.now = func() time.Time { return current }

	c.recordResolverHealthEvent("a", false, current)
	current = base.Add(5 * time.Second)
	c.recordResolverHealthEvent("a", false, current)
	current = base.Add(10 * time.Second)
	c.recordResolverHealthEvent("a", false, current)

	c.runResolverAutoDisable(current)

	if c.connections[0].IsValid {
		t.Fatal("expected timed out resolver to be disabled")
	}
	if c.Balancer().ValidCount() != 1 {
		t.Fatalf("unexpected valid count after disable: got=%d want=%d", c.Balancer().ValidCount(), 1)
	}
	if !c.isRuntimeDisabledResolver("a") {
		t.Fatal("expected disabled resolver to be tracked for runtime recheck")
	}
}

func TestResolverHealthRecheckReactivatesConnection(t *testing.T) {
	cfg := config.ClientConfig{
		RecheckInactiveEnabled:  true,
		RecheckInactiveInterval: 60.0,
		RecheckServerInterval:   3.0,
		RecheckBatchSize:        2,
	}
	c := New(cfg, nil, nil)
	c.successMTUChecks = true
	c.syncedUploadMTU = 120
	c.syncedDownloadMTU = 180
	c.connections = []Connection{
		{Key: "a", ResolverLabel: "1.1.1.1:53", IsValid: false},
		{Key: "b", ResolverLabel: "2.2.2.2:53", IsValid: true},
	}
	c.connectionsByKey = map[string]int{"a": 0, "b": 1}
	c.initResolverRecheckMeta()
	c.rebuildBalancer()
	c.recheckConnectionFn = func(conn *Connection) bool {
		return conn != nil && conn.Key == "a"
	}

	now := time.Date(2026, 3, 19, 12, 30, 0, 0, time.UTC)
	c.now = func() time.Time { return now }
	c.resolverHealthMu.Lock()
	c.resolverRecheck["a"] = resolverRecheckState{
		FailCount:   2,
		NextAt:      now.Add(-time.Second),
		WasValidOne: true,
	}
	c.runtimeDisabled["a"] = resolverDisabledState{
		DisabledAt:  now.Add(-time.Minute),
		NextRetryAt: now.Add(-time.Second),
		RetryCount:  2,
		Cause:       "timeout window",
	}
	c.resolverHealthMu.Unlock()

	c.runResolverRecheckBatch(context.Background(), now)

	if !c.connections[0].IsValid {
		t.Fatal("expected recheck to reactivate resolver")
	}
	if c.Balancer().ValidCount() != 2 {
		t.Fatalf("unexpected valid count after recheck: got=%d want=%d", c.Balancer().ValidCount(), 2)
	}
	if c.isRuntimeDisabledResolver("a") {
		t.Fatal("expected runtime disabled marker to be cleared after reactivation")
	}
}
