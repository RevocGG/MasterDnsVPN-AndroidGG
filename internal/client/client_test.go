// ==============================================================================
// MasterDnsVPN
// Author: MasterkinG32
// Github: https://github.com/masterking32
// Year: 2026
// ==============================================================================

package client

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"masterdnsvpn-go/internal/arq"
	"masterdnsvpn-go/internal/compression"
	"masterdnsvpn-go/internal/config"
	"masterdnsvpn-go/internal/dnscache"
	DnsParser "masterdnsvpn-go/internal/dnsparser"
	Enums "masterdnsvpn-go/internal/enums"
	"masterdnsvpn-go/internal/security"
	SocksProto "masterdnsvpn-go/internal/socksproto"
	VpnProto "masterdnsvpn-go/internal/vpnproto"
)

type noopConn struct{}

func (n *noopConn) Read(b []byte) (int, error)       { return 0, io.EOF }
func (n *noopConn) Write(b []byte) (int, error)      { return len(b), nil }
func (n *noopConn) Close() error                     { return nil }
func (n *noopConn) LocalAddr() net.Addr              { return nil }
func (n *noopConn) RemoteAddr() net.Addr             { return nil }
func (n *noopConn) SetDeadline(time.Time) error      { return nil }
func (n *noopConn) SetReadDeadline(time.Time) error  { return nil }
func (n *noopConn) SetWriteDeadline(time.Time) error { return nil }

func TestBuildConnectionMap(t *testing.T) {
	cfg := config.ClientConfig{
		ProtocolType: "SOCKS5",
		Domains: []string{
			"a.example.com",
			"b.example.com",
		},
		Resolvers: []config.ResolverAddress{
			{IP: "8.8.8.8", Port: 53},
			{IP: "2001:4860:4860::8888", Port: 5353},
		},
	}

	c := New(cfg, nil, nil)
	c.BuildConnectionMap()

	if got, want := len(c.Connections()), 4; got != want {
		t.Fatalf("unexpected connection count: got=%d want=%d", got, want)
	}

	first := c.Connections()[0]
	if first.Domain == "" || first.Resolver == "" || first.Key == "" {
		t.Fatalf("connection fields should be populated: %+v", first)
	}
	if !first.IsValid {
		t.Fatalf("connections should start valid")
	}
	if first.Resolver == "2001:4860:4860::8888" && first.ResolverLabel != "[2001:4860:4860::8888]:5353" {
		t.Fatalf("unexpected ipv6 resolver label: got=%q", first.ResolverLabel)
	}
	if c.Balancer().ValidCount() != 4 {
		t.Fatalf("unexpected valid connection count: got=%d want=%d", c.Balancer().ValidCount(), 4)
	}
}

func TestResetRuntimeState(t *testing.T) {
	c := New(config.ClientConfig{}, nil, nil)
	c.sessionReady = true
	c.sessionID = 11
	c.sessionCookie = 22
	c.enqueueSeq = 33

	c.ResetRuntimeState(false)
	if c.sessionID != 0 || c.enqueueSeq != 0 || c.sessionReady {
		t.Fatalf("reset should clear session state and enqueue seq: sid=%d enqueue=%d ready=%t", c.sessionID, c.enqueueSeq, c.sessionReady)
	}
	if c.sessionCookie != 22 {
		t.Fatalf("reset without cookie reset should preserve session cookie: got=%d", c.sessionCookie)
	}

	c.ResetRuntimeState(true)
	if c.sessionCookie != 0 {
		t.Fatalf("reset with cookie reset should clear session cookie: got=%d", c.sessionCookie)
	}
}

func TestSetConnectionValidityKeepsClientAndBalancerInSync(t *testing.T) {
	cfg := config.ClientConfig{
		Domains: []string{"a.example.com"},
		Resolvers: []config.ResolverAddress{
			{IP: "8.8.8.8", Port: 53},
		},
	}

	c := New(cfg, nil, nil)
	c.BuildConnectionMap()
	key := c.Connections()[0].Key

	if !c.SetConnectionValidity(key, false) {
		t.Fatal("SetConnectionValidity returned false")
	}
	if c.Connections()[0].IsValid {
		t.Fatal("client connection validity was not updated")
	}
	if got := c.Balancer().ValidCount(); got != 0 {
		t.Fatalf("unexpected valid count after disable: got=%d want=0", got)
	}

	if !c.SetConnectionValidity(key, true) {
		t.Fatal("SetConnectionValidity returned false when re-enabling")
	}
	if !c.Connections()[0].IsValid {
		t.Fatal("client connection validity was not restored")
	}
	if got := c.Balancer().ValidCount(); got != 1 {
		t.Fatalf("unexpected valid count after enable: got=%d want=1", got)
	}
}

func TestBuildSessionInitPayloadLayout(t *testing.T) {
	c := New(config.ClientConfig{
		BaseEncodeData:          true,
		UploadCompressionType:   2,
		DownloadCompressionType: 1,
	}, nil, nil)
	c.syncedUploadMTU = 150
	c.syncedDownloadMTU = 200

	payload, useBase64, verifyCode, err := c.buildSessionInitPayload()
	if err != nil {
		t.Fatalf("buildSessionInitPayload returned error: %v", err)
	}
	if !useBase64 {
		t.Fatal("expected base64 response mode")
	}
	if len(payload) != 10 {
		t.Fatalf("unexpected payload len: got=%d want=10", len(payload))
	}
	if payload[0] != 1 {
		t.Fatalf("unexpected response mode byte: got=%d want=1", payload[0])
	}
	if payload[1] != 0x21 {
		t.Fatalf("unexpected compression pair: got=%#x want=%#x", payload[1], 0x21)
	}
	if got := int(binary.BigEndian.Uint16(payload[2:4])); got != 150 {
		t.Fatalf("unexpected upload mtu: got=%d want=150", got)
	}
	if got := int(binary.BigEndian.Uint16(payload[4:6])); got != 200 {
		t.Fatalf("unexpected download mtu: got=%d want=200", got)
	}
	if string(payload[6:10]) != string(verifyCode[:]) {
		t.Fatalf("unexpected verify code bytes: got=%v want=%v", payload[6:10], verifyCode)
	}
}

func TestValidateServerPacketAllowsPreSessionResponses(t *testing.T) {
	c := New(config.ClientConfig{}, nil, nil)
	if !c.validateServerPacket(VpnProto.Packet{PacketType: Enums.PACKET_MTU_UP_RES}) {
		t.Fatal("pre-session mtu-up response should be accepted")
	}
	if !c.validateServerPacket(VpnProto.Packet{PacketType: Enums.PACKET_MTU_DOWN_RES}) {
		t.Fatal("pre-session mtu-down response should be accepted")
	}
	if !c.validateServerPacket(VpnProto.Packet{PacketType: Enums.PACKET_SESSION_ACCEPT}) {
		t.Fatal("pre-session session-accept should be accepted")
	}
	if !c.validateServerPacket(VpnProto.Packet{PacketType: Enums.PACKET_SESSION_BUSY}) {
		t.Fatal("pre-session session-busy response should be accepted")
	}
	if !c.validateServerPacket(VpnProto.Packet{PacketType: Enums.PACKET_ERROR_DROP}) {
		t.Fatal("pre-session error-drop response should be accepted")
	}
}

