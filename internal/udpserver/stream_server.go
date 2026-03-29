// ==============================================================================
// MasterDnsVPN
// Author: MasterkinG32
// Github: https://github.com/masterking32
// Year: 2026
// ==============================================================================

package udpserver

import (
	"io"
	"sync"
	"sync/atomic"
	"time"

	"masterdnsvpn-go/internal/arq"
	Enums "masterdnsvpn-go/internal/enums"
	"masterdnsvpn-go/internal/mlq"
)

// Stream_server encapsulates an ARQ instance and its transmit queue for a single stream.
type Stream_server struct {
	mu        sync.RWMutex
	txQueueMu sync.Mutex
	cleanupMu sync.Once

	ID        uint16
	SessionID uint8
	ARQ       *arq.ARQ
	TXQueue   *mlq.MultiLevelQueue[*serverStreamTXPacket]

	Status       string
	CreatedAt    time.Time
	LastActivity time.Time
	CloseTime    time.Time

	UpstreamConn io.ReadWriteCloser
	TargetHost   string
	TargetPort   uint16
	Connected    bool
	onClosed     func(uint16, time.Time, string)
	onQueueStateChanged func(uint16, bool)
	txHasItems         atomic.Uint32

	// Tracking for deduplication (similar to Python's _track_stream_packet_once)
	// Key: packetType << 16 | sequenceNum
	// For data packets, we might also want to track by sequence if multiple types exist.
}

func NewStreamServer(streamID uint16, sessionID uint8, arqConfig arq.Config, localConn io.ReadWriteCloser, mtu int, queueInitialCapacity int, logger arq.Logger) *Stream_server {
	if queueInitialCapacity < 1 {
		queueInitialCapacity = 32
	}
	s := &Stream_server{
		ID:           streamID,
		SessionID:    sessionID,
		TXQueue:      mlq.New[*serverStreamTXPacket](queueInitialCapacity),
		Status:       "PENDING",
		CreatedAt:    time.Now(),
		LastActivity: time.Now(),
	}

	s.ARQ = arq.NewARQ(streamID, sessionID, s, localConn, mtu, logger, arqConfig)
	s.ARQ.Start()
	return s
}

// PushTXPacket implements arq.PacketEnqueuer.
// It adds a packet to the stream's multi-level queue.
func (s *Stream_server) PushTXPacket(priority int, packetType uint8, sequenceNum uint16, fragmentID uint8, totalFragments uint8, compressionType uint8, ttl time.Duration, payload []byte) bool {
	s.mu.Lock()
	s.LastActivity = time.Now()
	s.mu.Unlock()

	priority = Enums.NormalizePacketPriority(packetType, priority)

	dataKey := Enums.PacketIdentityKey(s.ID, Enums.PACKET_STREAM_DATA, sequenceNum, fragmentID)
	resendKey := Enums.PacketIdentityKey(s.ID, Enums.PACKET_STREAM_RESEND, sequenceNum, fragmentID)
	key := Enums.PacketIdentityKey(s.ID, packetType, sequenceNum, fragmentID)

	pkt := getTXPacketFromPool()
	pkt.PacketType = packetType
	pkt.SequenceNum = sequenceNum
	pkt.FragmentID = fragmentID
	pkt.TotalFragments = totalFragments
	pkt.CompressionType = compressionType
	pkt.Payload = payload
	pkt.CreatedAt = time.Now()
	pkt.TTL = ttl

	s.txQueueMu.Lock()
	wasEmpty := s.TXQueue.FastSize() == 0

	switch packetType {
	case Enums.PACKET_STREAM_DATA:
		if _, exists := s.TXQueue.Get(dataKey); exists {
			s.txQueueMu.Unlock()
			putTXPacketToPool(pkt)
			return false
		}
		if _, exists := s.TXQueue.Get(resendKey); exists {
			s.txQueueMu.Unlock()
			putTXPacketToPool(pkt)
			return false
		}
	case Enums.PACKET_STREAM_RESEND:
		if _, exists := s.TXQueue.Get(resendKey); exists {
			s.txQueueMu.Unlock()
			putTXPacketToPool(pkt)
			return false
		}
	}

	ok := s.TXQueue.Push(priority, key, pkt)
	if !ok {
		// Packet already in queue or failed to push
		s.txQueueMu.Unlock()
		putTXPacketToPool(pkt)
		return false
	}

	if packetType == Enums.PACKET_STREAM_RESEND {
		if stale, removed := s.TXQueue.RemoveByKey(dataKey, func(p *serverStreamTXPacket) uint64 {
			return Enums.PacketIdentityKey(s.ID, p.PacketType, p.SequenceNum, p.FragmentID)
		}); removed {
			putTXPacketToPool(stale)
		}
	}

	isEmpty := s.TXQueue.FastSize() == 0
	s.txQueueMu.Unlock()

	if wasEmpty && !isEmpty {
		s.setTXQueueReady(true)
	}

	// Notify session that this stream is active (handled by the caller or session management)
	return true
}

