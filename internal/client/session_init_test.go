package client

import (
	"testing"

	"masterdnsvpn-go/internal/config"
)

func TestNextSessionInitAttemptUsesBalancerSnapshotConnection(t *testing.T) {
	c := buildTestClientWithResolvers(config.ClientConfig{}, "a", "b")
	connections := []*Connection{
		{Key: "a", Domain: "a.example.com", Resolver: "127.0.0.1", ResolverPort: 5300, ResolverLabel: "127.0.0.1:0"},
		{Key: "b", Domain: "b.example.com", Resolver: "127.0.0.1", ResolverPort: 5301, ResolverLabel: "127.0.0.1:1"},
	}
	c.balancer.SetConnections(connections)
	for _, conn := range connections {
		c.balancer.SetConnectionMTU(conn.Key, 120, 180, 220)
		c.balancer.SetConnectionValidity(conn.Key, true)
	}

	originalDomain := connections[0].Domain
	connections[0].Domain = "mutated.example.com"

	conn, _, _, err := c.nextSessionInitAttempt()
	if err != nil {
		t.Fatalf("nextSessionInitAttempt returned error: %v", err)
	}

	if conn.Domain != originalDomain {
		t.Fatalf("expected session init to use balancer snapshot domain %q, got %q", originalDomain, conn.Domain)
	}
}
