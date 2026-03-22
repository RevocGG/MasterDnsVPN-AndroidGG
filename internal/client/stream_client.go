// ==============================================================================
// MasterDnsVPN
// Author: MasterkinG32
// Github: https://github.com/masterking32
// Year: 2026
// ==============================================================================

package client

import (
	"net"
	"sync" // Added for sync.Pool
	"time"

	"masterdnsvpn-go/internal/arq"
	Enums "masterdnsvpn-go/internal/enums"
	"masterdnsvpn-go/internal/mlq"
)

var txPacketPool = sync.Pool{
	New: func() any {
		return &clientStreamTXPacket{}
	},
}

// Stream_client represents a single stream's data structure, mirroring the Python version's
// 'active_streams' dictionary elements.
type Stream_client struct {
	client *Client

	StreamID           uint16
	NetConn            net.Conn
	CreateTime         time.Time
	LastActivityTime   time.Time
	Status             string // PENDING, ACTIVE, CLOSED
	Stream             any    // The ARQ object
	StreamCreating     bool
	PendingInboundData map[uint16][]byte

	// High-performance multi-level priority queue
	txQueue *mlq.MultiLevelQueue[*clientStreamTXPacket]

	InitialPayload []byte
	PriorityCounts map[int]int

	// Metadata & Failover
	PreferredServerKey     string
	ResolverResendStreak   int
	LastResolverFailoverAt time.Time
	HandshakeLastProgress  time.Time
}

// getTrackingKey generates a unified key for duplicate prevention.
// It ensures that certain types (like DATA and RESEND) share the same tracking slot.
func getTrackingKey(packetType uint8, sequenceNum uint16, fragmentID uint8) uint32 {
	t := packetType
	// Map related types to a single ID to prevent duplicates across types
	if t == Enums.PACKET_STREAM_RESEND {
		t = Enums.PACKET_STREAM_DATA
	}
	// Key: [8bit PacketType][16bit SequenceNum][8bit FragmentID]
	return uint32(t)<<24 | uint32(sequenceNum)<<8 | uint32(fragmentID)
}

// get_new_stream_id finds the next available stream ID using a circular counter (1-65535).
func (c *Client) get_new_stream_id() (uint16, bool) {
	c.streamsMu.Lock()
	defer c.streamsMu.Unlock()

	start := c.last_stream_id + 1
	if start == 0 {
		start = 1
	}

	id := start
	wrapped := false

	// Cycle through IDs to find an available one in active_streams
	for {
		if _, exists := c.active_streams[id]; !exists {
			c.last_stream_id = id
			return id, true
		}

		id++
		if id == 0 {
			if wrapped {
				return 0, false // Fully occupied (unlikely but safe)
			}
			id = 1
			wrapped = true
		}

		if wrapped && id == start {
			return 0, false // Entire cycle checked, no free ID
		}
	}
}

// new_stream initializes a new Stream_client with default values.
func (c *Client) new_stream(streamID uint16, conn net.Conn, targetPayload []byte) *Stream_client {
	now := time.Now()

	s := &Stream_client{
		client:             c,
		StreamID:           streamID,
		NetConn:            conn,
		CreateTime:         now,
		LastActivityTime:   now,
		Status:             "PENDING",
		StreamCreating:     false,
		PendingInboundData: make(map[uint16][]byte),
		InitialPayload:     targetPayload,
		PriorityCounts:     make(map[int]int),

		txQueue: mlq.New[*clientStreamTXPacket](64),

		HandshakeLastProgress: now,
	}

	// Initialize and start the highly-optimized ARQ engine (Ported from Python)
	mtu := c.syncedUploadMTU
	if mtu <= 0 {
		mtu = 1200 // Safe default
	}

	arqCfg := arq.Config{
		WindowSize:               c.cfg.ARQWindowSize,
		RTO:                      0.2, // Fast retry out of the gate
		MaxRTO:                   1.5,
		IsSocks:                  c.cfg.ProtocolType == "SOCKS5",
		InitialData:              targetPayload,
		EnableControlReliability: true,
		ControlRTO:               0.8,
		ControlMaxRTO:            2.5,
		ControlMaxRetries:        40,
		InactivityTimeout:        1200.0,
		DataPacketTTL:            600.0,
		MaxDataRetries:           400,
		ControlPacketTTL:         600.0,
		FinDrainTimeout:          300.0,
		GracefulDrainTimeout:     600.0,
	}

	a := arq.NewARQ(streamID, c.sessionID, s, conn, mtu, c.log, arqCfg)
	s.Stream = a
	a.Start()

	c.streamsMu.Lock()
	if c.active_streams == nil {
		c.active_streams = make(map[uint16]*Stream_client)
	}
	c.active_streams[streamID] = s
	c.streamsMu.Unlock()

	return s
}

