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
	"net"
	"testing"
	"time"

	"masterdnsvpn-go/internal/arq"
	"masterdnsvpn-go/internal/config"
	DnsParser "masterdnsvpn-go/internal/dnsparser"
	Enums "masterdnsvpn-go/internal/enums"
	"masterdnsvpn-go/internal/logger"
	"masterdnsvpn-go/internal/mlq"
	"masterdnsvpn-go/internal/security"
	VpnProto "masterdnsvpn-go/internal/vpnproto"
)

func createTestClient(t *testing.T) *Client {
	cfg := config.ClientConfig{
		LogLevel: "debug",
		Domains:  []string{"example.com"},
		Resolvers: []config.ResolverAddress{
			{IP: "8.8.8.8", Port: 53},
		},
		RXChannelSize:        10,
		RX_TX_Workers:        1,
		TunnelProcessWorkers: 1,
		DataEncryptionMethod: 1,
		EncryptionKey:        "testkey",
	}
	log := logger.New("TestLogger", "debug")
	codec, err := security.NewCodec(1, "testkey")
	if err != nil {
		t.Fatalf("failed to create codec: %v", err)
	}

	return New(cfg, log, codec)
}

func TestResetRuntimeBindings(t *testing.T) {
	c := createTestClient(t)
	c.last_stream_id = 10
	c.sessionID = 1
	c.sessionReady = true
	c.socksRateLimit.RecordFailure("10.0.0.1")
	oldLimiter := c.socksRateLimit

	c.resetRuntimeBindings(true)

	if c.last_stream_id != 0 {
		t.Errorf("expected last_stream_id 0, got %d", c.last_stream_id)
	}

	if c.sessionID != 0 {
		t.Errorf("expected sessionID 0, got %d", c.sessionID)
	}

	if c.sessionReady {
		t.Error("expected sessionReady false")
	}

	if c.socksRateLimit == nil {
		t.Fatal("expected socksRateLimit to be reinitialized")
	}

	if c.socksRateLimit != oldLimiter {
		t.Fatal("expected socksRateLimit instance to be reset in place")
	}

	if c.socksRateLimit.IsBlocked("10.0.0.1") {
		t.Fatal("expected reset to clear prior SOCKS rate-limit state")
	}
}

func TestClearDispatchSignal(t *testing.T) {
	c := createTestClient(t)
	c.dispatchSignal = make(chan struct{}, 5)
	c.dispatchSignal <- struct{}{}
	c.dispatchSignal <- struct{}{}

	c.clearDispatchSignal()

	select {
	case <-c.dispatchSignal:
		t.Fatal("dispatchSignal should be empty")
	default:
	}
}

func TestClearPlannerQueueSpaceSignal(t *testing.T) {
	c := createTestClient(t)
	c.plannerQueueSpaceSignal = make(chan struct{}, 5)
	c.plannerQueueSpaceSignal <- struct{}{}
	c.plannerQueueSpaceSignal <- struct{}{}

	c.clearPlannerQueueSpaceSignal()

	select {
	case <-c.plannerQueueSpaceSignal:
		t.Fatal("plannerQueueSpaceSignal should be empty")
	default:
	}
}

func TestOnRXDropIncrementsCounter(t *testing.T) {
	c := createTestClient(t)
	addr := &net.UDPAddr{IP: net.IPv4(8, 8, 8, 8), Port: 53}

	c.onRXDrop(addr)

	if got := c.rxDroppedPackets.Load(); got != 1 {
		t.Fatalf("expected rxDroppedPackets=1, got %d", got)
	}
}

