// ==============================================================================
// MasterDnsVPN
// Author: MasterkinG32
// Github: https://github.com/masterking32
// Year: 2026
// ==============================================================================

package udpserver

import (
	"errors"
	"net"
	"testing"
	"time"

	"masterdnsvpn-go/internal/config"
	Enums "masterdnsvpn-go/internal/enums"
	fragmentStore "masterdnsvpn-go/internal/fragmentstore"
	VpnProto "masterdnsvpn-go/internal/vpnproto"
)

func newTestServerForStreamSyn(protocol string) *Server {
	return &Server{
		cfg: config.ServerConfig{
			ProtocolType:                  protocol,
			ForwardIP:                     "127.0.0.1",
			ForwardPort:                   9000,
			StreamResultPacketTTLSeconds:  300.0,
			StreamFailurePacketTTLSeconds: 120.0,
			ARQWindowSize:                 64,
			ARQInitialRTOSeconds:          0.2,
			ARQMaxRTOSeconds:              1.0,
			ARQControlInitialRTOSeconds:   0.2,
			ARQControlMaxRTOSeconds:       1.0,
			ARQMaxControlRetries:          10,
			ARQInactivityTimeoutSeconds:   60.0,
			ARQDataPacketTTLSeconds:       60.0,
			ARQControlPacketTTLSeconds:    60.0,
			ARQMaxDataRetries:             100,
			ARQTerminalDrainTimeoutSec:    30.0,
			ARQTerminalAckWaitTimeoutSec:  10.0,
		},
		sessions:        newSessionStore(8, 32),
		dnsFragments:    fragmentStore.New[dnsFragmentKey](8),
		socks5Fragments: fragmentStore.New[socks5FragmentKey](8),
	}
}

func TestQueueImmediateControlAckCreatesStreamForStreamSyn(t *testing.T) {
	s := newTestServerForStreamSyn("TCP")
	record := newTestSessionRecord(21)
	record.DownloadCompression = 0
	s.sessions.byID[record.ID] = record

	packet := packetWithSession(Enums.PACKET_STREAM_SYN, record.ID, record.Cookie, 1)
	if !s.queueImmediateControlAck(record, packet) {
		t.Fatal("expected STREAM_SYN immediate ACK to be queued")
	}

	stream, ok := record.getStream(1)
	if !ok || stream == nil {
		t.Fatal("expected STREAM_SYN to create stream before queueing SYN_ACK")
	}

	key := Enums.PacketIdentityKey(stream.ID, Enums.PACKET_STREAM_SYN_ACK, packet.SequenceNum, packet.FragmentID)
	if _, ok := stream.TXQueue.Get(key); !ok {
		t.Fatal("expected STREAM_SYN_ACK to be queued on created stream")
	}
}

func TestProcessDeferredStreamSynQueuesConnectedAndEnablesIO(t *testing.T) {
	s := newTestServerForStreamSyn("TCP")
	record := newTestSessionRecord(22)
	record.DownloadCompression = 0
	s.sessions.byID[record.ID] = record

	local, remote := net.Pipe()
	defer remote.Close()

	s.dialStreamUpstreamFn = func(network string, address string, timeoutSeconds time.Duration) (net.Conn, error) {
		return local, nil
	}

	packet := packetWithSession(Enums.PACKET_STREAM_SYN, record.ID, record.Cookie, 2)
	s.processDeferredStreamSyn(packet)

	stream, ok := record.getStream(2)
	if !ok || stream == nil {
		t.Fatal("expected stream to exist after STREAM_SYN processing")
	}
	defer stream.Abort("test cleanup")

	stream.mu.RLock()
	connected := stream.Connected
	status := stream.Status
	stream.mu.RUnlock()
	if !connected {
		t.Fatal("expected stream to be marked connected")
	}
	if status != "CONNECTED" {
		t.Fatalf("expected stream status CONNECTED, got %q", status)
	}

	key := Enums.PacketIdentityKey(stream.ID, Enums.PACKET_STREAM_CONNECTED, packet.SequenceNum, 0)
	if pkt, ok := stream.TXQueue.Get(key); !ok || pkt == nil {
		t.Fatal("expected STREAM_CONNECTED to be queued after successful connect")
	}
}