// PushTXPacket adds a packet to the appropriate priority queue if it's not a duplicate.
func (s *Stream_client) PushTXPacket(priority int, packetType uint8, sequenceNum uint16, fragmentID uint8, totalFragments uint8, payload []byte) bool {
	// Generate the tracking key (Policy)
	key := getTrackingKey(packetType, sequenceNum, fragmentID)

	// Delegate to MLQ (Mechanism)
	if priority < 0 || priority >= 6 {
		priority = 3 // Default
	}

	// Skip Ping packets if the queue is already congested (prevent bloat)
	if packetType == Enums.PACKET_PING && s.txQueue != nil && s.txQueue.Size() > 200 {
		return false
	}

	// Get a packet from the pool
	p := txPacketPool.Get().(*clientStreamTXPacket)
	p.PacketType = packetType
	p.SequenceNum = sequenceNum
	p.FragmentID = fragmentID
	p.TotalFragments = totalFragments
	p.Payload = payload
	p.CreatedAt = time.Now()
	p.RetryCount = 0
	p.Scheduled = false

	if ok := s.txQueue.Push(priority, key, p); !ok {
		// Duplicate found in census
		s.ReleaseTXPacket(p)
		return false
	}

	select {
	case s.client.txSignal <- struct{}{}:
	default:
	}

	return true
}

// PopNextTXPacket retrieves the highest priority packet from the queues.
func (s *Stream_client) PopNextTXPacket() (*clientStreamTXPacket, int, bool) {
	// Delegate to MLQ
	packet, priority, ok := s.txQueue.Pop(func(p *clientStreamTXPacket) uint32 {
		return getTrackingKey(p.PacketType, p.SequenceNum, p.FragmentID)
	})

	return packet, priority, ok
}

// GetQueuedPacket checks if a packet exists in any priority queue in O(1).
func (s *Stream_client) GetQueuedPacket(packetType uint8, sequenceNum uint16, fragmentID uint8) (*clientStreamTXPacket, bool) {
	key := getTrackingKey(packetType, sequenceNum, fragmentID)
	return s.txQueue.Get(key)
}

// Close gracefully shuts down the stream and releases all resources.
func (s *Stream_client) Close() {
	// 1. Close the ARQ object if it exists
	if s.Stream != nil {
		if a, ok := s.Stream.(*arq.ARQ); ok {
			a.Close("Stream_client.Close called", false)
		}
	}

	// 2. Close the network connection
	if s.NetConn != nil {
		_ = s.NetConn.Close()
	}

	// 3. Clear the TX queue and return all packets to the pool (Safety)
	if s.txQueue != nil {
		s.txQueue.Clear(func(p *clientStreamTXPacket) {
			s.ReleaseTXPacket(p)
		})
	}

	// 4. Clear inbound buffer
	s.PendingInboundData = nil
	s.Status = "CLOSED"
}

// ReleaseTXPacket returns a packet to the pool.
func (s *Stream_client) ReleaseTXPacket(p *clientStreamTXPacket) {
	if p == nil {
		return
	}
	p.Payload = nil // Clear payload reference
	txPacketPool.Put(p)
}

// -----------------------------------------------------------------------------------------
// Virtual Stream 0 Support
// -----------------------------------------------------------------------------------------

type fakeConn struct{}

func (f *fakeConn) Read(b []byte) (n int, err error) {
	// Block eternally so ARQ's ioLoop doesn't spin or immediately exit
	select {}
}

func (f *fakeConn) Write(b []byte) (n int, err error) {
	// Swallow data silently for Stream 0 local-writes
	return len(b), nil
}

func (f *fakeConn) Close() error {
	return nil
}

// InitVirtualStream0 initializes Stream #0 instantly upon Session start.
// This serves as the control and Ping channel running perpetually without timeout.
func (c *Client) InitVirtualStream0() {
	c.streamsMu.Lock()
	defer c.streamsMu.Unlock()

	streamID := uint16(0)
	s := &Stream_client{
		client:     c,
		StreamID:   streamID,
		txQueue:    mlq.New[*clientStreamTXPacket](64),
		CreateTime: time.Now(),
	}

	mtu := c.syncedUploadMTU
	if mtu <= 0 {
		mtu = 1200
	}

	arqCfg := arq.Config{
		WindowSize:               c.cfg.ARQWindowSize,
		RTO:                      0.2, // Fast retry out of the gate
		MaxRTO:                   1.5,
		IsSocks:                  false,
		IsVirtual:                true, // Bypasses internal timeout closures
		EnableControlReliability: true,
		ControlRTO:               0.8,
		ControlMaxRTO:            2.5,
		ControlMaxRetries:        40,
		InactivityTimeout:        999999.0, // Infinite
		DataPacketTTL:            999999.0,
		MaxDataRetries:           99999,
		ControlPacketTTL:         999999.0,
		FinDrainTimeout:          300.0,
		GracefulDrainTimeout:     600.0,
	}

	conn := &fakeConn{}
	a := arq.NewARQ(streamID, c.sessionID, s, conn, mtu, c.log, arqCfg)
	s.Stream = a
	c.active_streams[streamID] = s
	a.Start()

	c.log.Infof("🚀 <green>Virtual Stream 0 (Control Channel) Initialized.</green>")
}

// CloseAllStreams completely flushes all ARQ bindings. For Stream 0, it calls ForceClose.
func (c *Client) CloseAllStreams() {
	c.streamsMu.Lock()
	streams := make([]*Stream_client, 0, len(c.active_streams))
	for _, s := range c.active_streams {
		streams = append(streams, s)
	}
	c.active_streams = make(map[uint16]*Stream_client)
	c.streamsMu.Unlock()

	for _, s := range streams {
		if a, ok := s.Stream.(*arq.ARQ); ok {
			if s.StreamID == 0 {
				a.ForceClose("Session Reset (Virtual Stream 0 Force Destroy)")
			} else {
				a.Close("Session Reset (All Streams Destroy)", false)
			}
		}
	}
}