func TestTrackResolverSendBoundsResolverPendingGrowth(t *testing.T) {
	c := createTestClient(t)
	base := time.Now()

	for i := 0; i < resolverPendingHardCap+32; i++ {
		c.balancer.pendingStoreForTest(balancerResolverSampleKey{
			resolverAddr: "127.0.0.1:5300",
			dnsID:        uint16(i),
		}, balancerResolverSample{
			serverKey: "resolver-a",
			sentAt:    base.Add(-time.Minute),
		})
	}

	packet := []byte{0x12, 0x34}
	c.balancer.TrackResolverSend(packet, "127.0.0.1:5300", "", "resolver-a", base, c.tunnelPacketTimeout)

	pendingCount := c.balancer.pendingCount()
	_, inserted := c.balancer.pendingLookupForTest(balancerResolverSampleKey{
		resolverAddr: "127.0.0.1:5300",
		dnsID:        binary.BigEndian.Uint16(packet),
	})

	if pendingCount > resolverPendingHardCap {
		t.Fatalf("expected resolverPending to stay bounded, got=%d hardCap=%d", pendingCount, resolverPendingHardCap)
	}
	if !inserted {
		t.Fatal("expected latest resolver sample to remain tracked")
	}
}

func TestDrainQueues(t *testing.T) {
	c := createTestClient(t)
	c.plannerQueue = make(chan plannerTask, 5)
	c.encodedTXChannel = make(chan writerTask, 5)
	c.rxChannel = make(chan asyncReadPacket, 5)

	c.plannerQueue <- plannerTask{}
	c.encodedTXChannel <- writerTask{}
	c.rxChannel <- asyncReadPacket{data: make([]byte, 10)}

	c.drainQueues()

	if len(c.plannerQueue) != 0 {
		t.Errorf("expected plannerQueue empty, got %d", len(c.plannerQueue))
	}
	if len(c.encodedTXChannel) != 0 {
		t.Errorf("expected encodedTXChannel empty, got %d", len(c.encodedTXChannel))
	}
	if len(c.rxChannel) != 0 {
		t.Errorf("expected rxChannel empty, got %d", len(c.rxChannel))
	}
}

func TestApplyPlannerNoConnectionPolicyDropsControlTask(t *testing.T) {
	c := createTestClient(t)
	stream := &Stream_client{client: c, StreamID: 9}
	item := &clientStreamTXPacket{
		PacketType: Enums.PACKET_STREAM_SYN,
		Payload:    []byte("syn"),
	}

	c.applyPlannerNoConnectionPolicy(plannerTask{
		opts:     VpnProto.BuildOptions{PacketType: Enums.PACKET_STREAM_SYN, StreamID: stream.StreamID},
		item:     item,
		selected: stream,
	})

	if item.Payload != nil {
		t.Fatal("expected control task to be released when no connection is available")
	}
}

func TestApplyPlannerNoConnectionPolicyRequeuesDataTask(t *testing.T) {
	c := createTestClient(t)
	stream := &Stream_client{
		client:   c,
		StreamID: 10,
		txQueue:  mlq.New[*clientStreamTXPacket](8),
	}
	item := &clientStreamTXPacket{
		PacketType:  Enums.PACKET_STREAM_DATA,
		SequenceNum: 7,
		Payload:     []byte("data"),
	}

	c.applyPlannerNoConnectionPolicy(plannerTask{
		opts:     VpnProto.BuildOptions{PacketType: Enums.PACKET_STREAM_DATA, StreamID: stream.StreamID},
		item:     item,
		selected: stream,
	})

	if item.Payload != nil {
		t.Fatal("expected original dequeued data task to be released after requeue")
	}
	if stream.txQueue == nil || stream.txQueue.FastSize() != 1 {
		t.Fatalf("expected data task to be requeued, got queue size %d", stream.txQueue.FastSize())
	}
	queued, _, ok := stream.txQueue.Peek()
	if !ok || queued == nil {
		t.Fatal("expected requeued packet to be present")
	}
	if queued.PacketType != Enums.PACKET_STREAM_DATA || queued.SequenceNum != 7 {
		t.Fatalf("unexpected requeued packet: type=%d seq=%d", queued.PacketType, queued.SequenceNum)
	}
}