func TestInitializeSessionReturnsBusyForMatchingVerifyCode(t *testing.T) {
	codec, err := security.NewCodec(0, "")
	if err != nil {
		t.Fatalf("NewCodec returned error: %v", err)
	}
	c := New(config.ClientConfig{
		Domains: []string{"a.com", "b.com"},
		Resolvers: []config.ResolverAddress{
			{IP: "1.1.1.1", Port: 53},
			{IP: "8.8.8.8", Port: 53},
		},
		MTUTestTimeout: 1,
	}, nil, codec)
	c.BuildConnectionMap()
	c.applySyncedMTUState(150, 200, 220)

	callCount := 0
	c.exchangeQueryFn = func(conn Connection, packet []byte, timeout time.Duration) ([]byte, error) {
		callCount++
		response, err := DnsParser.BuildVPNResponsePacket(packet, conn.Domain, VpnProto.Packet{
			SessionID:  0,
			PacketType: Enums.PACKET_SESSION_BUSY,
			Payload:    c.sessionInitVerify[:],
		}, c.sessionInitBase64)
		if err != nil {
			t.Fatalf("BuildVPNResponsePacket returned error: %v", err)
		}
		return response, nil
	}

	err = c.InitializeSession(1)
	if !errors.Is(err, ErrSessionInitBusy) {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 1 {
		t.Fatalf("expected one init attempt, got=%d", callCount)
	}
	if until := c.sessionInitBusyUntil(); until.IsZero() || time.Until(until) <= 0 {
		t.Fatal("expected session busy deadline to be set")
	}
}

func TestHandleServerDropPacketResetsRuntimeOnce(t *testing.T) {
	c := New(config.ClientConfig{}, nil, nil)
	stream := &clientStream{
		ID:     9,
		Conn:   &noopConn{},
		TXWake: make(chan struct{}, 1),
		StopCh: make(chan struct{}),
	}
	c.storeStream(stream)
	c.sessionID = 7
	c.sessionCookie = 99
	c.sessionReady = true
	c.sessionInitPayload = []byte{1, 2, 3}
	c.sessionInitReady = true

	packet := VpnProto.Packet{PacketType: Enums.PACKET_ERROR_DROP, SessionID: 7}
	if err := c.handleServerDropPacket(packet); !errors.Is(err, ErrSessionDropped) {
		t.Fatalf("unexpected first drop error: %v", err)
	}
	if c.SessionReady() {
		t.Fatal("session should be cleared after drop")
	}
	if c.sessionInitReady || len(c.sessionInitPayload) != 0 {
		t.Fatal("session init state should be reset on first drop")
	}
	if !c.reconnectPending.Load() {
		t.Fatal("reconnect should be pending after drop")
	}
	if _, ok := c.getStream(stream.ID); ok {
		t.Fatal("active stream should be cleared after drop")
	}

	c.sessionInitPayload = []byte{9}
	c.sessionInitReady = true
	if err := c.handleServerDropPacket(packet); err != nil {
		t.Fatalf("unexpected repeated drop error: %v", err)
	}
	if !c.sessionInitReady || len(c.sessionInitPayload) != 1 {
		t.Fatal("repeated delayed drop should not reset init state again")
	}
}

func TestValidateServerPacketRequiresMatchingSessionCookie(t *testing.T) {
	c := New(config.ClientConfig{}, nil, nil)
	c.sessionReady = true
	c.sessionID = 7
	c.sessionCookie = 55

	valid := VpnProto.Packet{
		SessionID:     7,
		SessionCookie: 55,
		PacketType:    Enums.PACKET_PONG,
	}
	if !c.validateServerPacket(valid) {
		t.Fatal("matching session packet should be accepted")
	}

	wrongCookie := valid
	wrongCookie.SessionCookie = 66
	if c.validateServerPacket(wrongCookie) {
		t.Fatal("packet with wrong session cookie should be rejected")
	}

	wrongSession := valid
	wrongSession.SessionID = 8
	if c.validateServerPacket(wrongSession) {
		t.Fatal("packet with wrong session id should be rejected")
	}
}

func TestApplySessionCompressionPolicyDisablesSmallMTUDirections(t *testing.T) {
	c := New(config.ClientConfig{
		CompressionMinSize: compression.DefaultMinSize,
	}, nil, nil)
	c.syncedUploadMTU = compression.DefaultMinSize
	c.syncedDownloadMTU = compression.DefaultMinSize - 1
	c.uploadCompression = compression.TypeZLIB
	c.downloadCompression = compression.TypeZLIB

	c.applySessionCompressionPolicy()

	if c.uploadCompression != compression.TypeOff {
		t.Fatalf("upload compression should be disabled, got=%d", c.uploadCompression)
	}
	if c.downloadCompression != compression.TypeOff {
		t.Fatalf("download compression should be disabled, got=%d", c.downloadCompression)
	}
}

func TestApplySessionCompressionPolicyKeepsLargeMTUDirections(t *testing.T) {
	c := New(config.ClientConfig{
		CompressionMinSize: compression.DefaultMinSize,
	}, nil, nil)
	c.syncedUploadMTU = compression.DefaultMinSize + 1
	c.syncedDownloadMTU = compression.DefaultMinSize + 50
	c.uploadCompression = compression.TypeZLIB
	c.downloadCompression = compression.TypeOff

	c.applySessionCompressionPolicy()

	if c.uploadCompression != compression.TypeZLIB {
		t.Fatalf("upload compression should stay enabled, got=%d", c.uploadCompression)
	}
	if c.downloadCompression != compression.TypeOff {
		t.Fatalf("download compression should stay off, got=%d", c.downloadCompression)
	}
}

func TestNewKeepsLocalDNSDefaults(t *testing.T) {
	c := New(config.ClientConfig{
		LocalDNSEnabled:   true,
		LocalDNSIP:        "127.0.0.1",
		LocalDNSPort:      5353,
		LocalDNSWorkers:   2,
		LocalDNSQueueSize: 512,
	}, nil, nil)

	if !c.cfg.LocalDNSEnabled {
		t.Fatal("expected local dns listener to stay enabled in config")
	}
	if c.cfg.LocalDNSIP != "127.0.0.1" || c.cfg.LocalDNSPort != 5353 {
		t.Fatalf("unexpected local dns bind config: %s:%d", c.cfg.LocalDNSIP, c.cfg.LocalDNSPort)
	}
}

func TestHandleDNSQueryPacketCreatesPendingEntry(t *testing.T) {
	c := New(config.ClientConfig{
		LocalDNSCacheMaxRecords:   8,
		LocalDNSCacheTTLSeconds:   60,
		LocalDNSPendingTimeoutSec: 30,
	}, nil, nil)
	now := time.Unix(1700000000, 0)
	c.now = func() time.Time { return now }

	query := buildClientTestDNSQuery(0x1234, "example.com", Enums.DNS_RECORD_TYPE_A, Enums.DNSQ_CLASS_IN)
	response, dispatch := c.handleDNSQueryPacket(query)
	if dispatch == nil {
		t.Fatal("expected pending dispatch request")
	}
	if len(response) == 0 {
		t.Fatal("expected temporary servfail response")
	}
	cacheKey := dnscache.BuildKey("example.com", Enums.DNS_RECORD_TYPE_A, Enums.DNSQ_CLASS_IN)
	entry, ok := c.LocalDNSCache().Snapshot(cacheKey)
	if !ok {
		t.Fatal("expected cache entry to be created")
	}
	if entry.Status != dnscache.StatusPending {
		t.Fatalf("expected pending cache status, got=%d", entry.Status)
	}
}

func TestHandleDNSQueryPacketUsesReadyCache(t *testing.T) {
	c := New(config.ClientConfig{
		LocalDNSCacheMaxRecords:   8,
		LocalDNSCacheTTLSeconds:   60,
		LocalDNSPendingTimeoutSec: 30,
	}, nil, nil)
	now := time.Unix(1700000000, 0)
	c.now = func() time.Time { return now }

	query := buildClientTestDNSQuery(0x1234, "example.com", Enums.DNS_RECORD_TYPE_A, Enums.DNSQ_CLASS_IN)
	cacheKey := dnscache.BuildKey("example.com", Enums.DNS_RECORD_TYPE_A, Enums.DNSQ_CLASS_IN)
	rawResponse := []byte{
		0x00, 0x00, 0x81, 0x80, 0x00, 0x01, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
	}
	rawResponse = append(rawResponse, encodeClientTestDNSName("example.com")...)
	rawResponse = append(rawResponse, 0x00, byte(Enums.DNS_RECORD_TYPE_A), 0x00, byte(Enums.DNSQ_CLASS_IN))
	c.LocalDNSCache().SetReady(cacheKey, "example.com", Enums.DNS_RECORD_TYPE_A, Enums.DNSQ_CLASS_IN, rawResponse, now)

	response, dispatch := c.handleDNSQueryPacket(query)
	if dispatch != nil {
		t.Fatal("did not expect dispatch for ready cache hit")
	}
	if len(response) < 2 {
		t.Fatal("expected cached response")
	}
	if binary.BigEndian.Uint16(response[:2]) != 0x1234 {
		t.Fatalf("expected patched response id, got=%#x", binary.BigEndian.Uint16(response[:2]))
	}
}

func TestHandleDNSQueryPacketRejectsUnsupportedQueryType(t *testing.T) {
	c := New(config.ClientConfig{}, nil, nil)
	query := buildClientTestDNSQuery(0x1234, "example.com", Enums.DNS_RECORD_TYPE_ANY, Enums.DNSQ_CLASS_IN)

	response, dispatch := c.handleDNSQueryPacket(query)
	if dispatch != nil {
		t.Fatal("unsupported query should not dispatch")
	}
	if len(response) < 4 {
		t.Fatal("expected not-implemented response")
	}
	if got := binary.BigEndian.Uint16(response[2:4]) & 0x000F; got != Enums.DNSR_CODE_NOT_IMPLEMENTED {
		t.Fatalf("unexpected rcode: got=%d want=%d", got, Enums.DNSR_CODE_NOT_IMPLEMENTED)
	}
}

func TestResolveDNSQueryPacketDedupesPendingDispatch(t *testing.T) {
	codec, err := security.NewCodec(0, "")
	if err != nil {
		t.Fatalf("NewCodec returned error: %v", err)
	}

	c := New(config.ClientConfig{
		LocalDNSPendingTimeoutSec: 1,
		Domains:                   []string{"v.example.com"},
	}, nil, codec)
	now := time.Unix(1700000000, 0)
	c.now = func() time.Time { return now }
	c.connections = []Connection{{
		Domain:        "v.example.com",
		Resolver:      "127.0.0.1",
		ResolverPort:  5353,
		ResolverLabel: "127.0.0.1:5353",
		Key:           "127.0.0.1|5353|v.example.com",
		IsValid:       true,
	}}
	c.connectionsByKey = map[string]int{c.connections[0].Key: 0}
	c.rebuildBalancer()
	c.sessionID = 7
	c.sessionCookie = 9
	c.sessionReady = true
	c.responseMode = mtuProbeRawResponse

	query := buildClientTestDNSQuery(0x1234, "example.com", Enums.DNS_RECORD_TYPE_A, Enums.DNSQ_CLASS_IN)
	expectedFallback, err := DnsParser.BuildServerFailureResponse(query)
	if err != nil {
		t.Fatalf("BuildServerFailureResponse returned error: %v", err)
	}

	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var callCount int
	c.exchangeQueryFn = func(conn Connection, packet []byte, timeout time.Duration) ([]byte, error) {
		callCount++
		if callCount == 1 {
			started <- struct{}{}
			<-release
		}
		queryPacket, err := DnsParser.ParsePacketLite(packet)
		if err != nil || !queryPacket.HasQuestion {
			t.Fatalf("unexpected tunnel dns query: err=%v", err)
		}
		vpnPacket, err := VpnProto.ParseFromLabels(extractTestTunnelLabels(queryPacket.FirstQuestion.Name, "v.example.com"), c.codec)
		if err != nil {
			t.Fatalf("ParseFromLabels returned error: %v", err)
		}
		return DnsParser.BuildVPNResponsePacket(packet, queryPacket.FirstQuestion.Name, VpnProto.Packet{
			SessionID:      c.sessionID,
			SessionCookie:  c.sessionCookie,
			PacketType:     Enums.PACKET_DNS_QUERY_REQ_ACK,
			StreamID:       0,
			SequenceNum:    vpnPacket.SequenceNum,
			FragmentID:     0,
			TotalFragments: 1,
		}, false)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := c.startStream0Runtime(ctx); err != nil {
		t.Fatalf("startStream0Runtime returned error: %v", err)
	}

	results := make(chan []byte, 2)
	go func() { results <- c.resolveDNSQueryPacket(query) }()
	<-started
	go func() { results <- c.resolveDNSQueryPacket(query) }()

	response1 := <-results
	response2 := <-results
	if string(response1) != string(expectedFallback) || string(response2) != string(expectedFallback) {
		t.Fatal("expected both queries to return the same immediate fallback response")
	}

	close(release)
	deadline := time.Now().Add(2 * time.Second)
	for callCount < 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if callCount != 1 {
		t.Fatalf("expected one tunnel dispatch, got=%d", callCount)
	}
}

func TestHandleDNSQueryPacketRejectsMalformedQuery(t *testing.T) {
	c := New(config.ClientConfig{}, nil, nil)
	response, dispatch := c.handleDNSQueryPacket([]byte{0x12, 0x34, 0x00})
	if dispatch != nil {
		t.Fatal("did not expect dispatch for malformed query")
	}
	if response != nil {
		t.Fatal("short non-dns packet should be ignored")
	}

	query := buildClientTestDNSQuery(0x1234, "example.com", Enums.DNS_RECORD_TYPE_A, Enums.DNSQ_CLASS_IN)
	query = query[:len(query)-2]
	response, dispatch = c.handleDNSQueryPacket(query)
	if dispatch != nil {
		t.Fatal("did not expect dispatch for malformed dns query")
	}
	if len(response) == 0 {
		t.Fatal("expected format error response for malformed dns query")
	}
}

func TestHandleInboundDNSResponseFragmentCachesReadyTunnelResponse(t *testing.T) {
	c := New(config.ClientConfig{
		LocalDNSCacheMaxRecords: 8,
		LocalDNSCacheTTLSeconds: 60,
	}, nil, nil)
	now := time.Unix(1700000000, 0)
	c.now = func() time.Time { return now }

	rawQuery := buildClientTestDNSQuery(0x1234, "example.com", Enums.DNS_RECORD_TYPE_A, Enums.DNSQ_CLASS_IN)
	rawResponse := []byte{
		0x00, 0x00, 0x81, 0x80, 0x00, 0x01, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
	}
	rawResponse = append(rawResponse, encodeClientTestDNSName("example.com")...)
	rawResponse = append(rawResponse, 0x00, byte(Enums.DNS_RECORD_TYPE_A), 0x00, byte(Enums.DNSQ_CLASS_IN))

	firstHalf := rawResponse[:len(rawResponse)/2]
	secondHalf := rawResponse[len(rawResponse)/2:]

	if err := c.handleInboundDNSResponseFragment(VpnProto.Packet{
		SessionID:      7,
		PacketType:     Enums.PACKET_DNS_QUERY_RES,
		SequenceNum:    41,
		HasSequenceNum: true,
		FragmentID:     0,
		TotalFragments: 2,
		Payload:        firstHalf,
	}); err != nil {
		t.Fatalf("handleInboundDNSResponseFragment returned error: %v", err)
	}

	cacheKey := dnscache.BuildKey("example.com", Enums.DNS_RECORD_TYPE_A, Enums.DNSQ_CLASS_IN)
	if _, ok := c.LocalDNSCache().GetReady(cacheKey, rawQuery, now); ok {
		t.Fatal("response must not be cached before all fragments arrive")
	}

	if err := c.handleInboundDNSResponseFragment(VpnProto.Packet{
		SessionID:      7,
		PacketType:     Enums.PACKET_DNS_QUERY_RES,
		SequenceNum:    41,
		HasSequenceNum: true,
		FragmentID:     1,
		TotalFragments: 2,
		Payload:        secondHalf,
	}); err != nil {
		t.Fatalf("handleInboundDNSResponseFragment returned error: %v", err)
	}

	cached, ok := c.LocalDNSCache().GetReady(cacheKey, rawQuery, now)
	if !ok {
		t.Fatal("expected assembled dns response to be cached")
	}
	if binary.BigEndian.Uint16(cached[:2]) != 0x1234 {
		t.Fatalf("expected cached response id to be patched, got=%#x", binary.BigEndian.Uint16(cached[:2]))
	}
}

func TestHandleInboundDNSResponseFragmentDoesNotCacheServerFailures(t *testing.T) {
	c := New(config.ClientConfig{
		LocalDNSCacheMaxRecords: 8,
		LocalDNSCacheTTLSeconds: 60,
	}, nil, nil)
	now := time.Unix(1700000000, 0)
	c.now = func() time.Time { return now }

	rawQuery := buildClientTestDNSQuery(0x1234, "example.com", Enums.DNS_RECORD_TYPE_A, Enums.DNSQ_CLASS_IN)
	serverFailure, err := DnsParser.BuildServerFailureResponse(rawQuery)
	if err != nil {
		t.Fatalf("BuildServerFailureResponse returned error: %v", err)
	}

	if err := c.handleInboundDNSResponseFragment(VpnProto.Packet{
		SessionID:      7,
		PacketType:     Enums.PACKET_DNS_QUERY_RES,
		SequenceNum:    43,
		HasSequenceNum: true,
		FragmentID:     0,
		TotalFragments: 2,
		Payload:        serverFailure[:len(serverFailure)/2],
	}); err != nil {
		t.Fatalf("handleInboundDNSResponseFragment returned error: %v", err)
	}
	if err := c.handleInboundDNSResponseFragment(VpnProto.Packet{
		SessionID:      7,
		PacketType:     Enums.PACKET_DNS_QUERY_RES,
		SequenceNum:    43,
		HasSequenceNum: true,
		FragmentID:     1,
		TotalFragments: 2,
		Payload:        serverFailure[len(serverFailure)/2:],
	}); err != nil {
		t.Fatalf("handleInboundDNSResponseFragment returned error: %v", err)
	}

	cacheKey := dnscache.BuildKey("example.com", Enums.DNS_RECORD_TYPE_A, Enums.DNSQ_CLASS_IN)
	if _, ok := c.LocalDNSCache().GetReady(cacheKey, rawQuery, now); ok {
		t.Fatal("server failure dns responses must not be cached")
	}
}

func TestHandleInboundDNSResponseFragmentFlushesPersistedCacheImmediately(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.ClientConfig{
		ConfigDir:                 tempDir,
		LocalDNSCachePersist:      true,
		LocalDNSCacheMaxRecords:   8,
		LocalDNSCacheTTLSeconds:   3600,
		LocalDNSPendingTimeoutSec: 10,
	}

	now := time.Unix(1700000000, 0)
	rawQuery := buildClientTestDNSQuery(0x1234, "example.com", Enums.DNS_RECORD_TYPE_A, Enums.DNSQ_CLASS_IN)
	rawResponse := []byte{
		0x00, 0x00,
		0x81, 0x80,
		0x00, 0x01,
		0x00, 0x01,
		0x00, 0x00,
		0x00, 0x00,
	}
	rawResponse = append(rawResponse, encodeClientTestDNSName("example.com")...)
	rawResponse = append(rawResponse, 0x00, byte(Enums.DNS_RECORD_TYPE_A), 0x00, byte(Enums.DNSQ_CLASS_IN))
	rawResponse = append(rawResponse, 0xC0, 0x0C, 0x00, byte(Enums.DNS_RECORD_TYPE_A), 0x00, byte(Enums.DNSQ_CLASS_IN), 0x00, 0x00, 0x00, 0x3C, 0x00, 0x04, 1, 2, 3, 4)

	writer := New(cfg, nil, nil)
	writer.now = func() time.Time { return now }

	if err := writer.handleInboundDNSResponseFragment(VpnProto.Packet{
		SessionID:      7,
		PacketType:     Enums.PACKET_DNS_QUERY_RES,
		SequenceNum:    55,
		HasSequenceNum: true,
		FragmentID:     0,
		TotalFragments: 1,
		Payload:        rawResponse,
	}); err != nil {
		t.Fatalf("handleInboundDNSResponseFragment returned error: %v", err)
	}

	reader := New(cfg, nil, nil)
	reader.now = func() time.Time { return now.Add(time.Minute) }
	reader.ensureLocalDNSCacheLoaded()

	cacheKey := dnscache.BuildKey("example.com", Enums.DNS_RECORD_TYPE_A, Enums.DNSQ_CLASS_IN)
	cached, ok := reader.LocalDNSCache().GetReady(cacheKey, rawQuery, reader.now())
	if !ok {
		t.Fatal("expected persisted dns response to load without manual flush")
	}
	if binary.BigEndian.Uint16(cached[:2]) != 0x1234 {
		t.Fatalf("expected cached response id to be patched for current query, got=%#x", binary.BigEndian.Uint16(cached[:2]))
	}
}

func TestStartStream0RuntimeLoadsPersistedLocalDNSCache(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.ClientConfig{
		ConfigDir:                 tempDir,
		LocalDNSCachePersist:      true,
		LocalDNSCacheMaxRecords:   8,
		LocalDNSCacheTTLSeconds:   3600,
		LocalDNSPendingTimeoutSec: 10,
	}

	now := time.Unix(1700000000, 0)
	rawQuery := buildClientTestDNSQuery(0x1234, "example.com", Enums.DNS_RECORD_TYPE_A, Enums.DNSQ_CLASS_IN)
	rawResponse := []byte{
		0x00, 0x00,
		0x81, 0x80,
		0x00, 0x01,
		0x00, 0x01,
		0x00, 0x00,
		0x00, 0x00,
	}
	rawResponse = append(rawResponse, encodeClientTestDNSName("example.com")...)
	rawResponse = append(rawResponse, 0x00, byte(Enums.DNS_RECORD_TYPE_A), 0x00, byte(Enums.DNSQ_CLASS_IN))
	rawResponse = append(rawResponse, 0xC0, 0x0C, 0x00, byte(Enums.DNS_RECORD_TYPE_A), 0x00, byte(Enums.DNSQ_CLASS_IN), 0x00, 0x00, 0x00, 0x3C, 0x00, 0x04, 1, 2, 3, 4)
	cacheKey := dnscache.BuildKey("example.com", Enums.DNS_RECORD_TYPE_A, Enums.DNSQ_CLASS_IN)

	writer := New(cfg, nil, nil)
	writer.now = func() time.Time { return now }
	writer.LocalDNSCache().SetReady(cacheKey, "example.com", Enums.DNS_RECORD_TYPE_A, Enums.DNSQ_CLASS_IN, rawResponse, now)
	writer.flushLocalDNSCache()

	reader := New(cfg, nil, nil)
	reader.now = func() time.Time { return now.Add(time.Minute) }
	ctx, cancel := context.WithCancel(context.Background())
	if err := reader.startStream0Runtime(ctx); err != nil {
		t.Fatalf("startStream0Runtime returned error: %v", err)
	}
	defer func() {
		cancel()
		time.Sleep(20 * time.Millisecond)
	}()

	cached, ok := reader.LocalDNSCache().GetReady(cacheKey, rawQuery, reader.now())
	if !ok {
		t.Fatal("expected persisted local dns cache entry to be loaded on runtime start")
	}
	if binary.BigEndian.Uint16(cached[:2]) != 0x1234 {
		t.Fatalf("expected cached response id to be patched for current query, got=%#x", binary.BigEndian.Uint16(cached[:2]))
	}
}

func TestQueueDNSDispatchEnqueuesFragmentedRequests(t *testing.T) {
	oldBaseDelay := stream0DNSRetryBaseDelay
	oldMaxDelay := stream0DNSRetryMaxDelay
	stream0DNSRetryBaseDelay = 20 * time.Millisecond
	stream0DNSRetryMaxDelay = 40 * time.Millisecond
	defer func() {
		stream0DNSRetryBaseDelay = oldBaseDelay
		stream0DNSRetryMaxDelay = oldMaxDelay
	}()

	codec, err := security.NewCodec(0, "")
	if err != nil {
		t.Fatalf("NewCodec returned error: %v", err)
	}

	c := New(config.ClientConfig{
		LocalDNSPendingTimeoutSec:  1,
		LocalDNSFragmentTimeoutSec: 300,
		Domains:                    []string{"v.example.com"},
	}, nil, codec)
	c.connections = []Connection{{
		Domain:        "v.example.com",
		Resolver:      "127.0.0.1",
		ResolverPort:  5353,
		ResolverLabel: "127.0.0.1:5353",
		Key:           "127.0.0.1|5353|v.example.com",
		IsValid:       true,
	}}
	c.connectionsByKey = map[string]int{c.connections[0].Key: 0}
	c.rebuildBalancer()
	c.sessionID = 7
	c.sessionCookie = 9
	c.sessionReady = true
	c.responseMode = mtuProbeRawResponse
	c.syncedUploadMTU = 20

	seenFragments := make(chan VpnProto.Packet, 4)
	c.exchangeQueryFn = func(conn Connection, packet []byte, timeout time.Duration) ([]byte, error) {
		queryPacket, err := DnsParser.ParsePacketLite(packet)
		if err != nil || !queryPacket.HasQuestion {
			t.Fatalf("unexpected tunnel query: err=%v", err)
		}
		vpnPacket, err := VpnProto.ParseFromLabels(extractTestTunnelLabels(queryPacket.FirstQuestion.Name, "v.example.com"), c.codec)
		if err != nil {
			t.Fatalf("ParseFromLabels returned error: %v", err)
		}
		seenFragments <- vpnPacket
		return DnsParser.BuildVPNResponsePacket(packet, queryPacket.FirstQuestion.Name, VpnProto.Packet{
			SessionID:      c.sessionID,
			SessionCookie:  c.sessionCookie,
			PacketType:     Enums.PACKET_DNS_QUERY_REQ_ACK,
			StreamID:       0,
			SequenceNum:    vpnPacket.SequenceNum,
			FragmentID:     vpnPacket.FragmentID,
			TotalFragments: vpnPacket.TotalFragments,
		}, false)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := c.startStream0Runtime(ctx); err != nil {
		t.Fatalf("startStream0Runtime returned error: %v", err)
	}

	query := buildClientTestDNSQuery(0x1234, "example.com", Enums.DNS_RECORD_TYPE_A, Enums.DNSQ_CLASS_IN)
	dispatch := &dnsDispatchRequest{
		Query:  query,
		Domain: "example.com",
		QType:  Enums.DNS_RECORD_TYPE_A,
		QClass: Enums.DNSQ_CLASS_IN,
	}
	c.queueDNSDispatch(dispatch)

	collected := make([]VpnProto.Packet, 0, 4)
	timeout := time.After(2 * time.Second)
	for len(collected) < 2 {
		select {
		case packet := <-seenFragments:
			collected = append(collected, packet)
		case <-timeout:
			t.Fatalf("timed out waiting for fragmented dns requests, seen=%d", len(collected))
		}
	}

	sequence := collected[0].SequenceNum
	total := collected[0].TotalFragments
	if total < 2 {
		t.Fatalf("expected fragmented dns request, total=%d", total)
	}
	for _, packet := range collected {
		if packet.PacketType != Enums.PACKET_DNS_QUERY_REQ {
			t.Fatalf("unexpected packet type: %d", packet.PacketType)
		}
		if packet.StreamID != 0 {
			t.Fatalf("dns request fragments must use main stream, got=%d", packet.StreamID)
		}
		if packet.SequenceNum != sequence {
			t.Fatalf("expected shared sequence number, got=%d want=%d", packet.SequenceNum, sequence)
		}
		if packet.TotalFragments != total {
			t.Fatalf("expected stable total fragments, got=%d want=%d", packet.TotalFragments, total)
		}
	}
}

func TestStream0RuntimeUsesSlowPingForPendingDNSOnly(t *testing.T) {
	oldDNSOnlyInterval := stream0DNSOnlyPingInterval
	oldDNSOnlyWarmDuration := stream0DNSOnlyWarmDuration
	oldDNSOnlyWarmInterval := stream0DNSOnlyWarmPingInterval
	oldDNSOnlyWarmSleep := stream0DNSOnlyWarmMaxSleep
	oldDNSOnlySleep := stream0PingDNSOnlyMaxSleep
	stream0DNSOnlyWarmDuration = time.Second
	stream0DNSOnlyWarmPingInterval = 25 * time.Millisecond
	stream0DNSOnlyWarmMaxSleep = 10 * time.Millisecond
	stream0DNSOnlyPingInterval = 25 * time.Millisecond
	stream0PingDNSOnlyMaxSleep = 10 * time.Millisecond
	defer func() {
		stream0DNSOnlyWarmDuration = oldDNSOnlyWarmDuration
		stream0DNSOnlyWarmPingInterval = oldDNSOnlyWarmInterval
		stream0DNSOnlyWarmMaxSleep = oldDNSOnlyWarmSleep
		stream0DNSOnlyPingInterval = oldDNSOnlyInterval
		stream0PingDNSOnlyMaxSleep = oldDNSOnlySleep
	}()

	codec, err := security.NewCodec(0, "")
	if err != nil {
		t.Fatalf("NewCodec returned error: %v", err)
	}

	c := New(config.ClientConfig{
		LocalDNSPendingTimeoutSec: 10,
	}, nil, codec)
	now := time.Unix(1700000000, 0)
	c.now = func() time.Time { return now }
	c.connections = []Connection{{
		Domain:        "v.example.com",
		Resolver:      "127.0.0.1",
		ResolverPort:  5353,
		ResolverLabel: "127.0.0.1:5353",
		Key:           "127.0.0.1|5353|v.example.com",
		IsValid:       true,
	}}
	c.connectionsByKey = map[string]int{c.connections[0].Key: 0}
	c.rebuildBalancer()
	c.sessionID = 7
	c.sessionCookie = 9
	c.sessionReady = true
	c.responseMode = mtuProbeRawResponse

	c.localDNSCache.LookupOrCreatePending(
		dnscache.BuildKey("example.com", Enums.DNS_RECORD_TYPE_A, Enums.DNSQ_CLASS_IN),
		"example.com",
		Enums.DNS_RECORD_TYPE_A,
		Enums.DNSQ_CLASS_IN,
		now,
	)

	pingSeen := make(chan struct{}, 1)
	c.exchangeQueryFn = func(conn Connection, packet []byte, timeout time.Duration) ([]byte, error) {
		queryPacket, err := DnsParser.ParsePacketLite(packet)
		if err != nil || !queryPacket.HasQuestion {
			t.Fatalf("unexpected tunnel query: err=%v", err)
		}
		vpnPacket, err := VpnProto.ParseFromLabels(extractTestTunnelLabels(queryPacket.FirstQuestion.Name, "v.example.com"), c.codec)
		if err != nil {
			t.Fatalf("ParseFromLabels returned error: %v", err)
		}
		if vpnPacket.PacketType == Enums.PACKET_PING {
			select {
			case pingSeen <- struct{}{}:
			default:
			}
		}
		return DnsParser.BuildVPNResponsePacket(packet, queryPacket.FirstQuestion.Name, VpnProto.Packet{
			SessionID:     c.sessionID,
			SessionCookie: c.sessionCookie,
			PacketType:    Enums.PACKET_PONG,
			Payload:       []byte("PO:test"),
		}, false)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := c.startStream0Runtime(ctx); err != nil {
		t.Fatalf("startStream0Runtime returned error: %v", err)
	}

	select {
	case <-pingSeen:
	case <-time.After(time.Second):
		t.Fatal("expected dns-only keepalive ping to be sent")
	}
}

func TestStream0RuntimeRetriesDNSQueryAfterMissingAck(t *testing.T) {
	oldBaseDelay := stream0DNSRetryBaseDelay
	oldMaxDelay := stream0DNSRetryMaxDelay
	stream0DNSRetryBaseDelay = 20 * time.Millisecond
	stream0DNSRetryMaxDelay = 40 * time.Millisecond
	defer func() {
		stream0DNSRetryBaseDelay = oldBaseDelay
		stream0DNSRetryMaxDelay = oldMaxDelay
	}()

	codec, err := security.NewCodec(0, "")
	if err != nil {
		t.Fatalf("NewCodec returned error: %v", err)
	}

	c := New(config.ClientConfig{
		LocalDNSPendingTimeoutSec:  1,
		LocalDNSFragmentTimeoutSec: 300,
		Domains:                    []string{"v.example.com"},
	}, nil, codec)
	c.connections = []Connection{{
		Domain:        "v.example.com",
		Resolver:      "127.0.0.1",
		ResolverPort:  5353,
		ResolverLabel: "127.0.0.1:5353",
		Key:           "127.0.0.1|5353|v.example.com",
		IsValid:       true,
	}}
	c.connectionsByKey = map[string]int{c.connections[0].Key: 0}
	c.rebuildBalancer()
	c.sessionID = 7
	c.sessionCookie = 9
	c.sessionReady = true
	c.responseMode = mtuProbeRawResponse
	c.syncedUploadMTU = EDnsSafeUDPSize

	callCount := 0
	c.exchangeQueryFn = func(conn Connection, packet []byte, timeout time.Duration) ([]byte, error) {
		callCount++
		queryPacket, err := DnsParser.ParsePacketLite(packet)
		if err != nil || !queryPacket.HasQuestion {
			t.Fatalf("unexpected tunnel dns query: err=%v", err)
		}
		vpnPacket, err := VpnProto.ParseFromLabels(extractTestTunnelLabels(queryPacket.FirstQuestion.Name, "v.example.com"), c.codec)
		if err != nil {
			t.Fatalf("ParseFromLabels returned error: %v", err)
		}
		packetType := uint8(Enums.PACKET_PONG)
		fragmentID := uint8(0)
		totalFragments := uint8(1)
		if callCount >= 2 {
			packetType = uint8(Enums.PACKET_DNS_QUERY_REQ_ACK)
			fragmentID = vpnPacket.FragmentID
			totalFragments = vpnPacket.TotalFragments
		}
		return DnsParser.BuildVPNResponsePacket(packet, queryPacket.FirstQuestion.Name, VpnProto.Packet{
			SessionID:      c.sessionID,
			SessionCookie:  c.sessionCookie,
			PacketType:     packetType,
			StreamID:       0,
			SequenceNum:    vpnPacket.SequenceNum,
			FragmentID:     fragmentID,
			TotalFragments: totalFragments,
			Payload:        []byte("PO:test"),
		}, false)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := c.startStream0Runtime(ctx); err != nil {
		t.Fatalf("startStream0Runtime returned error: %v", err)
	}

	rawQuery := buildClientTestDNSQuery(0x1234, "example.com", Enums.DNS_RECORD_TYPE_A, Enums.DNSQ_CLASS_IN)
	if err := c.stream0Runtime.QueueDNSRequest(rawQuery); err != nil {
		t.Fatalf("QueueDNSRequest returned error: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		c.stream0Runtime.mu.Lock()
		pending := len(c.stream0Runtime.dnsRequests)
		c.stream0Runtime.mu.Unlock()
		if pending == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected dns request retries to eventually receive ack, pending=%d callCount=%d", pending, callCount)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if callCount < 2 {
		t.Fatalf("expected retry to happen, got callCount=%d", callCount)
	}
}

func TestOpenSOCKS5StreamCompletesHandshake(t *testing.T) {
	codec, err := security.NewCodec(0, "")
	if err != nil {
		t.Fatalf("NewCodec returned error: %v", err)
	}

	c := New(config.ClientConfig{
		LocalDNSPendingTimeoutSec: 1,
	}, nil, codec)
	c.connections = []Connection{{
		Domain:        "v.example.com",
		Resolver:      "127.0.0.1",
		ResolverPort:  5353,
		ResolverLabel: "127.0.0.1:5353",
		Key:           "127.0.0.1|5353|v.example.com",
		IsValid:       true,
	}}
	c.connectionsByKey = map[string]int{c.connections[0].Key: 0}
	c.rebuildBalancer()
	c.sessionID = 7
	c.sessionCookie = 9
	c.sessionReady = true
	c.responseMode = mtuProbeRawResponse

	c.exchangeQueryFn = func(conn Connection, packet []byte, timeout time.Duration) ([]byte, error) {
		queryPacket, err := DnsParser.ParsePacketLite(packet)
		if err != nil || !queryPacket.HasQuestion {
			t.Fatalf("unexpected tunnel query: err=%v", err)
		}
		vpnPacket, err := VpnProto.ParseFromLabels(extractTestTunnelLabels(queryPacket.FirstQuestion.Name, "v.example.com"), c.codec)
		if err != nil {
			t.Fatalf("ParseFromLabels returned error: %v", err)
		}
		responseType := uint8(Enums.PACKET_SOCKS5_SYN_ACK)
		if vpnPacket.PacketType == Enums.PACKET_STREAM_SYN {
			responseType = uint8(Enums.PACKET_STREAM_SYN_ACK)
		}
		return DnsParser.BuildVPNResponsePacket(packet, queryPacket.FirstQuestion.Name, VpnProto.Packet{
			SessionID:      c.sessionID,
			SessionCookie:  c.sessionCookie,
			PacketType:     responseType,
			StreamID:       vpnPacket.StreamID,
			SequenceNum:    vpnPacket.SequenceNum,
			FragmentID:     0,
			TotalFragments: 1,
		}, false)
	}

	streamID, err := c.OpenSOCKS5Stream([]byte{0x03, 0x0B, 'e', 'x', 'a', 'm', 'p', 'l', 'e', '.', 'c', 'o', 'm', 0x01, 0xBB}, time.Second)
	if err != nil {
		t.Fatalf("OpenSOCKS5Stream returned error: %v", err)
	}
	if streamID == 0 {
		t.Fatal("expected non-zero stream id")
	}
}

func TestOpenSOCKS5StreamReturnsServerError(t *testing.T) {
	codec, err := security.NewCodec(0, "")
	if err != nil {
		t.Fatalf("NewCodec returned error: %v", err)
	}

	c := New(config.ClientConfig{
		LocalDNSPendingTimeoutSec: 1,
	}, nil, codec)
	c.connections = []Connection{{
		Domain:        "v.example.com",
		Resolver:      "127.0.0.1",
		ResolverPort:  5353,
		ResolverLabel: "127.0.0.1:5353",
		Key:           "127.0.0.1|5353|v.example.com",
		IsValid:       true,
	}}
	c.connectionsByKey = map[string]int{c.connections[0].Key: 0}
	c.rebuildBalancer()
	c.sessionID = 7
	c.sessionCookie = 9
	c.sessionReady = true
	c.responseMode = mtuProbeRawResponse

	c.exchangeQueryFn = func(conn Connection, packet []byte, timeout time.Duration) ([]byte, error) {
		queryPacket, err := DnsParser.ParsePacketLite(packet)
		if err != nil || !queryPacket.HasQuestion {
			t.Fatalf("unexpected tunnel query: err=%v", err)
		}
		vpnPacket, err := VpnProto.ParseFromLabels(extractTestTunnelLabels(queryPacket.FirstQuestion.Name, "v.example.com"), c.codec)
		if err != nil {
			t.Fatalf("ParseFromLabels returned error: %v", err)
		}
		responseType := uint8(Enums.PACKET_SOCKS5_CONNECTION_REFUSED)
		if vpnPacket.PacketType == Enums.PACKET_STREAM_SYN {
			responseType = uint8(Enums.PACKET_STREAM_SYN_ACK)
		}
		return DnsParser.BuildVPNResponsePacket(packet, queryPacket.FirstQuestion.Name, VpnProto.Packet{
			SessionID:      c.sessionID,
			SessionCookie:  c.sessionCookie,
			PacketType:     responseType,
			StreamID:       vpnPacket.StreamID,
			SequenceNum:    vpnPacket.SequenceNum,
			FragmentID:     0,
			TotalFragments: 1,
		}, false)
	}

	_, err = c.OpenSOCKS5Stream([]byte{0x01, 127, 0, 0, 1, 0x01, 0xBB}, time.Second)
	if err == nil {
		t.Fatal("expected handshake error")
	}
}

func TestOpenTCPStreamCompletesHandshake(t *testing.T) {
	codec, err := security.NewCodec(0, "")
	if err != nil {
		t.Fatalf("NewCodec returned error: %v", err)
	}

	c := New(config.ClientConfig{
		ProtocolType:              "TCP",
		LocalDNSPendingTimeoutSec: 1,
	}, nil, codec)
	c.connections = []Connection{{
		Domain:        "v.example.com",
		Resolver:      "127.0.0.1",
		ResolverPort:  5353,
		ResolverLabel: "127.0.0.1:5353",
		Key:           "127.0.0.1|5353|v.example.com",
		IsValid:       true,
	}}
	c.connectionsByKey = map[string]int{c.connections[0].Key: 0}
	c.rebuildBalancer()
	c.sessionID = 7
	c.sessionCookie = 9
	c.sessionReady = true
	c.responseMode = mtuProbeRawResponse

	c.exchangeQueryFn = func(conn Connection, packet []byte, timeout time.Duration) ([]byte, error) {
		queryPacket, err := DnsParser.ParsePacketLite(packet)
		if err != nil || !queryPacket.HasQuestion {
			t.Fatalf("unexpected tunnel query: err=%v", err)
		}
		vpnPacket, err := VpnProto.ParseFromLabels(extractTestTunnelLabels(queryPacket.FirstQuestion.Name, "v.example.com"), c.codec)
		if err != nil {
			t.Fatalf("ParseFromLabels returned error: %v", err)
		}
		if vpnPacket.PacketType != Enums.PACKET_STREAM_SYN || !VpnProto.IsTCPForwardSynPayload(vpnPacket.Payload) {
			t.Fatalf("unexpected tcp syn packet: %+v", vpnPacket)
		}
		return DnsParser.BuildVPNResponsePacket(packet, queryPacket.FirstQuestion.Name, VpnProto.Packet{
			SessionID:      c.sessionID,
			SessionCookie:  c.sessionCookie,
			PacketType:     Enums.PACKET_STREAM_SYN_ACK,
			StreamID:       vpnPacket.StreamID,
			SequenceNum:    vpnPacket.SequenceNum,
			FragmentID:     0,
			TotalFragments: 1,
		}, false)
	}

	streamID, err := c.OpenTCPStream(time.Second)
	if err != nil {
		t.Fatalf("OpenTCPStream returned error: %v", err)
	}
	if streamID == 0 {
		t.Fatal("expected non-zero stream id")
	}
}

func TestPerformSOCKS5HandshakeParsesConnectRequest(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	c := New(config.ClientConfig{}, nil, nil)

	done := make(chan error, 1)
	go func() {
		defer close(done)
		request, err := c.performSOCKS5Handshake(serverConn)
		if err != nil {
			done <- err
			return
		}
		if request.Command != 0x01 {
			done <- errors.New("unexpected socks5 command")
			return
		}
		expected := []byte{0x03, 0x0B, 'e', 'x', 'a', 'm', 'p', 'l', 'e', '.', 'c', 'o', 'm', 0x01, 0xBB}
		if string(request.TargetPayload) != string(expected) {
			done <- errors.New("unexpected socks5 payload")
			return
		}
		done <- nil
	}()

	if _, err := clientConn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	reply := make([]byte, 2)
	if _, err := io.ReadFull(clientConn, reply); err != nil {
		t.Fatalf("ReadFull returned error: %v", err)
	}
	if string(reply) != string([]byte{0x05, 0x00}) {
		t.Fatalf("unexpected greeting reply: %v", reply)
	}
	request := []byte{0x05, 0x01, 0x00, 0x03, 0x0B, 'e', 'x', 'a', 'm', 'p', 'l', 'e', '.', 'c', 'o', 'm', 0x01, 0xBB}
	if _, err := clientConn.Write(request); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("performSOCKS5Handshake returned error: %v", err)
	}
}

func TestPerformSOCKS5HandshakeParsesUDPAssociateRequest(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	c := New(config.ClientConfig{}, nil, nil)

	done := make(chan error, 1)
	go func() {
		defer close(done)
		request, err := c.performSOCKS5Handshake(serverConn)
		if err != nil {
			done <- err
			return
		}
		if request.Command != 0x03 {
			done <- errors.New("unexpected socks5 udp associate command")
			return
		}
		expected := []byte{0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
		if string(request.TargetPayload) != string(expected) {
			done <- errors.New("unexpected socks5 udp associate payload")
			return
		}
		done <- nil
	}()

	if _, err := clientConn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	reply := make([]byte, 2)
	if _, err := io.ReadFull(clientConn, reply); err != nil {
		t.Fatalf("ReadFull returned error: %v", err)
	}
	request := []byte{0x05, 0x03, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	if _, err := clientConn.Write(request); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("performSOCKS5Handshake returned error: %v", err)
	}
}

func TestPerformSOCKS5HandshakeAuthenticatesUserPass(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	c := New(config.ClientConfig{
		SOCKS5Auth: true,
		SOCKS5User: "user",
		SOCKS5Pass: "pass",
	}, nil, nil)

	done := make(chan error, 1)
	go func() {
		defer close(done)
		request, err := c.performSOCKS5Handshake(serverConn)
		if err != nil {
			done <- err
			return
		}
		if request.Command != 0x01 {
			done <- errors.New("unexpected socks5 command")
			return
		}
		done <- nil
	}()

	if _, err := clientConn.Write([]byte{0x05, 0x02, 0x00, 0x02}); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	reply := make([]byte, 2)
	if _, err := io.ReadFull(clientConn, reply); err != nil {
		t.Fatalf("ReadFull returned error: %v", err)
	}
	if string(reply) != string([]byte{0x05, 0x02}) {
		t.Fatalf("unexpected method selection reply: %v", reply)
	}
	authRequest := []byte{0x01, 0x04, 'u', 's', 'e', 'r', 0x04, 'p', 'a', 's', 's'}
	if _, err := clientConn.Write(authRequest); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if _, err := io.ReadFull(clientConn, reply); err != nil {
		t.Fatalf("ReadFull returned error: %v", err)
	}
	if string(reply) != string([]byte{0x01, 0x00}) {
		t.Fatalf("unexpected auth reply: %v", reply)
	}
	request := []byte{0x05, 0x01, 0x00, 0x03, 0x0B, 'e', 'x', 'a', 'm', 'p', 'l', 'e', '.', 'c', 'o', 'm', 0x01, 0xBB}
	if _, err := clientConn.Write(request); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("performSOCKS5Handshake returned error: %v", err)
	}
}

func TestPerformSOCKS5HandshakeRejectsInvalidCredentials(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	c := New(config.ClientConfig{
		SOCKS5Auth: true,
		SOCKS5User: "user",
		SOCKS5Pass: "pass",
	}, nil, nil)

	done := make(chan error, 1)
	go func() {
		defer close(done)
		_, err := c.performSOCKS5Handshake(serverConn)
		done <- err
	}()

	if _, err := clientConn.Write([]byte{0x05, 0x01, 0x02}); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	reply := make([]byte, 2)
	if _, err := io.ReadFull(clientConn, reply); err != nil {
		t.Fatalf("ReadFull returned error: %v", err)
	}
	if string(reply) != string([]byte{0x05, 0x02}) {
		t.Fatalf("unexpected method selection reply: %v", reply)
	}
	authRequest := []byte{0x01, 0x04, 'u', 's', 'e', 'r', 0x05, 'w', 'r', 'o', 'n', 'g'}
	if _, err := clientConn.Write(authRequest); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if _, err := io.ReadFull(clientConn, reply); err != nil {
		t.Fatalf("ReadFull returned error: %v", err)
	}
	if string(reply) != string([]byte{0x01, 0x01}) {
		t.Fatalf("unexpected auth failure reply: %v", reply)
	}
	if err := <-done; !errors.Is(err, errSOCKS5AuthFailed) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleSOCKS5UDPDatagramResolvesDNS(t *testing.T) {
	codec, err := security.NewCodec(0, "")
	if err != nil {
		t.Fatalf("NewCodec returned error: %v", err)
	}
	c := New(config.ClientConfig{}, nil, codec)

	query := buildClientTestDNSQuery(0x2201, "example.com", Enums.DNS_RECORD_TYPE_A, Enums.DNSQ_CLASS_IN)
	serverFailure, err := DnsParser.BuildServerFailureResponse(query)
	if err != nil {
		t.Fatalf("BuildServerFailureResponse returned error: %v", err)
	}

	cacheKey := dnscache.BuildKey("example.com", Enums.DNS_RECORD_TYPE_A, Enums.DNSQ_CLASS_IN)
	c.localDNSCache.SetReady(cacheKey, "example.com", Enums.DNS_RECORD_TYPE_A, Enums.DNSQ_CLASS_IN, serverFailure, c.now())

	packet := SocksProto.BuildUDPDatagram(SocksProto.Target{
		AddressType: SocksProto.AddressTypeIPv4,
		Host:        "8.8.8.8",
		Port:        53,
	}, query)
	response := c.handleSOCKS5UDPDatagram(packet)
	if len(response) == 0 {
		t.Fatal("expected udp associate dns response")
	}

	datagram, err := SocksProto.ParseUDPDatagram(response)
	if err != nil {
		t.Fatalf("ParseUDPDatagram returned error: %v", err)
	}
	if datagram.Target.Port != 53 || datagram.Target.Host != "8.8.8.8" {
		t.Fatalf("unexpected udp response target: %+v", datagram.Target)
	}
	if binary.BigEndian.Uint16(datagram.Payload[:2]) != 0x2201 {
		t.Fatalf("expected dns response id to be preserved, got=%#x", binary.BigEndian.Uint16(datagram.Payload[:2]))
	}
}

func TestHandleInboundStreamPacketIgnoresDuplicateData(t *testing.T) {
	c := New(config.ClientConfig{}, nil, nil)
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	stream := c.createStream(5, serverConn)
	packet := VpnProto.Packet{
		PacketType:  Enums.PACKET_STREAM_DATA,
		StreamID:    5,
		SequenceNum: 7,
		Payload:     []byte("abc"),
	}

	writeDone := make(chan []byte, 2)
	go func() {
		buffer := make([]byte, 8)
		n, _ := clientConn.Read(buffer)
		writeDone <- append([]byte(nil), buffer[:n]...)
		_ = clientConn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		n, err := clientConn.Read(buffer)
		if err == nil {
			writeDone <- append([]byte(nil), buffer[:n]...)
			return
		}
		writeDone <- nil
	}()

	c.exchangeQueryFn = func(conn Connection, packet []byte, timeout time.Duration) ([]byte, error) {
		return nil, ErrTunnelDNSDispatchFailed
	}

	_, _ = c.handleInboundStreamPacket(packet, time.Second)
	_, _ = c.handleInboundStreamPacket(packet, time.Second)

	first := <-writeDone
	second := <-writeDone
	if string(first) != "abc" {
		t.Fatalf("unexpected first payload: %q", first)
	}
	if second != nil {
		t.Fatalf("duplicate inbound stream data must not be written again: %q", second)
	}

	c.deleteStream(stream.ID)
}

func TestClientStreamTXLoopAdvancesQueueOnDataAck(t *testing.T) {
	codec, err := security.NewCodec(0, "")
	if err != nil {
		t.Fatalf("NewCodec returned error: %v", err)
	}

	c := New(config.ClientConfig{
		LocalDNSPendingTimeoutSec: 1,
	}, nil, codec)
	c.connections = []Connection{{
		Domain:        "v.example.com",
		Resolver:      "127.0.0.1",
		ResolverPort:  5353,
		ResolverLabel: "127.0.0.1:5353",
		Key:           "127.0.0.1|5353|v.example.com",
		IsValid:       true,
	}}
	c.connectionsByKey = map[string]int{c.connections[0].Key: 0}
	c.rebuildBalancer()
	c.sessionID = 7
	c.sessionCookie = 9
	c.sessionReady = true
	c.responseMode = mtuProbeRawResponse

	callSeq := make(chan uint16, 4)
	c.exchangeQueryFn = func(conn Connection, packet []byte, timeout time.Duration) ([]byte, error) {
		queryPacket, err := DnsParser.ParsePacketLite(packet)
		if err != nil || !queryPacket.HasQuestion {
			t.Fatalf("unexpected tunnel query: err=%v", err)
		}
		vpnPacket, err := VpnProto.ParseFromLabels(extractTestTunnelLabels(queryPacket.FirstQuestion.Name, "v.example.com"), c.codec)
		if err != nil {
			t.Fatalf("ParseFromLabels returned error: %v", err)
		}
		callSeq <- vpnPacket.SequenceNum
		return DnsParser.BuildVPNResponsePacket(packet, queryPacket.FirstQuestion.Name, VpnProto.Packet{
			SessionID:      c.sessionID,
			SessionCookie:  c.sessionCookie,
			PacketType:     Enums.PACKET_STREAM_DATA_ACK,
			StreamID:       vpnPacket.StreamID,
			SequenceNum:    vpnPacket.SequenceNum,
			FragmentID:     0,
			TotalFragments: 1,
		}, false)
	}

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	stream := c.createStream(17, serverConn)
	defer c.deleteStream(stream.ID)

	if err := c.queueStreamPacket(stream, Enums.PACKET_STREAM_DATA, []byte("one")); err != nil {
		t.Fatalf("queueStreamPacket returned error: %v", err)
	}
	if err := c.queueStreamPacket(stream, Enums.PACKET_STREAM_DATA, []byte("two")); err != nil {
		t.Fatalf("queueStreamPacket returned error: %v", err)
	}

	timeout := time.After(2 * time.Second)
	seen := make([]uint16, 0, 2)
	for len(seen) < 2 {
		select {
		case seq := <-callSeq:
			seen = append(seen, seq)
		case <-timeout:
			t.Fatalf("timed out waiting for queued packets, seen=%v", seen)
		}
	}
	if seen[0] == seen[1] {
		t.Fatalf("expected different sequence numbers for queued packets: %v", seen)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		stream.mu.Lock()
		queueLen := len(stream.TXQueue)
		inflightLen := len(stream.TXInFlight)
		stream.mu.Unlock()
		if queueLen == 0 && inflightLen == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected queue to drain after acks, queueLen=%d inflightLen=%d", queueLen, inflightLen)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestClientStreamTXAckRemovesOutOfOrderInflightPacket(t *testing.T) {
	c := New(config.ClientConfig{}, nil, nil)
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	stream := c.createStream(21, serverConn)
	defer c.deleteStream(stream.ID)

	for _, payload := range [][]byte{[]byte("one"), []byte("two"), []byte("three")} {
		if err := c.queueStreamPacket(stream, Enums.PACKET_STREAM_DATA, payload); err != nil {
			t.Fatalf("queueStreamPacket returned error: %v", err)
		}
	}

	if packet, waitFor, stop := nextClientStreamTX(stream, 4); stop || packet == nil || waitFor != 0 {
		t.Fatalf("expected first inflight packet, stop=%v packet=%v wait=%v", stop, packet, waitFor)
	}
	if packet, waitFor, stop := nextClientStreamTX(stream, 4); stop || packet == nil || waitFor != 0 {
		t.Fatalf("expected second inflight packet, stop=%v packet=%v wait=%v", stop, packet, waitFor)
	}

	stream.mu.Lock()
	beforeAckLen := len(stream.TXInFlight)
	if len(stream.TXInFlight) < 2 {
		stream.mu.Unlock()
		t.Fatalf("expected at least 2 inflight packets, got=%d", len(stream.TXInFlight))
	}
	secondSeq := stream.TXInFlight[1].SequenceNum
	stream.mu.Unlock()

	ackClientStreamTX(stream, secondSeq, time.Now())

	stream.mu.Lock()
	defer stream.mu.Unlock()
	if len(stream.TXInFlight) != beforeAckLen-1 {
		t.Fatalf("expected inflight queue to shrink by one after out-of-order ack, before=%d after=%d", beforeAckLen, len(stream.TXInFlight))
	}
	for _, packet := range stream.TXInFlight {
		if packet.SequenceNum == secondSeq {
			t.Fatal("acked packet must be removed from inflight queue")
		}
	}
}

func TestQueueStreamRSTClearsPendingData(t *testing.T) {
	c := New(config.ClientConfig{}, nil, nil)
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	stream := c.createStream(22, serverConn)
	defer c.deleteStream(stream.ID)

	if err := c.queueStreamPacket(stream, Enums.PACKET_STREAM_DATA, []byte("one")); err != nil {
		t.Fatalf("queueStreamPacket returned error: %v", err)
	}
	if err := c.queueStreamPacket(stream, Enums.PACKET_STREAM_DATA, []byte("two")); err != nil {
		t.Fatalf("queueStreamPacket returned error: %v", err)
	}
	if packet, _, stop := nextClientStreamTX(stream, 4); stop || packet == nil {
		t.Fatalf("expected first packet to move inflight, stop=%v packet=%v", stop, packet)
	}

	if err := c.queueStreamPacket(stream, Enums.PACKET_STREAM_RST, nil); err != nil {
		t.Fatalf("queueStreamPacket returned error: %v", err)
	}

	stream.mu.Lock()
	defer stream.mu.Unlock()
	if len(stream.TXInFlight) != 0 {
		t.Fatalf("expected inflight data to be cleared on reset, got=%d", len(stream.TXInFlight))
	}
	if len(stream.TXQueue) != 1 || stream.TXQueue[0].PacketType != Enums.PACKET_STREAM_RST {
		t.Fatalf("expected reset to become sole queued packet, queue=%+v", stream.TXQueue)
	}
}

func TestQueueStreamPacketRejectsDataOnBackpressure(t *testing.T) {
	c := New(config.ClientConfig{
		StreamTXWindow:     1,
		StreamTXQueueLimit: 2,
	}, nil, nil)
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	stream := c.createStream(23, serverConn)
	defer c.deleteStream(stream.ID)

	if err := c.queueStreamPacket(stream, Enums.PACKET_STREAM_DATA, []byte("one")); err != nil {
		t.Fatalf("queueStreamPacket returned error: %v", err)
	}
	if packet, _, stop := nextClientStreamTX(stream, 1); stop || packet == nil {
		t.Fatalf("expected first packet to move inflight, stop=%v packet=%v", stop, packet)
	}
	if err := c.queueStreamPacket(stream, Enums.PACKET_STREAM_DATA, []byte("two")); err != nil {
		t.Fatalf("queueStreamPacket returned error: %v", err)
	}
	if err := c.queueStreamPacket(stream, Enums.PACKET_STREAM_DATA, []byte("three")); !errors.Is(err, ErrClientStreamBackpressure) {
		t.Fatalf("expected backpressure error, got=%v", err)
	}
	if err := c.queueStreamPacket(stream, Enums.PACKET_STREAM_FIN, nil); err != nil {
		t.Fatalf("control packet should still enqueue under backpressure: %v", err)
	}
}

func TestExpireClientStreamTXQueuesRSTOnRetryBudgetExceeded(t *testing.T) {
	c := New(config.ClientConfig{
		StreamTXWindow:     1,
		StreamTXQueueLimit: 8,
		StreamTXMaxRetries: 1,
		StreamTXTTLSeconds: 60,
	}, nil, nil)
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	stream := c.createStream(24, serverConn)
	defer c.deleteStream(stream.ID)

	if err := c.queueStreamPacket(stream, Enums.PACKET_STREAM_DATA, []byte("stalled")); err != nil {
		t.Fatalf("queueStreamPacket returned error: %v", err)
	}
	if packet, _, stop := nextClientStreamTX(stream, 1); stop || packet == nil {
		t.Fatalf("expected first packet to move inflight, stop=%v packet=%v", stop, packet)
	}

	stream.mu.Lock()
	if len(stream.TXInFlight) != 1 {
		stream.mu.Unlock()
		t.Fatalf("expected one inflight packet, got=%d", len(stream.TXInFlight))
	}
	stream.TXInFlight[0].RetryCount = 1
	stream.TXInFlight[0].CreatedAt = time.Now().Add(-time.Second)
	stream.mu.Unlock()

	if !c.expireClientStreamTX(stream, time.Now()) {
		t.Fatal("expected stalled inflight packet to trigger reset scheduling")
	}

	stream.mu.Lock()
	defer stream.mu.Unlock()
	if !stream.ResetSent {
		t.Fatal("expected stream reset flag to be set")
	}
	if len(stream.TXInFlight) != 0 {
		t.Fatalf("expected inflight queue to be cleared, got=%d", len(stream.TXInFlight))
	}
	if len(stream.TXQueue) != 1 || stream.TXQueue[0].PacketType != Enums.PACKET_STREAM_RST {
		t.Fatalf("expected queued reset packet, queue=%+v", stream.TXQueue)
	}
}

func TestClientStreamRTTAdjustsRetryBaseAfterAck(t *testing.T) {
	c := New(config.ClientConfig{}, nil, nil)
	stream := c.createStream(9, nil)

	if err := c.queueStreamPacket(stream, Enums.PACKET_STREAM_DATA, []byte("alpha")); err != nil {
		t.Fatalf("queueStreamPacket returned error: %v", err)
	}
	packet, _, stop := nextClientStreamTX(stream, 1)
	if stop || packet == nil {
		t.Fatalf("expected inflight packet, stop=%v packet=%v", stop, packet)
	}

	sentAt := time.Now().Add(-150 * time.Millisecond)
	stream.mu.Lock()
	stream.TXInFlight[0].LastSentAt = sentAt
	stream.TXInFlight[0].Scheduled = true
	stream.mu.Unlock()

	ackClientStreamTX(stream, packet.SequenceNum, sentAt.Add(150*time.Millisecond))

	if err := c.queueStreamPacket(stream, Enums.PACKET_STREAM_DATA, []byte("beta")); err != nil {
		t.Fatalf("queueStreamPacket returned error: %v", err)
	}

	stream.mu.Lock()
	defer stream.mu.Unlock()
	if len(stream.TXQueue) != 1 {
		t.Fatalf("expected one queued packet, got=%d", len(stream.TXQueue))
	}
	if stream.TXQueue[0].RetryDelay == streamTXInitialRetryDelay {
		t.Fatalf("expected adaptive retry base to change from default, got=%v", stream.TXQueue[0].RetryDelay)
	}
	if stream.TXQueue[0].RetryDelay < streamTXMinRetryDelay || stream.TXQueue[0].RetryDelay > streamTXMaxRetryDelay {
		t.Fatalf("expected retry delay to stay clamped, got=%v", stream.TXQueue[0].RetryDelay)
	}
}

func TestHandlePackedServerControlBlocksAcksQueuedStreamPackets(t *testing.T) {
	c := New(config.ClientConfig{}, nil, nil)
	localConn, remoteConn := net.Pipe()
	defer localConn.Close()
	defer remoteConn.Close()

	stream := c.createStream(9, localConn)
	stream.mu.Lock()
	stream.TXInFlight = append(stream.TXInFlight, clientStreamTXPacket{
		PacketType:  Enums.PACKET_STREAM_DATA,
		SequenceNum: 7,
		LastSentAt:  time.Now(),
		RetryDelay:  streamTXInitialRetryDelay,
		CreatedAt:   time.Now(),
		Scheduled:   true,
	})
	stream.mu.Unlock()

	payload := make([]byte, 0, 2*arq.PackedControlBlockSize)
	payload = append(payload,
		Enums.PACKET_STREAM_DATA_ACK, 0x00, 0x09, 0x00, 0x07, 0x00, 0x00,
		Enums.PACKET_STREAM_FIN_ACK, 0x00, 0x0A, 0x00, 0x01, 0x00, 0x00,
	)

	if err := c.handlePackedServerControlBlocks(payload, time.Second); err != nil {
		t.Fatalf("handlePackedServerControlBlocks returned error: %v", err)
	}

	stream.mu.Lock()
	defer stream.mu.Unlock()
	if len(stream.TXInFlight) != 0 {
		t.Fatalf("expected packed ACK to clear inflight packet, got=%d", len(stream.TXInFlight))
	}
}

func TestHandlePackedServerControlBlocksAcksDNSRequestFragment(t *testing.T) {
	c := New(config.ClientConfig{}, nil, nil)
	sequenceNum := uint16(14)
	c.stream0Runtime.mu.Lock()
	c.stream0Runtime.dnsRequests[sequenceNum] = &stream0DNSRequestState{
		fragments: map[uint8]*stream0DNSFragmentState{
			0: {
				packet: arq.QueuedPacket{
					PacketType:     Enums.PACKET_DNS_QUERY_REQ,
					StreamID:       0,
					SequenceNum:    sequenceNum,
					FragmentID:     0,
					TotalFragments: 1,
				},
				createdAt: time.Now(),
			},
		},
	}
	c.stream0Runtime.mu.Unlock()

	payload := make([]byte, 0, arq.PackedControlBlockSize)
	payload = append(payload,
		Enums.PACKET_DNS_QUERY_REQ_ACK, 0x00, 0x00, byte(sequenceNum>>8), byte(sequenceNum), 0x00, 0x01,
	)

	if err := c.handlePackedServerControlBlocks(payload, time.Second); err != nil {
		t.Fatalf("handlePackedServerControlBlocks returned error: %v", err)
	}

	c.stream0Runtime.mu.Lock()
	defer c.stream0Runtime.mu.Unlock()
	if _, ok := c.stream0Runtime.dnsRequests[sequenceNum]; ok {
		t.Fatal("expected packed dns ack to clear queued dns request fragment")
	}
}

func TestSendScheduledPacketFailsWithoutValidConnections(t *testing.T) {
	codec, err := security.NewCodec(0, "")
	if err != nil {
		t.Fatalf("NewCodec returned error: %v", err)
	}
	c := New(config.ClientConfig{}, nil, codec)
	c.sessionID = 7
	c.sessionCookie = 9
	c.sessionReady = true

	_, sendErr := c.sendScheduledPacket(arq.QueuedPacket{
		PacketType:     Enums.PACKET_DNS_QUERY_REQ,
		StreamID:       0,
		SequenceNum:    1,
		FragmentID:     0,
		TotalFragments: 1,
		Payload:        []byte{1},
		Priority:       arq.DefaultPriorityForPacket(Enums.PACKET_DNS_QUERY_REQ),
	})
	if !errors.Is(sendErr, ErrNoValidConnections) {
		t.Fatalf("unexpected error: %v", sendErr)
	}
}

func TestStream0RuntimeProcessDequeueHandlesServerDrop(t *testing.T) {
	codec, err := security.NewCodec(0, "")
	if err != nil {
		t.Fatalf("NewCodec returned error: %v", err)
	}

	c := New(config.ClientConfig{
		LocalDNSPendingTimeoutSec: 1,
		Domains:                   []string{"v.example.com"},
	}, nil, codec)
	c.connections = []Connection{{
		Domain:        "v.example.com",
		Resolver:      "127.0.0.1",
		ResolverPort:  5353,
		ResolverLabel: "127.0.0.1:5353",
		Key:           "127.0.0.1|5353|v.example.com",
		IsValid:       true,
	}}
	c.connectionsByKey = map[string]int{c.connections[0].Key: 0}
	c.rebuildBalancer()
	c.sessionID = 7
	c.sessionCookie = 9
	c.sessionReady = true
	c.sessionInitPayload = []byte{1, 2, 3}
	c.sessionInitReady = true

	c.exchangeQueryFn = func(conn Connection, packet []byte, timeout time.Duration) ([]byte, error) {
		parsed, err := DnsParser.ParsePacketLite(packet)
		if err != nil {
			t.Fatalf("ParsePacketLite returned error: %v", err)
		}
		if !parsed.HasQuestion {
			t.Fatal("expected dns question in scheduled packet")
		}
		response, err := DnsParser.BuildVPNResponsePacket(packet, parsed.FirstQuestion.Name, VpnProto.Packet{
			SessionID:  7,
			PacketType: Enums.PACKET_ERROR_DROP,
			Payload:    []byte("INV"),
		}, false)
		if err != nil {
			t.Fatalf("BuildVPNResponsePacket returned error: %v", err)
		}
		return response, nil
	}

	c.stream0Runtime.processDequeue(arq.QueuedPacket{
		PacketType:     Enums.PACKET_DNS_QUERY_REQ,
		StreamID:       0,
		SequenceNum:    1,
		FragmentID:     0,
		TotalFragments: 1,
		Payload:        []byte{1},
		Priority:       arq.DefaultPriorityForPacket(Enums.PACKET_DNS_QUERY_REQ),
	})

	if !c.reconnectPending.Load() {
		t.Fatal("expected reconnect to be pending after queued main-packet drop response")
	}
	if c.SessionReady() {
		t.Fatal("expected session to be cleared after queued main-packet drop response")
	}
	if c.sessionInitReady || len(c.sessionInitPayload) != 0 {
		t.Fatal("expected session init state to be reset after queued main-packet drop response")
	}
}

func TestSelectTargetConnectionsForPacketPrefersStickyStreamResolver(t *testing.T) {
	c := New(config.ClientConfig{
		PacketDuplicationCount: 2,
	}, nil, nil)
	c.connections = []Connection{
		{Key: "a", Domain: "a.example.com", Resolver: "1.1.1.1", ResolverPort: 53, ResolverLabel: "1.1.1.1:53", IsValid: true},
		{Key: "b", Domain: "b.example.com", Resolver: "2.2.2.2", ResolverPort: 53, ResolverLabel: "2.2.2.2:53", IsValid: true},
		{Key: "c", Domain: "c.example.com", Resolver: "3.3.3.3", ResolverPort: 53, ResolverLabel: "3.3.3.3:53", IsValid: true},
	}
	c.connectionsByKey = map[string]int{"a": 0, "b": 1, "c": 2}
	c.rebuildBalancer()

	stream := c.createStream(11, nil)
	defer c.deleteStream(stream.ID)

	stream.mu.Lock()
	stream.PreferredServerKey = "b"
	stream.LastResolverFailover = time.Now()
	stream.mu.Unlock()

	selected, err := c.selectTargetConnectionsForPacket(Enums.PACKET_STREAM_DATA, stream.ID)
	if err != nil {
		t.Fatalf("selectTargetConnectionsForPacket returned error: %v", err)
	}
	if len(selected) != 2 {
		t.Fatalf("unexpected selected count: got=%d want=2", len(selected))
	}
	if selected[0].Key != "b" {
		t.Fatalf("expected preferred resolver first, got=%q", selected[0].Key)
	}
}

func TestSelectTargetConnectionsForPacketFailsOverOnResendStreak(t *testing.T) {
	c := New(config.ClientConfig{
		PacketDuplicationCount:                1,
		StreamResolverFailoverResendThreshold: 1,
		StreamResolverFailoverCooldownSec:     0.1,
	}, nil, nil)
	c.connections = []Connection{
		{Key: "a", Domain: "a.example.com", Resolver: "1.1.1.1", ResolverPort: 53, ResolverLabel: "1.1.1.1:53", IsValid: true},
		{Key: "b", Domain: "b.example.com", Resolver: "2.2.2.2", ResolverPort: 53, ResolverLabel: "2.2.2.2:53", IsValid: true},
	}
	c.connectionsByKey = map[string]int{"a": 0, "b": 1}
	c.rebuildBalancer()

	stream := c.createStream(12, nil)
	defer c.deleteStream(stream.ID)

	stream.mu.Lock()
	stream.PreferredServerKey = "a"
	stream.LastResolverFailover = time.Now().Add(-2 * time.Second)
	stream.mu.Unlock()

	selected, err := c.selectTargetConnectionsForPacket(Enums.PACKET_STREAM_RESEND, stream.ID)
	if err != nil {
		t.Fatalf("selectTargetConnectionsForPacket returned error: %v", err)
	}
	if len(selected) != 1 {
		t.Fatalf("unexpected selected count: got=%d want=1", len(selected))
	}
	if selected[0].Key != "b" {
		t.Fatalf("expected resend failover to switch preferred resolver, got=%q", selected[0].Key)
	}
}

func TestSelectTargetConnectionsForPacketUsesSetupDuplicationCount(t *testing.T) {
	c := New(config.ClientConfig{
		PacketDuplicationCount:      1,
		SetupPacketDuplicationCount: 3,
	}, nil, nil)
	c.connections = []Connection{
		{Key: "a", Domain: "a.example.com", Resolver: "1.1.1.1", ResolverPort: 53, ResolverLabel: "1.1.1.1:53", IsValid: true},
		{Key: "b", Domain: "b.example.com", Resolver: "2.2.2.2", ResolverPort: 53, ResolverLabel: "2.2.2.2:53", IsValid: true},
		{Key: "c", Domain: "c.example.com", Resolver: "3.3.3.3", ResolverPort: 53, ResolverLabel: "3.3.3.3:53", IsValid: true},
	}
	c.connectionsByKey = map[string]int{"a": 0, "b": 1, "c": 2}
	c.rebuildBalancer()

	selected, err := c.selectTargetConnectionsForPacket(Enums.PACKET_STREAM_SYN, 99)
	if err != nil {
		t.Fatalf("selectTargetConnectionsForPacket returned error: %v", err)
	}
	if len(selected) != 3 {
		t.Fatalf("unexpected selected count: got=%d want=3", len(selected))
	}
}

func extractTestTunnelLabels(qName string, baseDomain string) string {
	suffix := "." + baseDomain
	if len(qName) <= len(suffix) || qName[len(qName)-len(suffix):] != suffix {
		return ""
	}
	labels := qName[:len(qName)-len(suffix)]
	out := make([]byte, 0, len(labels))
	for i := 0; i < len(labels); i++ {
		if labels[i] != '.' {
			out = append(out, labels[i])
		}
	}
	return string(out)
}

func buildClientTestDNSQuery(id uint16, name string, qType uint16, qClass uint16) []byte {
	packet := []byte{
		byte(id >> 8), byte(id),
		0x01, 0x00,
		0x00, 0x01,
		0x00, 0x00,
		0x00, 0x00,
		0x00, 0x00,
	}
	packet = append(packet, encodeClientTestDNSName(name)...)
	packet = append(packet, byte(qType>>8), byte(qType), byte(qClass>>8), byte(qClass))
	return packet
}

func encodeClientTestDNSName(name string) []byte {
	if name == "" {
		return []byte{0x00}
	}

	encoded := make([]byte, 0, len(name)+2)
	labelStart := 0
	for i := 0; i <= len(name); i++ {
		if i < len(name) && name[i] != '.' {
			continue
		}
		label := name[labelStart:i]
		encoded = append(encoded, byte(len(label)))
		encoded = append(encoded, label...)
		labelStart = i + 1
	}
	encoded = append(encoded, 0x00)
	return encoded
}
