// ==============================================================================
// MasterDnsVPN
// Author: MasterkinG32
// Github: https://github.com/masterking32
// Year: 2026
// ==============================================================================

package client

import (
	"time"

	Enums "masterdnsvpn-go/internal/enums"
)

func (c *Client) runtimePacketDuplicationCount(packetType uint8) int {
	if c == nil {
		return 1
	}
	count := c.packetDuplicationCount
	if count < 1 {
		count = 1
	}
	if packetType == Enums.PACKET_STREAM_SYN || packetType == Enums.PACKET_SOCKS5_SYN {
		if c.setupPacketDuplicationCount > count {
			count = c.setupPacketDuplicationCount
		}
	}
	return count
}

func (c *Client) selectTargetConnectionsForPacket(packetType uint8, streamID uint16) ([]Connection, error) {
	targetCount := c.runtimePacketDuplicationCount(packetType)
	if streamID == 0 || c == nil || c.Balancer().ValidCount() <= 0 {
		return c.selectUniqueRuntimeConnections(targetCount)
	}
	if packetType != Enums.PACKET_STREAM_DATA && packetType != Enums.PACKET_STREAM_RESEND {
		return c.selectUniqueRuntimeConnections(targetCount)
	}

	stream, ok := c.getStream(streamID)
	if !ok || stream == nil {
		return c.selectUniqueRuntimeConnections(targetCount)
	}

	var (
		preferred Connection
		found     bool
	)
	if packetType == Enums.PACKET_STREAM_RESEND {
		preferred, found = c.selectStreamPreferredConnectionForResend(stream)
	} else {
		preferred, found = c.ensureStreamPreferredConnection(stream)
	}
	if !found {
		return c.selectUniqueRuntimeConnections(targetCount)
	}
	if targetCount <= 1 {
		return []Connection{preferred}, nil
	}

	selected := make([]Connection, 0, targetCount)
	selected = append(selected, preferred)
	seenKeys := map[string]struct{}{preferred.Key: {}}

	for _, connection := range c.GetUniqueConnections(targetCount) {
		if !connection.IsValid || connection.Key == "" {
			continue
		}
		if _, exists := seenKeys[connection.Key]; exists {
			continue
		}
		selected = append(selected, connection)
		seenKeys[connection.Key] = struct{}{}
		if len(selected) >= targetCount {
			return selected, nil
		}
	}

	for _, connection := range c.Balancer().GetAllValidConnections() {
		if !connection.IsValid || connection.Key == "" {
			continue
		}
		if _, exists := seenKeys[connection.Key]; exists {
			continue
		}
		selected = append(selected, connection)
		seenKeys[connection.Key] = struct{}{}
		if len(selected) >= targetCount {
			break
		}
	}

	if len(selected) == 0 {
		return nil, ErrNoValidConnections
	}
	return selected, nil
}

func (c *Client) selectUniqueRuntimeConnections(requiredCount int) ([]Connection, error) {
	if c == nil {
		return nil, ErrNoValidConnections
	}
	connections := c.GetUniqueConnections(requiredCount)
	if len(connections) == 0 {
		return nil, ErrNoValidConnections
	}
	return connections, nil
}

func (c *Client) selectStreamPreferredConnectionForResend(stream *clientStream) (Connection, bool) {
	if c == nil || stream == nil {
		return Connection{}, false
	}

	stream.mu.Lock()
	stream.ResolverResendStreak++
	streak := stream.ResolverResendStreak
	stream.mu.Unlock()

	if streak >= c.streamResolverFailoverResendThreshold {
		return c.maybeFailoverStreamPreferredConnection(stream)
	}
	return c.ensureStreamPreferredConnection(stream)
}

func (c *Client) getValidStreamPreferredConnection(stream *clientStream) (Connection, bool) {
	if c == nil || stream == nil {
		return Connection{}, false
	}
	stream.mu.Lock()
	preferredKey := stream.PreferredServerKey
	stream.mu.Unlock()
	if preferredKey == "" {
		return Connection{}, false
	}
	connection, ok := c.GetConnectionByKey(preferredKey)
	if !ok || !connection.IsValid {
		return Connection{}, false
	}
	return connection, true
}

func (c *Client) selectAlternateStreamConnection(excludeKey string) (Connection, bool) {
	if c == nil {
		return Connection{}, false
	}
	blockedKey := excludeKey
	for _, connection := range c.Balancer().GetAllValidConnections() {
		if !connection.IsValid || connection.Key == "" {
			continue
		}
		if blockedKey != "" && connection.Key == blockedKey {
			continue
		}
		return connection, true
	}
	return Connection{}, false
}

func (c *Client) setStreamPreferredConnection(stream *clientStream, connection Connection) (Connection, bool) {
	if c == nil || stream == nil {
		return Connection{}, false
	}
	if !connection.IsValid || connection.Key == "" {
		stream.mu.Lock()
		stream.PreferredServerKey = ""
		stream.mu.Unlock()
		return Connection{}, false
	}

	now := time.Now()
	stream.mu.Lock()
	stream.PreferredServerKey = connection.Key
	stream.ResolverResendStreak = 0
	stream.LastResolverFailover = now
	stream.mu.Unlock()
	return connection, true
}

func (c *Client) ensureStreamPreferredConnection(stream *clientStream) (Connection, bool) {
	if preferred, ok := c.getValidStreamPreferredConnection(stream); ok {
		return preferred, true
	}
	fallback, ok := c.GetBestConnection()
	if !ok {
		return Connection{}, false
	}
	return c.setStreamPreferredConnection(stream, fallback)
}

func (c *Client) maybeFailoverStreamPreferredConnection(stream *clientStream) (Connection, bool) {
	current, ok := c.getValidStreamPreferredConnection(stream)
	if !ok {
		return c.ensureStreamPreferredConnection(stream)
	}

	stream.mu.Lock()
	lastSwitch := stream.LastResolverFailover
	stream.mu.Unlock()
	if !lastSwitch.IsZero() && time.Since(lastSwitch) < c.streamResolverFailoverCooldown {
		return current, true
	}

	replacement, ok := c.selectAlternateStreamConnection(current.Key)
	if !ok {
		return current, true
	}
	return c.setStreamPreferredConnection(stream, replacement)
}

func (c *Client) noteStreamProgress(streamID uint16) {
	if c == nil || streamID == 0 {
		return
	}
	stream, ok := c.getStream(streamID)
	if !ok || stream == nil {
		return
	}
	stream.mu.Lock()
	stream.ResolverResendStreak = 0
	stream.mu.Unlock()
}
