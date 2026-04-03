package client

import (
	"testing"

	Enums "masterdnsvpn-go/internal/enums"
	"masterdnsvpn-go/internal/mlq"
)

func TestStreamZeroAllowsMultipleQueuedPingsWithDifferentSequence(t *testing.T) {
	c := &Client{
		txSignal: make(chan struct{}, 8),
	}
	s := &Stream_client{
		client:   c,
		StreamID: 0,
		txQueue:  mlq.New[*clientStreamTXPacket](16),
	}

	if !s.PushTXPacket(Enums.DefaultPacketPriority(Enums.PACKET_PING), Enums.PACKET_PING, 1, 0, 0, 0, 0, []byte("a")) {
		t.Fatal("expected first ping to be queued")
	}
	if !s.PushTXPacket(Enums.DefaultPacketPriority(Enums.PACKET_PING), Enums.PACKET_PING, 2, 0, 0, 0, 0, []byte("b")) {
		t.Fatal("expected second ping with distinct sequence to be queued")
	}
	if got := s.txQueue.FastSize(); got != 2 {
		t.Fatalf("expected two queued pings, got %d", got)
	}
	if s.PushTXPacket(Enums.DefaultPacketPriority(Enums.PACKET_PING), Enums.PACKET_PING, 2, 0, 0, 0, 0, []byte("dup")) {
		t.Fatal("expected duplicate ping sequence to be rejected")
	}
}

func TestPingManagerSequenceWrapsThroughUint16(t *testing.T) {
	p := &PingManager{}
	p.nextPingSeq.Store(0xFFFF)

	if got := p.nextPingSequence(); got != 0 {
		t.Fatalf("expected wrapped ping sequence 0, got %d", got)
	}
	if got := p.nextPingSequence(); got != 1 {
		t.Fatalf("expected next ping sequence 1 after wrap, got %d", got)
	}
}
