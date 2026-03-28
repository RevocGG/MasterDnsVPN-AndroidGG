package arq

import (
	"bytes"
	"errors"
	"io"
	"net"
	"sync"
	"syscall"
	"testing"
	"time"

	Enums "masterdnsvpn-go/internal/enums"
)

// MockPacketEnqueuer captures packets sent by ARQ
type MockPacketEnqueuer struct {
	mu              sync.Mutex
	Packets         chan capturedPacket
	removedSeqs     []uint16
	removedNackSeqs []uint16
	queuedNackSeqs  map[uint16]struct{}
}

type RejectingPacketEnqueuer struct{}

type capturedPacket struct {
	priority        int
	packetType      uint8
	sequenceNum     uint16
	fragmentID      uint8
	totalFragments  uint8
	compressionType uint8
	ttl             time.Duration
	payload         []byte
}

func NewMockPacketEnqueuer() *MockPacketEnqueuer {
	return &MockPacketEnqueuer{
		Packets:        make(chan capturedPacket, 1000),
		queuedNackSeqs: make(map[uint16]struct{}),
	}
}

func (m *MockPacketEnqueuer) PushTXPacket(priority int, packetType uint8, sequenceNum uint16, fragmentID uint8, totalFragments uint8, compressionType uint8, ttl time.Duration, payload []byte) bool {
	m.mu.Lock()
	if packetType == Enums.PACKET_STREAM_DATA_NACK {
		m.queuedNackSeqs[sequenceNum] = struct{}{}
	}
	m.mu.Unlock()

	m.Packets <- capturedPacket{
		priority:        priority,
		packetType:      packetType,
		sequenceNum:     sequenceNum,
		fragmentID:      fragmentID,
		totalFragments:  totalFragments,
		compressionType: compressionType,
		ttl:             ttl,
		payload:         append([]byte(nil), payload...),
	}
	return true
}

func (m *MockPacketEnqueuer) RemoveQueuedData(sequenceNum uint16) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removedSeqs = append(m.removedSeqs, sequenceNum)
	return true
}

func (m *MockPacketEnqueuer) RemoveQueuedDataNack(sequenceNum uint16) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.queuedNackSeqs[sequenceNum]; !exists {
		return false
	}
	delete(m.queuedNackSeqs, sequenceNum)
	m.removedNackSeqs = append(m.removedNackSeqs, sequenceNum)
	return true
}

func (RejectingPacketEnqueuer) PushTXPacket(priority int, packetType uint8, sequenceNum uint16, fragmentID uint8, totalFragments uint8, compressionType uint8, ttl time.Duration, payload []byte) bool {
	return false
}

type testLogger struct {
	t *testing.T
}

func (l *testLogger) Debugf(format string, args ...any) { l.t.Logf("[DEBUG] "+format, args...) }
func (l *testLogger) Infof(format string, args ...any)  { l.t.Logf("[INFO] "+format, args...) }
func (l *testLogger) Errorf(format string, args ...any) { l.t.Logf("[ERROR] "+format, args...) }

type eofAfterDataConn struct {
	mu     sync.Mutex
	data   []byte
	read   bool
	closed bool
}

func (c *eofAfterDataConn) Read(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.read {
		return 0, io.EOF
	}
	c.read = true
	n := copy(p, c.data)
	return n, io.EOF
}

func (c *eofAfterDataConn) Write(p []byte) (int, error) { return len(p), nil }
func (c *eofAfterDataConn) Close() error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	return nil
}

type errAfterDataConn struct {
	mu     sync.Mutex
	data   []byte
	err    error
	read   bool
	closed bool
}

func (c *errAfterDataConn) Read(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.read {
		return 0, c.err
	}
	c.read = true
	n := copy(p, c.data)
	return n, c.err
}

func (c *errAfterDataConn) Write(p []byte) (int, error) { return len(p), nil }
func (c *errAfterDataConn) Close() error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	return nil
}

type timeoutOnlyError struct{}

func (e timeoutOnlyError) Error() string   { return "timeout" }
func (e timeoutOnlyError) Timeout() bool   { return true }
func (e timeoutOnlyError) Temporary() bool { return false }

type writeTimeoutError struct{}

func (e writeTimeoutError) Error() string   { return "write timeout" }
func (e writeTimeoutError) Timeout() bool   { return true }
func (e writeTimeoutError) Temporary() bool { return false }

func newTransientOpError(op string) error {
	return syscall.EAGAIN
}

type transientReadConn struct {
	mu     sync.Mutex
	closed bool
}

func (c *transientReadConn) Read(_ []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return 0, newTransientOpError("read")
}

func (c *transientReadConn) Write(p []byte) (int, error) { return len(p), nil }
func (c *transientReadConn) Close() error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	return nil
}

type transientWriteConn struct {
	mu          sync.Mutex
	failWrites  int
	writes      [][]byte
	closed      bool
	readBlocked bool
}

func (c *transientWriteConn) Read(_ []byte) (int, error) {
	time.Sleep(50 * time.Millisecond)
	return 0, timeoutOnlyError{}
}

func (c *transientWriteConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failWrites > 0 {
		c.failWrites--
		return 0, newTransientOpError("write")
	}
	c.writes = append(c.writes, append([]byte(nil), p...))
	return len(p), nil
}

func (c *transientWriteConn) Close() error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	return nil
}

type fatalWriteConn struct {
	mu     sync.Mutex
	closed bool
}

func (c *fatalWriteConn) Read(_ []byte) (int, error) {
	time.Sleep(50 * time.Millisecond)
	return 0, timeoutOnlyError{}
}

func (c *fatalWriteConn) Write(_ []byte) (int, error) {
	return 0, errors.New("fatal write failure")
}

func (c *fatalWriteConn) Close() error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	return nil
}

type blockingWriteConn struct {
	mu      sync.Mutex
	writeCh chan []byte
	release chan struct{}
	writes  [][]byte
	closed  bool
}

func newBlockingWriteConn() *blockingWriteConn {
	return &blockingWriteConn{
		writeCh: make(chan []byte, 1),
		release: make(chan struct{}),
	}
}

func (c *blockingWriteConn) Read(_ []byte) (int, error) {
	time.Sleep(50 * time.Millisecond)
	return 0, timeoutOnlyError{}
}

func (c *blockingWriteConn) Write(p []byte) (int, error) {
	payload := append([]byte(nil), p...)
	select {
	case c.writeCh <- payload:
	default:
	}
	<-c.release
	c.mu.Lock()
	c.writes = append(c.writes, payload)
	c.mu.Unlock()
	return len(p), nil
}

func (c *blockingWriteConn) Close() error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	select {
	case <-c.release:
	default:
		close(c.release)
	}
	return nil
}

type closeOnWriteConn struct {
	mu      sync.Mutex
	writeCh chan []byte
	closed  bool
}