func TestProcessDeferredStreamSynQueuesConnectFailOnDialError(t *testing.T) {
	s := newTestServerForStreamSyn("TCP")
	record := newTestSessionRecord(23)
	record.DownloadCompression = 0
	s.sessions.byID[record.ID] = record
	s.dialStreamUpstreamFn = func(network string, address string, timeout time.Duration) (net.Conn, error) {
		return nil, errors.New("dial failed")
	}

	packet := packetWithSession(Enums.PACKET_STREAM_SYN, record.ID, record.Cookie, 3)
	s.processDeferredStreamSyn(packet)

	stream, ok := record.getStream(3)
	if !ok || stream == nil {
		t.Fatal("expected stream to exist after failed STREAM_SYN processing")
	}
	defer stream.Abort("test cleanup")

	key := Enums.PacketIdentityKey(stream.ID, Enums.PACKET_STREAM_CONNECT_FAIL, packet.SequenceNum, 0)
	pkt, ok := stream.TXQueue.Get(key)
	if !ok || pkt == nil {
		t.Fatal("expected STREAM_CONNECT_FAIL to be queued after dial failure")
	}
	if pkt.TTL != s.cfg.StreamFailurePacketTTL() {
		t.Fatalf("unexpected STREAM_CONNECT_FAIL TTL: got=%s want=%s", pkt.TTL, s.cfg.StreamFailurePacketTTL())
	}
}

func TestHandlePostSessionPacketRejectsMismatchedSynProtocol(t *testing.T) {
	s := newTestServerForStreamSyn("SOCKS5")
	record := newTestSessionRecord(24)
	s.sessions.byID[record.ID] = record

	packet := packetWithSession(Enums.PACKET_STREAM_SYN, record.ID, record.Cookie, 4)
	if handled := s.handlePostSessionPacket(packet, viewForRecord(record)); handled {
		t.Fatal("expected TCP STREAM_SYN to be rejected when server protocol is SOCKS5")
	}
	if _, ok := record.getStream(4); ok {
		t.Fatal("expected mismatched STREAM_SYN to be ignored without creating stream")
	}
}

func TestValidateSOCKSTargetHostRejectsLocalAndPrivateTargets(t *testing.T) {
	cases := []string{
		"127.0.0.1",
		"localhost",
		"api.localhost",
		"10.0.0.5",
		"172.16.1.9",
		"192.168.1.10",
		"169.254.1.1",
		"100.64.0.1",
		"198.18.0.1",
		"::1",
		"fe80::1",
		"fc00::1",
	}

	for _, host := range cases {
		if err := validateSOCKSTargetHost(host); err == nil {
			t.Fatalf("expected host %q to be rejected", host)
		}
	}
}

func TestValidateSOCKSTargetHostAllowsPublicTargets(t *testing.T) {
	cases := []string{
		"149.154.167.255",
		"8.8.8.8",
		"example.com",
	}

	for _, host := range cases {
		if err := validateSOCKSTargetHost(host); err != nil {
			t.Fatalf("expected host %q to be allowed, got %v", host, err)
		}
	}
}

func TestDialSOCKSStreamTargetRejectsBlockedTargetBeforeDial(t *testing.T) {
	s := newTestServerForStreamSyn("SOCKS5")
	s.useExternalSOCKS5 = true
	s.dialStreamUpstreamFn = func(network string, address string, timeout time.Duration) (net.Conn, error) {
		t.Fatalf("unexpected dial attempt to %s", address)
		return nil, nil
	}

	if _, err := s.dialSOCKSStreamTarget("127.0.0.1", 80, []byte{0x01, 0x7f, 0x00, 0x00, 0x01, 0x00, 0x50}); err == nil {
		t.Fatal("expected blocked target error")
	}
}

func TestMapSOCKSConnectErrorMapsBlockedTargetToRulesetDenied(t *testing.T) {
	s := newTestServerForStreamSyn("SOCKS5")
	if got := s.mapSOCKSConnectError(&blockedSOCKSTargetError{host: "127.0.0.1"}); got != Enums.PACKET_SOCKS5_RULESET_DENIED {
		t.Fatalf("unexpected packet type: got=%d want=%d", got, Enums.PACKET_SOCKS5_RULESET_DENIED)
	}
}

func packetWithSession(packetType uint8, sessionID uint8, cookie uint8, streamID uint16) VpnProto.Packet {
	return VpnProto.Packet{
		SessionID:      sessionID,
		SessionCookie:  cookie,
		PacketType:     packetType,
		StreamID:       streamID,
		HasStreamID:    true,
		SequenceNum:    1,
		HasSequenceNum: true,
	}
}

func viewForRecord(record *sessionRecord) *sessionRuntimeView {
	if record == nil {
		return nil
	}
	view := record.runtimeView()
	return &view
}