func (s *Stream_server) setTXQueueReady(hasItems bool) {
	if s == nil {
		return
	}
	var next uint32
	if hasItems {
		next = 1
	}
	if s.txHasItems.Swap(next) == next {
		return
	}
	if s.onQueueStateChanged != nil {
		s.onQueueStateChanged(s.ID, hasItems)
	}
}

func (s *Stream_server) NoteTXPacketDequeued(packet *serverStreamTXPacket) {
	if s == nil || packet == nil || s.ARQ == nil {
		return
	}
	s.ARQ.NoteTXPacketDequeued(packet.PacketType, packet.SequenceNum, packet.FragmentID)
}

func (s *Stream_server) RemoveQueuedData(sequenceNum uint16) bool {
	if s == nil || s.TXQueue == nil {
		return false
	}

	s.txQueueMu.Lock()
	removedAny := false
	for _, packetType := range []uint8{Enums.PACKET_STREAM_DATA, Enums.PACKET_STREAM_RESEND} {
		key := Enums.PacketIdentityKey(s.ID, packetType, sequenceNum, 0)
		pkt, ok := s.TXQueue.RemoveByKey(key, func(p *serverStreamTXPacket) uint64 {
			return Enums.PacketIdentityKey(s.ID, p.PacketType, p.SequenceNum, p.FragmentID)
		})
		if ok {
			putTXPacketToPool(pkt)
			removedAny = true
		}
	}
	isEmpty := s.TXQueue.FastSize() == 0
	s.txQueueMu.Unlock()
	if removedAny && isEmpty {
		s.setTXQueueReady(false)
	}
	return removedAny
}

func (s *Stream_server) RemoveQueuedDataNack(sequenceNum uint16) bool {
	if s == nil || s.TXQueue == nil {
		return false
	}

	s.txQueueMu.Lock()
	key := Enums.PacketIdentityKey(s.ID, Enums.PACKET_STREAM_DATA_NACK, sequenceNum, 0)
	pkt, ok := s.TXQueue.RemoveByKey(key, func(p *serverStreamTXPacket) uint64 {
		return Enums.PacketIdentityKey(s.ID, p.PacketType, p.SequenceNum, p.FragmentID)
	})
	if !ok {
		s.txQueueMu.Unlock()
		return false
	}

	putTXPacketToPool(pkt)
	isEmpty := s.TXQueue.FastSize() == 0
	s.txQueueMu.Unlock()
	if isEmpty {
		s.setTXQueueReady(false)
	}
	return true
}

func (s *Stream_server) ClearTXQueue() {
	if s == nil || s.TXQueue == nil {
		return
	}

	s.txQueueMu.Lock()
	hadItems := s.TXQueue.FastSize() > 0
	s.TXQueue.Clear(func(pkt *serverStreamTXPacket) {
		putTXPacketToPool(pkt)
	})
	s.txQueueMu.Unlock()
	if hadItems {
		s.setTXQueueReady(false)
	}
}

func (s *Stream_server) FastTXQueueSize() int {
	if s == nil || s.TXQueue == nil {
		return 0
	}
	return s.TXQueue.FastSize()
}