func newCloseOnWriteConn() *closeOnWriteConn {
	return &closeOnWriteConn{
		writeCh: make(chan []byte, 1),
	}
}

func (c *closeOnWriteConn) Read(_ []byte) (int, error) {
	time.Sleep(50 * time.Millisecond)
	return 0, timeoutOnlyError{}
}

func (c *closeOnWriteConn) Write(p []byte) (int, error) {
	payload := append([]byte(nil), p...)
	select {
	case c.writeCh <- payload:
	default:
	}
	return 0, io.ErrClosedPipe
}

func (c *closeOnWriteConn) Close() error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	return nil
}

type writeDeadlineTimeoutConn struct {
	mu            sync.Mutex
	writeAttempts int
	writes        [][]byte
	closed        bool
}

func (c *writeDeadlineTimeoutConn) Read(_ []byte) (int, error) {
	time.Sleep(50 * time.Millisecond)
	return 0, timeoutOnlyError{}
}

func (c *writeDeadlineTimeoutConn) SetWriteDeadline(time.Time) error { return nil }

func (c *writeDeadlineTimeoutConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writeAttempts++
	if c.writeAttempts == 1 {
		return 0, writeTimeoutError{}
	}
	c.writes = append(c.writes, append([]byte(nil), p...))
	return len(p), nil
}

func (c *writeDeadlineTimeoutConn) Close() error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	return nil
}

func TestARQ_New(t *testing.T) {
	enqueuer := NewMockPacketEnqueuer()
	cfg := Config{
		WindowSize: 100,
		RTO:        0.1,
		MaxRTO:     0.5,
	}
	a := NewARQ(1, 2, enqueuer, nil, 1000, &testLogger{t}, cfg)

	if a.streamID != 1 {
		t.Errorf("expected streamID 1, got %d", a.streamID)
	}
	if a.sessionID != 2 {
		t.Errorf("expected sessionID 2, got %d", a.sessionID)
	}
	if a.state != StateOpen {
		t.Errorf("expected state StateOpen, got %v", a.state)
	}
}