func TestRequiredWriterSlotsForFramesUsesSingleBatchSlot(t *testing.T) {
	c := createTestClient(t)

	frames := []encodedOutboundDatagram{
		{serverKey: "a", packet: []byte("one")},
		{serverKey: "b", packet: []byte("two")},
		{serverKey: "c", packet: []byte("three")},
	}

	if got := c.requiredWriterSlotsForFrames(frames); got != 1 {
		t.Fatalf("expected one writer queue slot for a multi-frame batch, got=%d", got)
	}

	if got := c.requiredWriterSlotsForFrames(nil); got != 0 {
		t.Fatalf("expected zero writer queue slots for empty frames, got=%d", got)
	}
}

func TestRequestSessionRestart(t *testing.T) {
	c := createTestClient(t)
	c.sessionResetSignal = make(chan struct{}, 1)

	c.requestSessionRestart("test reason")
	if !c.runtimeResetPending.Load() {
		t.Error("expected runtimeResetPending true")
	}

	select {
	case <-c.sessionResetSignal:
	default:
		t.Fatal("sessionResetSignal should have received a signal")
	}

	c.clearRuntimeResetRequest()
	if c.runtimeResetPending.Load() {
		t.Error("expected runtimeResetPending false")
	}
}

func TestStopAsyncRuntime(t *testing.T) {
	c := createTestClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	c.asyncCancel = cancel

	c.asyncWG.Add(1)
	go func() {
		defer c.asyncWG.Done()
		<-ctx.Done()
	}()

	c.StopAsyncRuntime()

	if c.asyncCancel != nil {
		t.Error("expected asyncCancel nil")
	}
}

func TestAsyncStreamCleanupWorker(t *testing.T) {
	c := createTestClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c.streamsMu.Lock()
	stream := &Stream_client{
		StreamID: 1,
	}
	a := arq.NewARQ(1, 1, nil, nil, 1400, nil, arq.Config{
		WindowSize: 300,
		RTO:        1.0,
		MaxRTO:     8.0,
	})
	stream.Stream = a
	c.active_streams[1] = stream
	c.streamsMu.Unlock()

	c.asyncWG.Add(1)
	go c.asyncStreamCleanupWorker(ctx)

	// Wait for a tick
	time.Sleep(1200 * time.Millisecond)

	cancel()
	c.asyncWG.Wait()
}

func TestStartAsyncRuntime(t *testing.T) {
	c := createTestClient(t)
	c.cfg.ListenIP = "127.0.0.1"
	c.cfg.ListenPort = 0
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := c.StartAsyncRuntime(ctx)
	if err != nil {
		t.Logf("StartAsyncRuntime failed (expected if ports are busy): %v", err)
		return
	}

	if len(c.tunnelConns) != c.tunnelRX_TX_Workers {
		t.Fatalf("expected %d tunnel sockets, got %d", c.tunnelRX_TX_Workers, len(c.tunnelConns))
	}

	if c.asyncCancel == nil {
		t.Error("expected asyncCancel not nil")
	}

	c.StopAsyncRuntime()
}

func TestStartAsyncRuntimeCleansUpOnListenerStartFailure(t *testing.T) {
	c := createTestClient(t)
	c.cfg.ListenIP = ""
	c.cfg.ListenPort = 0

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := c.StartAsyncRuntime(ctx)
	if err == nil {
		t.Fatal("expected StartAsyncRuntime to fail for invalid listener address")
	}
	if c.asyncCancel != nil {
		t.Fatal("expected asyncCancel to be cleared after startup failure")
	}
	if len(c.tunnelConns) != 0 {
		t.Fatal("expected tunnel sockets to be closed after startup failure")
	}
}

