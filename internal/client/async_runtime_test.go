// ==============================================================================
// MasterDnsVPN
// Author: MasterkinG32
// Github: https://github.com/masterking32
// Year: 2026
// ==============================================================================

package client

import (
	"context"
	"net"
	"testing"
	"time"

	"masterdnsvpn-go/internal/arq"
	"masterdnsvpn-go/internal/config"
	"masterdnsvpn-go/internal/logger"
	"masterdnsvpn-go/internal/security"
)

func createTestClient(t *testing.T) *Client {
	cfg := config.ClientConfig{
		LogLevel: "debug",
		Domains:  []string{"example.com"},
		Resolvers: []config.ResolverAddress{
			{IP: "8.8.8.8", Port: 53},
		},
		TXChannelSize:        10,
		RXChannelSize:        10,
		TunnelReaderWorkers:  1,
		TunnelWriterWorkers:  1,
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
}

func TestClearTxSignal(t *testing.T) {
	c := createTestClient(t)
	c.txSignal = make(chan struct{}, 5)
	c.txSignal <- struct{}{}
	c.txSignal <- struct{}{}

	c.clearTxSignal()

	select {
	case <-c.txSignal:
		t.Fatal("txSignal should be empty")
	default:
	}
}

func TestClearTxSpaceSignal(t *testing.T) {
	c := createTestClient(t)
	c.txSpaceSignal = make(chan struct{}, 5)
	c.txSpaceSignal <- struct{}{}
	c.txSpaceSignal <- struct{}{}

	c.clearTxSpaceSignal()

	select {
	case <-c.txSpaceSignal:
		t.Fatal("txSpaceSignal should be empty")
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

func TestDrainQueues(t *testing.T) {
	c := createTestClient(t)
	c.txChannel = make(chan asyncPacket, 5)
	c.rxChannel = make(chan asyncReadPacket, 5)

	c.txChannel <- asyncPacket{}
	c.rxChannel <- asyncReadPacket{data: make([]byte, 10)}

	c.drainQueues()

	if len(c.txChannel) != 0 {
		t.Errorf("expected txChannel empty, got %d", len(c.txChannel))
	}
	if len(c.rxChannel) != 0 {
		t.Errorf("expected rxChannel empty, got %d", len(c.rxChannel))
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := c.StartAsyncRuntime(ctx)
	if err != nil {
		t.Logf("StartAsyncRuntime failed (expected if ports are busy): %v", err)
		return
	}

	if c.tunnelConn == nil {
		t.Error("expected tunnelConn not nil")
	}
	if c.asyncCancel == nil {
		t.Error("expected asyncCancel not nil")
	}

	c.StopAsyncRuntime()
}