func (s *Stream_server) PopNextTXPacket() (*serverStreamTXPacket, int, bool) {
	if s == nil || s.TXQueue == nil {
		return nil, 0, false
	}
	s.txQueueMu.Lock()
	packet, priority, ok := s.TXQueue.Pop(func(p *serverStreamTXPacket) uint64 {
		return Enums.PacketIdentityKey(s.ID, p.PacketType, p.SequenceNum, p.FragmentID)
	})
	isEmpty := s.TXQueue.FastSize() == 0
	s.txQueueMu.Unlock()
	if ok && isEmpty {
		s.setTXQueueReady(false)
	}
	return packet, priority, ok
}

func (s *Stream_server) PopAnyTXPacket(maxPriority int, predicate func(*serverStreamTXPacket) bool) (*serverStreamTXPacket, bool) {
	if s == nil || s.TXQueue == nil {
		return nil, false
	}
	s.txQueueMu.Lock()
	packet, ok := s.TXQueue.PopAnyIf(maxPriority, predicate, func(p *serverStreamTXPacket) uint64 {
		return Enums.PacketIdentityKey(s.ID, p.PacketType, p.SequenceNum, p.FragmentID)
	})
	isEmpty := s.TXQueue.FastSize() == 0
	s.txQueueMu.Unlock()
	if ok && isEmpty {
		s.setTXQueueReady(false)
	}
	return packet, ok
}

func (s *Stream_server) Abort(reason string) {
	s.CloseStream(true, 0, reason)
}

func (s *Stream_server) attachUpstreamConn(conn io.ReadWriteCloser, host string, port uint16, status string) bool {
	if s == nil || conn == nil {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Status == "CLOSED" || !s.CloseTime.IsZero() {
		return false
	}
	if s.ARQ != nil && s.ARQ.IsClosed() {
		return false
	}
	if s.UpstreamConn != nil || s.Connected {
		return false
	}

	s.UpstreamConn = conn
	s.TargetHost = host
	s.TargetPort = port
	s.Connected = true
	if status != "" {
		s.Status = status
	}
	s.LastActivity = time.Now()
	return true
}

func (s *Stream_server) cleanupResources() {
	var upstream io.ReadWriteCloser

	s.mu.Lock()
	s.Status = "CLOSED"
	s.CloseTime = time.Now()
	s.Connected = false
	upstream = s.UpstreamConn
	s.UpstreamConn = nil
	s.mu.Unlock()

	if upstream != nil {
		_ = upstream.Close()
	}
	s.ClearTXQueue()
}

func (s *Stream_server) finalizeAfterARQClose(reason string) {
	if s == nil {
		return
	}

	s.cleanupMu.Do(func() {
		now := time.Now()
		s.cleanupResources()
		if s.onClosed != nil {
			s.onClosed(s.ID, now, reason)
		}
	})
}

func (s *Stream_server) OnARQClosed(reason string) {
	s.finalizeAfterARQClose(reason)
}

func (s *Stream_server) closeUpstreamOnly(status string) {
	if s == nil {
		return
	}

	var upstream io.ReadWriteCloser

	s.mu.Lock()
	if status != "" {
		s.Status = status
	} else if s.Status != "CLOSED" {
		s.Status = "CLOSING"
	}
	s.CloseTime = time.Now()
	s.Connected = false
	upstream = s.UpstreamConn
	s.UpstreamConn = nil
	s.mu.Unlock()

	if upstream != nil {
		_ = upstream.Close()
	}
}

func (s *Stream_server) CloseStream(force bool, ttl time.Duration, reason string) {
	if s == nil {
		return
	}

	if s.ARQ != nil {
		if force {
			s.closeUpstreamOnly("CLOSED")
			s.ARQ.Close(reason, arq.CloseOptions{
				SendRST: true,
				TTL:     ttl,
			})
			return
		}

		s.ARQ.Close(reason, arq.CloseOptions{
			SendCloseRead: true,
			AfterDrain:    true,
			TTL:           ttl,
		})
		return
	}

	s.finalizeAfterARQClose(reason)
}