func TestResolverHealthLoopCollectsResolverTimeoutsWhenAutoDisableEnabled(t *testing.T) {
	c := createTestClient(t)
	c.cfg.AutoDisableTimeoutServers = true
	c.cfg.RecheckInactiveServersEnabled = false

	now := time.Now()
	c.nowFn = func() time.Time {
		return now
	}

	addr := &net.UDPAddr{IP: net.ParseIP("8.8.8.8"), Port: 53}
	serverKey := "resolver-a"
	key := balancerResolverSampleKey{
		resolverAddr: addr.String(),
		dnsID:        0x1337,
	}

	c.balancer.SetAutoDisableConfig(
		true,
		180*time.Second,
	)

	c.balancer.pendingStoreForTest(key, balancerResolverSample{
		serverKey: serverKey,
		sentAt:    now.Add(-10 * time.Second),
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go c.runResolverHealthLoop(ctx)

	waitForResolverHealthCondition(t, 3*time.Second, func() bool {
		sample, ok := c.balancer.pendingLookupForTest(key)
		return ok && sample.timedOut
	}, "expected resolver timeout sample to be collected when auto-disable is enabled")
}

func TestHandleInboundPacketTreatsMissingTXTAsResolverSuccess(t *testing.T) {
	c := buildTestClientWithResolvers(config.ClientConfig{}, "a", "b", "c", "d")
	addr := &net.UDPAddr{IP: net.ParseIP("8.8.8.8"), Port: 53}

	query, err := DnsParser.BuildTXTQuestionPacket("x.v.example.com", 16, 4096)
	if err != nil {
		t.Fatalf("BuildTXTQuestionPacket returned error: %v", err)
	}
	response, err := DnsParser.BuildEmptyNoErrorResponse(query)
	if err != nil {
		t.Fatalf("BuildEmptyNoErrorResponse returned error: %v", err)
	}

	dnsID := binary.BigEndian.Uint16(response[:2])
	c.balancer.pendingStoreForTest(balancerResolverSampleKey{
		resolverAddr: addr.String(),
		dnsID:        dnsID,
	}, balancerResolverSample{
		serverKey: "a",
		sentAt:    time.Now().Add(-200 * time.Millisecond),
	})

	c.handleInboundPacket(response, addr, "")

	if got := c.balancer.pendingCount(); got != 0 {
		t.Fatalf("expected resolverPending to be cleared after empty DNS success, got=%d", got)
	}
}

func TestHandleInboundPacketTreatsServerFailureWithoutTXTAsResolverFailure(t *testing.T) {
	c := buildTestClientWithResolvers(config.ClientConfig{
		AutoDisableTimeoutServers:       true,
		AutoDisableTimeoutWindowSeconds: 10,
	}, "a", "b", "c", "d")
	addr := &net.UDPAddr{IP: net.ParseIP("8.8.8.8"), Port: 53}

	query, err := DnsParser.BuildTXTQuestionPacket("x.v.example.com", Enums.DNS_RECORD_TYPE_TXT, 4096)
	if err != nil {
		t.Fatalf("BuildTXTQuestionPacket returned error: %v", err)
	}
	response, err := DnsParser.BuildServerFailureResponse(query)
	if err != nil {
		t.Fatalf("BuildServerFailureResponse returned error: %v", err)
	}

	dnsID := binary.BigEndian.Uint16(response[:2])
	c.balancer.pendingStoreForTest(balancerResolverSampleKey{
		resolverAddr: addr.String(),
		dnsID:        dnsID,
	}, balancerResolverSample{
		serverKey: "a",
		sentAt:    time.Now().Add(-200 * time.Millisecond),
	})

	c.handleInboundPacket(response, addr, "")

	if got := c.balancer.pendingCount(); got != 0 {
		t.Fatalf("expected resolverPending to be cleared after SERVFAIL response, got=%d", got)
	}
	stats := c.balancer.statsForKey("a")
	if stats == nil {
		t.Fatal("expected stats for resolver a to exist")
	}
	if stats.windowLost.Load() != 1 {
		t.Fatalf("expected one timeout-window failure after SERVFAIL response, got=%d", stats.windowLost.Load())
	}
}