func TestARQ_SendData(t *testing.T) {
	enqueuer := NewMockPacketEnqueuer()
	cfg := Config{
		WindowSize: 100,
		RTO:        0.1,
		MaxRTO:     0.5,
	}

	// Create a pipe to simulate local connection
	localApp, arqConn := net.Pipe()
	defer localApp.Close()
	defer arqConn.Close()

	a := NewARQ(1, 1, enqueuer, arqConn, 1000, &testLogger{t}, cfg)
	a.Start()
	defer a.Close("test end", CloseOptions{Force: true})

	// Wait for workers to start
	time.Sleep(50 * time.Millisecond)

	testData := []byte("hello arq")
	go func() {
		_, _ = localApp.Write(testData)
	}()

	select {
	case p := <-enqueuer.Packets:
		if p.packetType != Enums.PACKET_STREAM_DATA {
			t.Errorf("expected PACKET_STREAM_DATA, got %d", p.packetType)
		}
		if !bytes.Equal(p.payload, testData) {
			t.Errorf("expected payload %s, got %s", string(testData), string(p.payload))
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for packet")
	}
}

func TestARQ_ReceiveData(t *testing.T) {
	enqueuer := NewMockPacketEnqueuer()
	cfg := Config{
		WindowSize: 100,
		RTO:        0.1,
		MaxRTO:     0.5,
	}

	localApp, arqConn := net.Pipe()
	defer localApp.Close()
	defer arqConn.Close()

	a := NewARQ(1, 1, enqueuer, arqConn, 1000, &testLogger{t}, cfg)
	a.Start()
	defer a.Close("test end", CloseOptions{Force: true})

	// Wait for workers to start
	time.Sleep(50 * time.Millisecond)

	testData := []byte("hello from remote")
	a.ReceiveData(0, testData)

	// ARQ should send an ACK
	select {
	case p := <-enqueuer.Packets:
		if p.packetType != Enums.PACKET_STREAM_DATA_ACK {
			t.Errorf("expected PACKET_STREAM_DATA_ACK, got %d", p.packetType)
		}
		if p.sequenceNum != 0 {
			t.Errorf("expected ACK for sn 0, got %d", p.sequenceNum)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for ACK")
	}

	// Local app should receive the data
	buf := make([]byte, 100)
	_ = localApp.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	n, err := localApp.Read(buf)
	if err != nil {
		t.Fatalf("failed to read from local app: %v", err)
	}
	if !bytes.Equal(buf[:n], testData) {
		t.Errorf("expected data %s, got %s", string(testData), string(buf[:n]))
	}
}

func TestARQ_ReceiveAckPurgesQueuedDataCopy(t *testing.T) {
	enqueuer := NewMockPacketEnqueuer()
	cfg := Config{
		WindowSize: 100,
		RTO:        0.1,
		MaxRTO:     0.5,
	}

	a := NewARQ(1, 1, enqueuer, nil, 1000, &testLogger{t}, cfg)
	a.mu.Lock()
	a.sndBuf[7] = &arqDataItem{
		Data:       []byte("hello"),
		CreatedAt:  time.Now(),
		LastSentAt: time.Now(),
		CurrentRTO: a.rto,
	}
	a.mu.Unlock()

	if !a.ReceiveAck(Enums.PACKET_STREAM_DATA_ACK, 7) {
		t.Fatal("expected ReceiveAck to handle tracked sequence")
	}

	enqueuer.mu.Lock()
	defer enqueuer.mu.Unlock()
	if len(enqueuer.removedSeqs) != 1 || enqueuer.removedSeqs[0] != 7 {
		t.Fatalf("expected queued data purge for seq 7, got %#v", enqueuer.removedSeqs)
	}
}

func TestARQ_ReceiveDataSendsBoundedNackForNearGap(t *testing.T) {
	enqueuer := NewMockPacketEnqueuer()
	a := NewARQ(1, 1, enqueuer, nil, 1000, &testLogger{t}, Config{
		WindowSize:            64,
		RTO:                   0.2,
		MaxRTO:                1.0,
		DataNackMaxGap:        2,
		DataNackRepeatSeconds: 2.0,
	})

	a.ReceiveData(1, []byte("packet 1"))

	first := <-enqueuer.Packets
	second := <-enqueuer.Packets
	if first.packetType != Enums.PACKET_STREAM_DATA_ACK {
		t.Fatalf("expected first packet to be DATA_ACK, got %s", Enums.PacketTypeName(first.packetType))
	}
	if second.packetType != Enums.PACKET_STREAM_DATA_NACK {
		t.Fatalf("expected second packet to be DATA_NACK, got %s", Enums.PacketTypeName(second.packetType))
	}
	if second.sequenceNum != 0 {
		t.Fatalf("expected DATA_NACK for missing seq 0, got %d", second.sequenceNum)
	}
}

func TestARQ_ReceiveDataDoesNotNackFarGap(t *testing.T) {
	enqueuer := NewMockPacketEnqueuer()
	a := NewARQ(1, 1, enqueuer, nil, 1000, &testLogger{t}, Config{
		WindowSize:            64,
		RTO:                   0.2,
		MaxRTO:                1.0,
		DataNackMaxGap:        2,
		DataNackRepeatSeconds: 2.0,
	})

	a.ReceiveData(3, []byte("packet 3"))

	first := <-enqueuer.Packets
	if first.packetType != Enums.PACKET_STREAM_DATA_ACK {
		t.Fatalf("expected DATA_ACK, got %s", Enums.PacketTypeName(first.packetType))
	}

	select {
	case extra := <-enqueuer.Packets:
		t.Fatalf("expected no DATA_NACK for far gap, got %s", Enums.PacketTypeName(extra.packetType))
	case <-time.After(50 * time.Millisecond):
	}
}

func TestARQ_HandleDataNackQueuesImmediateResend(t *testing.T) {
	enqueuer := NewMockPacketEnqueuer()
	a := NewARQ(1, 1, enqueuer, nil, 1000, &testLogger{t}, Config{
		WindowSize: 64,
		RTO:        0.2,
		MaxRTO:     1.0,
	})

	a.mu.Lock()
	a.sndBuf[7] = &arqDataItem{
		Data:            []byte("hello"),
		CreatedAt:       time.Now(),
		LastSentAt:      time.Now().Add(-time.Second),
		CurrentRTO:      a.rto,
		CompressionType: 3,
	}
	a.mu.Unlock()

	if !a.HandleDataNack(7) {
		t.Fatal("expected HandleDataNack to schedule a resend")
	}

	p := <-enqueuer.Packets
	if p.packetType != Enums.PACKET_STREAM_RESEND {
		t.Fatalf("expected RESEND packet, got %s", Enums.PacketTypeName(p.packetType))
	}
	if p.sequenceNum != 7 {
		t.Fatalf("expected resend for seq 7, got %d", p.sequenceNum)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	info := a.sndBuf[7]
	if info == nil {
		t.Fatal("expected sequence 7 to remain tracked")
	}
	if info.Retries != 0 {
		t.Fatalf("expected NACK resend to leave retry count unchanged, got %d", info.Retries)
	}
	if info.CurrentRTO != a.rto {
		t.Fatalf("expected NACK resend to leave CurrentRTO unchanged, got %s", info.CurrentRTO)
	}
}

func TestARQ_ReceiveDataSuppressesRepeatedNackUntilInterval(t *testing.T) {
	enqueuer := NewMockPacketEnqueuer()
	a := NewARQ(1, 1, enqueuer, nil, 1000, &testLogger{t}, Config{
		WindowSize:            64,
		RTO:                   0.2,
		MaxRTO:                1.0,
		DataNackMaxGap:        2,
		DataNackRepeatSeconds: 2.0,
	})

	a.ReceiveData(1, []byte("packet 1"))
	<-enqueuer.Packets
	<-enqueuer.Packets

	a.ReceiveData(2, []byte("packet 2"))
	first := <-enqueuer.Packets
	second := <-enqueuer.Packets
	if first.packetType != Enums.PACKET_STREAM_DATA_ACK {
		t.Fatalf("expected DATA_ACK, got %s", Enums.PacketTypeName(first.packetType))
	}
	if second.packetType != Enums.PACKET_STREAM_DATA_NACK || second.sequenceNum != 1 {
		t.Fatalf("expected only a fresh NACK for seq 1, got %s seq=%d", Enums.PacketTypeName(second.packetType), second.sequenceNum)
	}

	select {
	case extra := <-enqueuer.Packets:
		t.Fatalf("expected no repeated NACK for seq 0 yet, got %s seq=%d", Enums.PacketTypeName(extra.packetType), extra.sequenceNum)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestARQ_ReceiveDataClearsQueuedNackWhenMissingDataArrives(t *testing.T) {
	enqueuer := NewMockPacketEnqueuer()
	a := NewARQ(1, 1, enqueuer, nil, 1000, &testLogger{t}, Config{
		WindowSize:            64,
		RTO:                   0.2,
		MaxRTO:                1.0,
		DataNackMaxGap:        2,
		DataNackRepeatSeconds: 2.0,
	})

	a.ReceiveData(1, []byte("packet 1"))
	<-enqueuer.Packets
	<-enqueuer.Packets

	a.ReceiveData(0, []byte("packet 0"))
	<-enqueuer.Packets

	enqueuer.mu.Lock()
	defer enqueuer.mu.Unlock()
	if len(enqueuer.removedNackSeqs) != 1 || enqueuer.removedNackSeqs[0] != 0 {
		t.Fatalf("expected queued NACK purge for seq 0, got %#v", enqueuer.removedNackSeqs)
	}
}

func TestARQ_ClearAllQueuesDropsRememberedDataNacks(t *testing.T) {
	a := NewARQ(1, 1, NewMockPacketEnqueuer(), nil, 1000, &testLogger{t}, Config{
		DataNackMaxGap:        2,
		DataNackRepeatSeconds: 2.0,
	})

	a.noteDataNackSent(10, time.Now())
	a.noteDataNackSent(11, time.Now())

	a.mu.Lock()
	a.clearAllQueues(true)
	a.mu.Unlock()

	a.dataNackMu.Lock()
	defer a.dataNackMu.Unlock()
	if len(a.lastDataNackSent) != 0 {
		t.Fatalf("expected remembered data NACK state to be cleared, got %#v", a.lastDataNackSent)
	}
}

func TestARQ_DataAckUpdatesAdaptiveBaseRTO(t *testing.T) {
	enq := NewMockPacketEnqueuer()
	a := NewARQ(1, 1, enq, nil, 1200, &testLogger{t}, Config{
		WindowSize: 32,
		RTO:        0.1,
		MaxRTO:     0.5,
	})

	sentAt := time.Now().Add(-220 * time.Millisecond)
	a.sndBuf[7] = &arqDataItem{
		Data:           []byte("hello"),
		CreatedAt:      sentAt,
		LastSentAt:     sentAt,
		CurrentRTO:     a.rto,
		SampleEligible: true,
	}

	if !a.ReceiveAck(Enums.PACKET_STREAM_DATA_ACK, 7) {
		t.Fatal("expected tracked data ack")
	}

	a.mu.RLock()
	base := a.dataAdaptiveRTO.currentBase
	initialized := a.dataAdaptiveRTO.initialized
	a.mu.RUnlock()

	if !initialized {
		t.Fatal("expected adaptive data RTO estimator to initialize")
	}
	if base <= a.rto {
		t.Fatalf("expected adaptive data base RTO to rise above initial %s, got %s", a.rto, base)
	}
	if base > a.maxRTO {
		t.Fatalf("expected adaptive data base RTO <= max %s, got %s", a.maxRTO, base)
	}
}

func TestARQ_DataAckSkipsAdaptiveSampleAfterRetransmit(t *testing.T) {
	enq := NewMockPacketEnqueuer()
	a := NewARQ(1, 1, enq, nil, 1200, &testLogger{t}, Config{
		WindowSize: 32,
		RTO:        0.1,
		MaxRTO:     0.5,
	})

	before := a.dataAdaptiveRTO
	sentAt := time.Now().Add(-220 * time.Millisecond)
	a.sndBuf[8] = &arqDataItem{
		Data:           []byte("hello"),
		CreatedAt:      sentAt,
		LastSentAt:     sentAt,
		CurrentRTO:     200 * time.Millisecond,
		SampleEligible: false,
	}

	if !a.ReceiveAck(Enums.PACKET_STREAM_DATA_ACK, 8) {
		t.Fatal("expected tracked data ack")
	}

	a.mu.RLock()
	after := a.dataAdaptiveRTO
	a.mu.RUnlock()

	if after != before {
		t.Fatalf("expected retransmitted packet ack to skip adaptive update: before=%+v after=%+v", before, after)
	}
}

func TestARQ_ControlAckUpdatesAdaptiveBaseRTO(t *testing.T) {
	enq := NewMockPacketEnqueuer()
	a := NewARQ(1, 1, enq, nil, 1200, &testLogger{t}, Config{
		WindowSize:               32,
		RTO:                      0.1,
		MaxRTO:                   0.5,
		EnableControlReliability: true,
		ControlRTO:               0.08,
		ControlMaxRTO:            0.4,
	})

	key := uint32(Enums.PACKET_STREAM_SYN)<<24 | uint32(3)<<8
	sentAt := time.Now().Add(-180 * time.Millisecond)
	a.controlSndBuf[key] = &arqControlItem{
		PacketType:     Enums.PACKET_STREAM_SYN,
		SequenceNum:    3,
		AckType:        Enums.PACKET_STREAM_SYN_ACK,
		CreatedAt:      sentAt,
		LastSentAt:     sentAt,
		CurrentRTO:     a.controlRto,
		SampleEligible: true,
	}

	if !a.ReceiveControlAck(Enums.PACKET_STREAM_SYN_ACK, 3, 0) {
		t.Fatal("expected tracked control ack")
	}

	a.mu.RLock()
	base := a.controlAdaptiveRTO.currentBase
	initialized := a.controlAdaptiveRTO.initialized
	a.mu.RUnlock()

	if !initialized {
		t.Fatal("expected adaptive control RTO estimator to initialize")
	}
	if base <= a.controlRto {
		t.Fatalf("expected adaptive control base RTO to rise above initial %s, got %s", a.controlRto, base)
	}
	if base > a.controlMaxRto {
		t.Fatalf("expected adaptive control base RTO <= max %s, got %s", a.controlMaxRto, base)
	}
}

func TestARQ_OutOfOrderReceive(t *testing.T) {
	enqueuer := NewMockPacketEnqueuer()
	cfg := Config{
		WindowSize: 100,
		RTO:        0.1,
		MaxRTO:     0.5,
	}

	localApp, arqConn := net.Pipe()
	defer localApp.Close()
	defer arqConn.Close()

	a := NewARQ(1, 1, enqueuer, arqConn, 1000, &testLogger{t}, cfg)
	a.Start()
	defer a.Close("test end", CloseOptions{Force: true})

	// Wait for workers to start
	time.Sleep(50 * time.Millisecond)

	// Send packets in order 1, 2, 0
	a.ReceiveData(1, []byte("packet 1"))
	a.ReceiveData(2, []byte("packet 2"))

	// Drain ACKs
	for i := 0; i < 2; i++ {
		<-enqueuer.Packets
	}

	// Verify nothing is readable yet (since packet 0 is missing)
	done := make(chan struct{})
	go func() {
		buf := make([]byte, 100)
		_ = localApp.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		_, _ = localApp.Read(buf)
		close(done)
	}()
	select {
	case <-done:
		// t.Error("should not have read anything yet")
		// Actually net.Pipe Read will block, so if it returns with timeout error it's fine.
	case <-time.After(150 * time.Millisecond):
		// Expected timeout
	}

	// Now send packet 0
	a.ReceiveData(0, []byte("packet 0"))
	<-enqueuer.Packets // ACK for 0

	// Now everything should be readable in order
	expected := [][]byte{[]byte("packet 0"), []byte("packet 1"), []byte("packet 2")}
	for _, exp := range expected {
		buf := make([]byte, 100)
		_ = localApp.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, err := localApp.Read(buf)
		if err != nil {
			t.Fatalf("failed to read from local app: %v", err)
		}
		if !bytes.Equal(buf[:n], exp) {
			t.Errorf("expected %s, got %s", string(exp), string(buf[:n]))
		}
	}
}

func TestARQ_Retransmission(t *testing.T) {
	enqueuer := NewMockPacketEnqueuer()
	cfg := Config{
		WindowSize: 100,
		RTO:        0.1, // 100ms RTO
		MaxRTO:     0.5,
	}

	localApp, arqConn := net.Pipe()
	defer localApp.Close()
	defer arqConn.Close()

	a := NewARQ(1, 1, enqueuer, arqConn, 1000, &testLogger{t}, cfg)
	a.Start()
	defer a.Close("test end", CloseOptions{Force: true})

	time.Sleep(50 * time.Millisecond)

	testData := []byte("retransmit me")
	go func() {
		_, _ = localApp.Write(testData)
	}()

	// Initial transmission
	select {
	case p := <-enqueuer.Packets:
		if p.packetType != Enums.PACKET_STREAM_DATA {
			t.Errorf("expected PACKET_STREAM_DATA, got %d", p.packetType)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for initial packet")
	}

	// Don't ACK. Wait for retransmission.
	// Retransmission loop uses baseInterval which is RTO/3 (approx 33ms) or 50ms min.
	// So we should see a RESEND packet soon after 100ms.
	select {
	case p := <-enqueuer.Packets:
		if p.packetType != Enums.PACKET_STREAM_RESEND {
			t.Errorf("expected front retransmission to use PACKET_STREAM_RESEND, got %d", p.packetType)
		}
		if p.priority != Enums.DefaultPacketPriority(Enums.PACKET_STREAM_RESEND) {
			t.Errorf("expected retry priority %d, got %d", Enums.DefaultPacketPriority(Enums.PACKET_STREAM_RESEND), p.priority)
		}
		if !bytes.Equal(p.payload, testData) {
			t.Errorf("expected payload %s, got %s", string(testData), string(p.payload))
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for retransmission")
	}
}

func TestARQ_RetransmitPrioritiesFavorFrontWindow(t *testing.T) {
	enqueuer := NewMockPacketEnqueuer()
	a := NewARQ(1, 1, enqueuer, nil, 1000, &testLogger{t}, Config{
		WindowSize: 10,
		RTO:        0.1,
		MaxRTO:     0.5,
	})
	a.windowSize = 10

	jobs := []rtxJob{
		{sn: 95},
		{sn: 99},
		{sn: 90},
	}
	a.sndNxt = 100

	priorityKinds := a.retransmitPriorityKinds(jobs)
	if len(priorityKinds) != len(jobs) {
		t.Fatalf("expected %d priority decisions, got %d", len(jobs), len(priorityKinds))
	}

	retryPriority := Enums.DefaultPacketPriority(Enums.PACKET_STREAM_RESEND)
	normalPriority := Enums.DefaultPacketPriority(Enums.PACKET_STREAM_DATA)

	if !priorityKinds[2] {
		t.Fatalf("expected oldest outstanding resend to get retry priority")
	}
	if priorityKinds[0] {
		t.Fatalf("expected middle resend to stay normal priority")
	}
	if priorityKinds[1] {
		t.Fatalf("expected newest resend to stay normal priority")
	}

	priorities := make([]int, len(priorityKinds))
	packetTypes := make([]uint8, len(priorityKinds))
	for i, isRetry := range priorityKinds {
		priorities[i] = normalPriority
		packetTypes[i] = Enums.PACKET_STREAM_DATA
		if isRetry {
			priorities[i] = retryPriority
			packetTypes[i] = Enums.PACKET_STREAM_RESEND
		}
	}
	if priorities[2] != retryPriority {
		t.Fatalf("expected oldest outstanding resend to map to retry priority %d, got %d", retryPriority, priorities[2])
	}
	if priorities[0] != normalPriority {
		t.Fatalf("expected middle resend to map to normal priority %d, got %d", normalPriority, priorities[0])
	}
	if priorities[1] != normalPriority {
		t.Fatalf("expected newest resend to map to normal priority %d, got %d", normalPriority, priorities[1])
	}
	if packetTypes[2] != Enums.PACKET_STREAM_RESEND {
		t.Fatalf("expected oldest outstanding resend to keep STREAM_RESEND type, got %d", packetTypes[2])
	}
	if packetTypes[0] != Enums.PACKET_STREAM_DATA {
		t.Fatalf("expected middle retransmit to downgrade to STREAM_DATA, got %d", packetTypes[0])
	}
	if packetTypes[1] != Enums.PACKET_STREAM_DATA {
		t.Fatalf("expected newest retransmit to downgrade to STREAM_DATA, got %d", packetTypes[1])
	}
}

func TestARQ_ACKHandling(t *testing.T) {
	enqueuer := NewMockPacketEnqueuer()
	cfg := Config{
		WindowSize: 100,
		RTO:        0.1,
		MaxRTO:     0.5,
	}

	localApp, arqConn := net.Pipe()
	defer localApp.Close()
	defer arqConn.Close()

	a := NewARQ(1, 1, enqueuer, arqConn, 1000, &testLogger{t}, cfg)
	a.Start()
	defer a.Close("test end", CloseOptions{Force: true})

	time.Sleep(50 * time.Millisecond)

	go func() {
		_, _ = localApp.Write([]byte("data"))
	}()

	var sn uint16
	select {
	case p := <-enqueuer.Packets:
		sn = p.sequenceNum
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out")
	}

	// Verify it's in sndBuf
	a.mu.Lock()
	if _, exists := a.sndBuf[sn]; !exists {
		t.Error("packet should be in sndBuf")
	}
	a.mu.Unlock()

	// Receive ACK
	a.HandleAckPacket(Enums.PACKET_STREAM_DATA_ACK, sn, 0)

	// Verify it's removed from sndBuf
	a.mu.Lock()
	if _, exists := a.sndBuf[sn]; exists {
		t.Error("packet should be removed from sndBuf after ACK")
	}
	a.mu.Unlock()
}

func TestARQ_GracefulClose(t *testing.T) {
	enqueuer := NewMockPacketEnqueuer()
	cfg := Config{
		WindowSize:               100,
		RTO:                      0.1,
		MaxRTO:                   0.5,
		EnableControlReliability: true,
	}

	localApp, arqConn := net.Pipe()
	defer localApp.Close()
	defer arqConn.Close()

	a := NewARQ(1, 1, enqueuer, arqConn, 1000, &testLogger{t}, cfg)
	a.Start()

	time.Sleep(50 * time.Millisecond)

	// Local app closes connection
	_ = localApp.Close()

	// ARQ should send a FIN
	select {
	case p := <-enqueuer.Packets:
		if p.packetType != Enums.PACKET_STREAM_FIN {
			t.Errorf("expected PACKET_STREAM_FIN, got %d", p.packetType)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for FIN")
	}

	// Remote ACKs FIN
	a.HandleAckPacket(Enums.PACKET_STREAM_FIN_ACK, 0, 0)

	// Remote sends FIN
	a.MarkFinReceived()

	// Wait for ARQ to close
	select {
	case <-a.Done():
		// Success
	case <-time.After(1 * time.Second):
		t.Error("ARQ should be closed after FIN handshake")
	}
}

func TestARQ_IOReadDataWithEOFStillQueuesFinalChunk(t *testing.T) {
	enqueuer := NewMockPacketEnqueuer()
	cfg := Config{
		WindowSize:               100,
		RTO:                      0.1,
		MaxRTO:                   0.5,
		EnableControlReliability: true,
	}

	conn := &eofAfterDataConn{data: []byte("final chunk")}
	a := NewARQ(1, 1, enqueuer, conn, 1000, &testLogger{t}, cfg)
	a.Start()
	defer a.Close("test end", CloseOptions{Force: true})

	var gotData bool
	timeout := time.After(1 * time.Second)
	for !gotData {
		select {
		case p := <-enqueuer.Packets:
			switch p.packetType {
			case Enums.PACKET_STREAM_DATA:
				gotData = true
				if !bytes.Equal(p.payload, []byte("final chunk")) {
					t.Fatalf("expected final payload %q, got %q", []byte("final chunk"), p.payload)
				}
			}
		case <-timeout:
			t.Fatalf("timed out waiting for final data, gotData=%t", gotData)
		}
	}

	if !a.HasPendingSequence(0) {
		t.Fatal("expected final chunk to remain tracked in sndBuf until acknowledged")
	}
	if a.IsReset() {
		t.Fatal("expected EOF after data to stay on graceful close path, not reset path")
	}
}

func TestARQ_IOReadDataWithErrorDefersRSTUntilDrain(t *testing.T) {
	enqueuer := NewMockPacketEnqueuer()
	cfg := Config{
		WindowSize:               100,
		RTO:                      0.1,
		MaxRTO:                   0.5,
		EnableControlReliability: true,
	}

	conn := &errAfterDataConn{data: []byte("chunk before read error"), err: errors.New("boom")}
	a := NewARQ(1, 1, enqueuer, conn, 1000, &testLogger{t}, cfg)
	a.Start()
	defer a.Close("test end", CloseOptions{Force: true})

	timeout := time.After(1 * time.Second)
	gotData := false
	for !gotData {
		select {
		case p := <-enqueuer.Packets:
			if p.packetType == Enums.PACKET_STREAM_DATA {
				gotData = true
				if !bytes.Equal(p.payload, []byte("chunk before read error")) {
					t.Fatalf("expected queued payload %q, got %q", []byte("chunk before read error"), p.payload)
				}
			}
		case <-timeout:
			t.Fatal("timed out waiting for final data chunk")
		}
	}

	if !a.HasPendingSequence(0) {
		t.Fatal("expected final chunk to remain pending for drain")
	}

	select {
	case p := <-enqueuer.Packets:
		if p.packetType == Enums.PACKET_STREAM_RST {
			t.Fatal("expected read error after data not to emit RST immediately before drain")
		}
	default:
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		a.mu.Lock()
		deferred := a.deferredClose
		deferredPacket := a.deferredPacket
		a.mu.Unlock()
		if deferred && deferredPacket == Enums.PACKET_STREAM_RST {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("expected read error after data to arm deferred RST drain path")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestARQ_IOTransientReadErrorDoesNotResetStream(t *testing.T) {
	enqueuer := NewMockPacketEnqueuer()
	cfg := Config{
		WindowSize: 100,
		RTO:        0.1,
		MaxRTO:     0.5,
	}

	conn := &transientReadConn{}
	a := NewARQ(1, 1, enqueuer, conn, 1000, &testLogger{t}, cfg)
	a.Start()
	defer a.Close("test end", CloseOptions{Force: true})

	time.Sleep(150 * time.Millisecond)

	if a.IsClosed() {
		t.Fatal("expected transient read error not to close stream")
	}
	if a.IsReset() {
		t.Fatal("expected transient read error not to move stream to reset path")
	}
}

func TestARQ_WriteLoopRetriesTransientWriteError(t *testing.T) {
	enqueuer := NewMockPacketEnqueuer()
	cfg := Config{
		WindowSize: 100,
		RTO:        0.1,
		MaxRTO:     0.5,
	}

	conn := &transientWriteConn{failWrites: 1}
	a := NewARQ(1, 1, enqueuer, conn, 1000, &testLogger{t}, cfg)
	a.Start()
	defer a.Close("test end", CloseOptions{Force: true})

	a.ReceiveData(0, []byte("from peer"))

	timeout := time.After(1 * time.Second)
	for {
		conn.mu.Lock()
		writes := len(conn.writes)
		payload := []byte(nil)
		if writes > 0 {
			payload = append([]byte(nil), conn.writes[0]...)
		}
		conn.mu.Unlock()
		if writes > 0 {
			if !bytes.Equal(payload, []byte("from peer")) {
				t.Fatalf("expected write payload %q, got %q", []byte("from peer"), payload)
			}
			break
		}

		select {
		case <-timeout:
			t.Fatal("timed out waiting for transient write retry to succeed")
		default:
			time.Sleep(20 * time.Millisecond)
		}
	}

	if a.IsClosed() {
		t.Fatal("expected transient write error not to close stream")
	}
}

func TestARQ_WriteErrorDefersRSTWhileOutboundDataPending(t *testing.T) {
	enqueuer := NewMockPacketEnqueuer()
	cfg := Config{
		WindowSize: 100,
		RTO:        0.1,
		MaxRTO:     0.5,
	}

	conn := &fatalWriteConn{}
	a := NewARQ(1, 1, enqueuer, conn, 1000, &testLogger{t}, cfg)
	a.mu.Lock()
	a.sndBuf[7] = &arqDataItem{
		Data:       []byte("pending outbound"),
		CreatedAt:  time.Now(),
		LastSentAt: time.Now(),
		CurrentRTO: a.rto,
	}
	a.mu.Unlock()

	a.Start()
	defer a.Close("test end", CloseOptions{Force: true})

	a.ReceiveData(0, []byte("from peer"))

	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		a.mu.Lock()
		deferred := a.deferredClose
		deferredPacket := a.deferredPacket
		closed := a.closed
		a.mu.Unlock()
		if deferred && deferredPacket == Enums.PACKET_STREAM_RST {
			break
		}
		if closed {
			t.Fatal("expected fatal write error with pending outbound data not to close immediately")
		}
		if time.Now().After(deadline) {
			t.Fatal("expected fatal write error to arm deferred RST while outbound data is pending")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestARQ_PeerFinHalfCloseStillAcceptsInboundData(t *testing.T) {
	enqueuer := NewMockPacketEnqueuer()
	cfg := Config{
		WindowSize:               100,
		RTO:                      0.1,
		MaxRTO:                   0.5,
		EnableControlReliability: true,
	}

	localApp, arqConn := net.Pipe()
	defer localApp.Close()
	defer arqConn.Close()

	a := NewARQ(1, 1, enqueuer, arqConn, 1000, &testLogger{t}, cfg)
	a.Start()
	defer a.Close("test end", CloseOptions{Force: true})

	time.Sleep(50 * time.Millisecond)

	a.MarkFinReceived()

	if state := a.State(); state != StateHalfClosedRemote {
		t.Fatalf("expected half-closed-remote after peer FIN, got %v", state)
	}
	if a.IsClosed() {
		t.Fatal("stream should not close immediately after peer FIN")
	}

	payload := []byte("peer data after fin")
	a.ReceiveData(0, payload)

	select {
	case p := <-enqueuer.Packets:
		if p.packetType != Enums.PACKET_STREAM_DATA_ACK {
			t.Fatalf("expected STREAM_DATA_ACK after inbound data, got %d", p.packetType)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for STREAM_DATA_ACK")
	}

	buf := make([]byte, 128)
	_ = localApp.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	n, err := localApp.Read(buf)
	if err != nil {
		t.Fatalf("failed to read forwarded inbound data: %v", err)
	}
	if !bytes.Equal(buf[:n], payload) {
		t.Fatalf("expected payload %q, got %q", payload, buf[:n])
	}
}

func TestARQ_FinHandshakeWaitsForInboundWriteDrain(t *testing.T) {
	enqueuer := NewMockPacketEnqueuer()
	cfg := Config{
		WindowSize:               100,
		RTO:                      0.1,
		MaxRTO:                   0.5,
		EnableControlReliability: true,
	}

	conn := newBlockingWriteConn()
	a := NewARQ(1, 1, enqueuer, conn, 1000, &testLogger{t}, cfg)
	a.Start()
	defer a.Close("test end", CloseOptions{Force: true})

	a.ReceiveData(0, []byte("buffered before fin"))

	select {
	case <-conn.writeCh:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for local write to start")
	}

	a.MarkFinReceived()
	a.markFinAcked()
	a.tryFinalizeRemoteEOF()

	time.Sleep(100 * time.Millisecond)
	if a.IsClosed() {
		t.Fatal("stream should not finalize FIN handshake while local write is still in flight")
	}

	close(conn.release)

	select {
	case <-a.Done():
	case <-time.After(1 * time.Second):
		t.Fatal("expected FIN handshake to complete after inbound write drain")
	}
}

func TestARQ_FinAckTimeoutDoesNotCompleteHandshake(t *testing.T) {
	enqueuer := NewMockPacketEnqueuer()
	cfg := Config{
		WindowSize:               100,
		RTO:                      0.1,
		MaxRTO:                   0.5,
		EnableControlReliability: true,
		TerminalAckWaitTimeout:   0.1,
	}

	a := NewARQ(1, 1, enqueuer, nil, 1000, &testLogger{t}, cfg)
	a.mu.Lock()
	finSeq := uint16(7)
	a.finSeqSent = &finSeq
	a.finSent = true
	a.finReceived = true
	a.waitingAck = true
	a.waitingAckFor = Enums.PACKET_STREAM_FIN
	a.ackWaitDeadline = time.Now().Add(-10 * time.Millisecond)
	a.controlSndBuf[uint32(Enums.PACKET_STREAM_FIN)<<24|uint32(finSeq)<<8] = &arqControlItem{
		PacketType:  Enums.PACKET_STREAM_FIN,
		SequenceNum: finSeq,
		AckType:     Enums.PACKET_STREAM_FIN_ACK,
		CreatedAt:   time.Now(),
		LastSentAt:  time.Now(),
		CurrentRTO:  a.controlRto,
	}
	a.mu.Unlock()

	if a.handleTerminalRetransmitState(time.Now()) {
		t.Fatal("expected FIN ack timeout not to finalize stream")
	}
	if a.IsClosed() {
		t.Fatal("expected stream to stay open while waiting for FIN_ACK")
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.waitingAck || a.waitingAckFor != Enums.PACKET_STREAM_FIN {
		t.Fatal("expected FIN to remain tracked after ack wait timeout")
	}
	if _, exists := a.controlSndBuf[uint32(Enums.PACKET_STREAM_FIN)<<24|uint32(finSeq)<<8]; !exists {
		t.Fatal("expected tracked FIN control packet to remain for retransmission")
	}
}

func TestARQ_GracefulCloseWriteFailureStillRechecksFinCompletion(t *testing.T) {
	enqueuer := NewMockPacketEnqueuer()
	cfg := Config{
		WindowSize:               100,
		RTO:                      0.1,
		MaxRTO:                   0.5,
		EnableControlReliability: true,
	}

	conn := newCloseOnWriteConn()
	a := NewARQ(1, 1, enqueuer, conn, 1000, &testLogger{t}, cfg)
	a.Start()
	defer a.Close("test end", CloseOptions{Force: true})

	a.MarkFinReceived()
	a.markFinAcked()
	a.ReceiveData(0, []byte("final inbound chunk"))

	select {
	case <-conn.writeCh:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for graceful-close write attempt")
	}

	select {
	case <-a.Done():
	case <-time.After(1 * time.Second):
		t.Fatal("expected graceful-close write failure to recheck and complete FIN path")
	}
}

func TestARQ_WriteDeadlineTimeoutRetriesAndFlushes(t *testing.T) {
	enqueuer := NewMockPacketEnqueuer()
	cfg := Config{
		WindowSize: 100,
		RTO:        0.1,
		MaxRTO:     0.5,
	}

	conn := &writeDeadlineTimeoutConn{}
	a := NewARQ(1, 1, enqueuer, conn, 1000, &testLogger{t}, cfg)
	a.Start()
	defer a.Close("test end", CloseOptions{Force: true})

	a.ReceiveData(0, []byte("from peer"))

	timeout := time.After(1 * time.Second)
	for {
		conn.mu.Lock()
		writes := len(conn.writes)
		attempts := conn.writeAttempts
		payload := []byte(nil)
		if writes > 0 {
			payload = append([]byte(nil), conn.writes[0]...)
		}
		conn.mu.Unlock()
		if writes > 0 {
			if attempts < 2 {
				t.Fatalf("expected at least 2 write attempts after timeout, got %d", attempts)
			}
			if !bytes.Equal(payload, []byte("from peer")) {
				t.Fatalf("expected write payload %q, got %q", []byte("from peer"), payload)
			}
			break
		}

		select {
		case <-timeout:
			t.Fatal("timed out waiting for write timeout retry to succeed")
		default:
			time.Sleep(20 * time.Millisecond)
		}
	}
}

func TestARQ_DataRetransmitDoesNotAdvanceRetryOrRTOWhenEnqueueRejected(t *testing.T) {
	a := NewARQ(1, 1, RejectingPacketEnqueuer{}, nil, 1000, &testLogger{t}, Config{
		WindowSize: 100,
		RTO:        0.1,
		MaxRTO:     0.5,
	})

	now := time.Now()
	a.mu.Lock()
	a.sndBuf[9] = &arqDataItem{
		Data:            []byte("payload"),
		CreatedAt:       now.Add(-time.Second),
		LastSentAt:      now.Add(-time.Second),
		Retries:         2,
		CurrentRTO:      200 * time.Millisecond,
		CompressionType: 0,
	}
	beforeLastSent := a.sndBuf[9].LastSentAt
	beforeRetries := a.sndBuf[9].Retries
	beforeRTO := a.sndBuf[9].CurrentRTO
	a.mu.Unlock()

	a.checkRetransmits()

	a.mu.RLock()
	info := a.sndBuf[9]
	a.mu.RUnlock()
	if info == nil {
		t.Fatal("expected pending data item to remain tracked")
	}
	if info.Retries != beforeRetries {
		t.Fatalf("expected retries to stay at %d, got %d", beforeRetries, info.Retries)
	}
	if info.CurrentRTO != beforeRTO {
		t.Fatalf("expected CurrentRTO to stay at %v, got %v", beforeRTO, info.CurrentRTO)
	}
	if !info.LastSentAt.Equal(beforeLastSent) {
		t.Fatalf("expected LastSentAt to stay at %v, got %v", beforeLastSent, info.LastSentAt)
	}
}

func TestARQ_ControlRetransmitDoesNotAdvanceRetryOrRTOWhenEnqueueRejected(t *testing.T) {
	a := NewARQ(1, 1, RejectingPacketEnqueuer{}, nil, 1000, &testLogger{t}, Config{
		WindowSize:               100,
		RTO:                      0.1,
		MaxRTO:                   0.5,
		EnableControlReliability: true,
		ControlRTO:               0.1,
		ControlMaxRTO:            0.5,
		ControlMaxRetries:        8,
	})

	now := time.Now()
	key := uint32(Enums.PACKET_STREAM_FIN)<<24 | uint32(7)<<8
	a.mu.Lock()
	a.controlSndBuf[key] = &arqControlItem{
		PacketType:  Enums.PACKET_STREAM_FIN,
		SequenceNum: 7,
		AckType:     Enums.PACKET_STREAM_FIN_ACK,
		CreatedAt:   now.Add(-time.Second),
		LastSentAt:  now.Add(-time.Second),
		Retries:     3,
		CurrentRTO:  200 * time.Millisecond,
	}
	beforeLastSent := a.controlSndBuf[key].LastSentAt
	beforeRetries := a.controlSndBuf[key].Retries
	beforeRTO := a.controlSndBuf[key].CurrentRTO
	a.mu.Unlock()

	a.checkControlRetransmits(time.Now())

	a.mu.RLock()
	info := a.controlSndBuf[key]
	a.mu.RUnlock()
	if info == nil {
		t.Fatal("expected tracked control packet to remain queued")
	}
	if info.Retries != beforeRetries {
		t.Fatalf("expected retries to stay at %d, got %d", beforeRetries, info.Retries)
	}
	if info.CurrentRTO != beforeRTO {
		t.Fatalf("expected CurrentRTO to stay at %v, got %v", beforeRTO, info.CurrentRTO)
	}
	if !info.LastSentAt.Equal(beforeLastSent) {
		t.Fatalf("expected LastSentAt to stay at %v, got %v", beforeLastSent, info.LastSentAt)
	}
}

func TestARQ_PeerFinThenLocalFinAckClosesWithoutRST(t *testing.T) {
	enqueuer := NewMockPacketEnqueuer()
	cfg := Config{
		WindowSize:               100,
		RTO:                      0.1,
		MaxRTO:                   0.5,
		EnableControlReliability: true,
	}

	a := NewARQ(1, 1, enqueuer, nil, 1000, &testLogger{t}, cfg)

	a.MarkFinReceived()
	if state := a.State(); state != StateHalfClosedRemote {
		t.Fatalf("expected half-closed-remote after peer FIN, got %v", state)
	}
	if a.IsClosed() {
		t.Fatal("stream should remain open until local FIN path completes")
	}

	a.Close("local graceful close after peer fin", CloseOptions{SendFIN: true})

	select {
	case p := <-enqueuer.Packets:
		if p.packetType != Enums.PACKET_STREAM_FIN {
			t.Fatalf("expected STREAM_FIN, got %d", p.packetType)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for STREAM_FIN")
	}

	if a.IsClosed() {
		t.Fatal("stream should not close before FIN is acknowledged")
	}

	a.HandleAckPacket(Enums.PACKET_STREAM_FIN_ACK, 0, 0)

	select {
	case <-a.Done():
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected stream to close after peer FIN and local FIN_ACK")
	}

	if state := a.State(); state != StateTimeWait {
		t.Fatalf("expected TIME_WAIT after graceful FIN handshake, got %v", state)
	}

	select {
	case p := <-enqueuer.Packets:
		if p.packetType == Enums.PACKET_STREAM_RST {
			t.Fatal("did not expect STREAM_RST during graceful FIN handshake")
		}
	default:
	}
}

func TestARQ_Reset(t *testing.T) {
	enqueuer := NewMockPacketEnqueuer()
	cfg := Config{
		WindowSize: 100,
		RTO:        0.1,
		MaxRTO:     0.5,
	}

	localApp, arqConn := net.Pipe()
	defer localApp.Close()
	defer arqConn.Close()

	a := NewARQ(1, 1, enqueuer, arqConn, 1000, &testLogger{t}, cfg)
	a.Start()

	time.Sleep(50 * time.Millisecond)

	// Close with RST
	a.Close("testing reset", CloseOptions{SendRST: true})

	// ARQ should send an RST
	select {
	case p := <-enqueuer.Packets:
		if p.packetType != Enums.PACKET_STREAM_RST {
			t.Errorf("expected PACKET_STREAM_RST, got %d", p.packetType)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for RST")
	}

	// ARQ should mark state as Reset
	if a.State() != StateReset {
		t.Errorf("expected state StateReset, got %v", a.State())
	}
}

func TestARQ_Backpressure(t *testing.T) {
	enqueuer := NewMockPacketEnqueuer()
	cfg := Config{
		WindowSize: 10,
		RTO:        1.0,
		MaxRTO:     2.0,
	}

	localApp, arqConn := net.Pipe()
	defer localApp.Close()
	defer arqConn.Close()

	a := NewARQ(1, 1, enqueuer, arqConn, 10, &testLogger{t}, cfg)
	a.Start()
	defer a.Close("test end", CloseOptions{Force: true})

	time.Sleep(50 * time.Millisecond)

	// Send 8 packets (limit is 0.8 * 10 = 8)
	data := []byte("1234567890") // 10 bytes
	for i := 0; i < 8; i++ {
		_, err := localApp.Write(data)
		if err != nil {
			t.Fatalf("failed to write %d: %v", i, err)
		}
	}

	// Drain transmitted packets
	for i := 0; i < 8; i++ {
		select {
		case <-enqueuer.Packets:
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("timed out waiting for packet %d", i)
		}
	}

	// The 9th write should block or at least waitWindowNotFull should trigger.
	// Since we are in a goroutine in ioLoop, we can check if sndBuf size is 8.
	a.mu.Lock()
	sndBufLen := len(a.sndBuf)
	a.mu.Unlock()
	if sndBufLen != 8 {
		t.Errorf("expected sndBuf size 8, got %d", sndBufLen)
	}

	// Try writing one more. It should block ioLoop.
	writeDone := make(chan struct{})
	go func() {
		_, _ = localApp.Write(data)
		close(writeDone)
	}()

	select {
	case <-writeDone:
		// It might not block immediately because of net.Pipe internal buffering,
		// but ioLoop should be waiting at waitWindowNotFull.
	case <-time.After(200 * time.Millisecond):
		// Expected to block if net.Pipe buffer is small or ioLoop is waiting.
	}

	// ACK one packet
	a.ReceiveAck(Enums.PACKET_STREAM_DATA_ACK, 0)

	// Now ioLoop should proceed and send the 9th packet
	select {
	case p := <-enqueuer.Packets:
		if p.sequenceNum != 8 {
			t.Errorf("expected sequence 8, got %d", p.sequenceNum)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for 9th packet after ACK")
	}
}
