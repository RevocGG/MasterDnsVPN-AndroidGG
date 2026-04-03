// ==============================================================================
// MasterDnsVPN
// Author: MasterkinG32
// Github: https://github.com/masterking32
// Year: 2026
// ==============================================================================

package dnsparser

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"masterdnsvpn-go/internal/compression"
	Enums "masterdnsvpn-go/internal/enums"
	VpnProto "masterdnsvpn-go/internal/vpnproto"
)

func TestBuildTunnelQuestionNameSplitsLabels(t *testing.T) {
	name, err := BuildTunnelQuestionName("v.example.com", stringsOf('a', 130))
	if err != nil {
		t.Fatalf("BuildTunnelQuestionName returned error: %v", err)
	}
	if len(name) > maxDNSNameLen {
		t.Fatalf("name exceeds max length: %d", len(name))
	}
}

func TestBuildAndExtractVPNResponsePacketSingleAnswer(t *testing.T) {
	query, err := BuildTXTQuestionPacket("x.v.example.com", Enums.DNS_RECORD_TYPE_TXT, 4096)
	if err != nil {
		t.Fatalf("BuildTXTQuestionPacket returned error: %v", err)
	}

	response, err := BuildVPNResponsePacket(query, "x.v.example.com", VpnProto.Packet{
		SessionID:  9,
		PacketType: Enums.PACKET_MTU_UP_RES,
		Payload:    []byte("challenge"),
	}, false)
	if err != nil {
		t.Fatalf("BuildVPNResponsePacket returned error: %v", err)
	}

	packet, err := ExtractVPNResponse(response, false)
	if err != nil {
		t.Fatalf("ExtractVPNResponse returned error: %v", err)
	}
	if packet.PacketType != Enums.PACKET_MTU_UP_RES {
		t.Fatalf("unexpected packet type: got=%d want=%d", packet.PacketType, Enums.PACKET_MTU_UP_RES)
	}
	if !bytes.Equal(packet.Payload, []byte("challenge")) {
		t.Fatalf("unexpected payload: got=%q", packet.Payload)
	}
}

func TestBuildAndExtractVPNResponsePacketChunked(t *testing.T) {
	query, err := BuildTXTQuestionPacket("x.v.example.com", Enums.DNS_RECORD_TYPE_TXT, 4096)
	if err != nil {
		t.Fatalf("BuildTXTQuestionPacket returned error: %v", err)
	}

	payload := bytes.Repeat([]byte{0xAB}, 700)
	response, err := BuildVPNResponsePacket(query, "x.v.example.com", VpnProto.Packet{
		SessionID:   7,
		PacketType:  Enums.PACKET_MTU_DOWN_RES,
		StreamID:    1,
		SequenceNum: 2,
		Payload:     payload,
	}, false)
	if err != nil {
		t.Fatalf("BuildVPNResponsePacket returned error: %v", err)
	}

	packet, err := ExtractVPNResponse(response, false)
	if err != nil {
		t.Fatalf("ExtractVPNResponse returned error: %v", err)
	}
	if packet.PacketType != Enums.PACKET_MTU_DOWN_RES {
		t.Fatalf("unexpected packet type: got=%d want=%d", packet.PacketType, Enums.PACKET_MTU_DOWN_RES)
	}
	if !bytes.Equal(packet.Payload, payload) {
		t.Fatalf("unexpected chunked payload size: got=%d want=%d", len(packet.Payload), len(payload))
	}
}

func TestBuildAndExtractVPNResponsePacketSingleAnswerBaseEncoded(t *testing.T) {
	query, err := BuildTXTQuestionPacket("x.v.example.com", Enums.DNS_RECORD_TYPE_TXT, 4096)
	if err != nil {
		t.Fatalf("BuildTXTQuestionPacket returned error: %v", err)
	}

	response, err := BuildVPNResponsePacket(query, "x.v.example.com", VpnProto.Packet{
		SessionID:  9,
		PacketType: Enums.PACKET_MTU_UP_RES,
		Payload:    []byte("challenge"),
	}, true)
	if err != nil {
		t.Fatalf("BuildVPNResponsePacket returned error: %v", err)
	}

	packet, err := ExtractVPNResponse(response, true)
	if err != nil {
		t.Fatalf("ExtractVPNResponse returned error: %v", err)
	}
	if packet.PacketType != Enums.PACKET_MTU_UP_RES {
		t.Fatalf("unexpected packet type: got=%d want=%d", packet.PacketType, Enums.PACKET_MTU_UP_RES)
	}
	if !bytes.Equal(packet.Payload, []byte("challenge")) {
		t.Fatalf("unexpected payload: got=%q", packet.Payload)
	}
}

func TestBuildAndExtractVPNResponsePacketChunkedBaseEncoded(t *testing.T) {
	query, err := BuildTXTQuestionPacket("x.v.example.com", Enums.DNS_RECORD_TYPE_TXT, 4096)
	if err != nil {
		t.Fatalf("BuildTXTQuestionPacket returned error: %v", err)
	}

	payload := bytes.Repeat([]byte{0xAB}, 700)
	response, err := BuildVPNResponsePacket(query, "x.v.example.com", VpnProto.Packet{
		SessionID:   7,
		PacketType:  Enums.PACKET_MTU_DOWN_RES,
		StreamID:    1,
		SequenceNum: 2,
		Payload:     payload,
	}, true)
	if err != nil {
		t.Fatalf("BuildVPNResponsePacket returned error: %v", err)
	}

	packet, err := ExtractVPNResponse(response, true)
	if err != nil {
		t.Fatalf("ExtractVPNResponse returned error: %v", err)
	}
	if packet.PacketType != Enums.PACKET_MTU_DOWN_RES {
		t.Fatalf("unexpected packet type: got=%d want=%d", packet.PacketType, Enums.PACKET_MTU_DOWN_RES)
	}
	if !bytes.Equal(packet.Payload, payload) {
		t.Fatalf("unexpected chunked payload size: got=%d want=%d", len(packet.Payload), len(payload))
	}
}

func TestBuildAndExtractVPNResponsePacketCompressed(t *testing.T) {
	query, err := BuildTXTQuestionPacket("x.v.example.com", Enums.DNS_RECORD_TYPE_TXT, 4096)
	if err != nil {
		t.Fatalf("BuildTXTQuestionPacket returned error: %v", err)
	}

	payload := bytes.Repeat([]byte("abcdabcdabcdabcd"), 16)
	response, err := BuildVPNResponsePacket(query, "x.v.example.com", VpnProto.Packet{
		SessionID:       7,
		PacketType:      Enums.PACKET_STREAM_DATA,
		SessionCookie:   9,
		StreamID:        1,
		SequenceNum:     2,
		FragmentID:      0,
		TotalFragments:  1,
		CompressionType: compression.TypeZLIB,
		Payload:         payload,
	}, false)
	if err != nil {
		t.Fatalf("BuildVPNResponsePacket returned error: %v", err)
	}

	packet, err := ExtractVPNResponse(response, false)
	if err != nil {
		t.Fatalf("ExtractVPNResponse returned error: %v", err)
	}
	if packet.PacketType != Enums.PACKET_STREAM_DATA {
		t.Fatalf("unexpected packet type: got=%d want=%d", packet.PacketType, Enums.PACKET_STREAM_DATA)
	}
	if !bytes.Equal(packet.Payload, payload) {
		t.Fatal("unexpected inflated payload")
	}
}

func TestExtractVPNResponseReordersChunkedAnswers(t *testing.T) {
	query, err := BuildTXTQuestionPacket("x.v.example.com", Enums.DNS_RECORD_TYPE_TXT, 4096)
	if err != nil {
		t.Fatalf("BuildTXTQuestionPacket returned error: %v", err)
	}

	rawFrame, err := VpnProto.BuildRaw(VpnProto.BuildOptions{
		SessionID:   7,
		PacketType:  Enums.PACKET_MTU_DOWN_RES,
		StreamID:    1,
		SequenceNum: 2,
		Payload:     bytes.Repeat([]byte{0xCD}, 700),
	})
	if err != nil {
		t.Fatalf("BuildRaw returned error: %v", err)
	}

	chunks, err := buildTXTAnswerChunks(rawFrame, false)
	if err != nil {
		t.Fatalf("buildTXTAnswerChunks returned error: %v", err)
	}
	if len(chunks) < 3 {
		t.Fatalf("expected chunked answers, got=%d", len(chunks))
	}

	reordered := make([][]byte, len(chunks))
	copy(reordered, chunks)
	reordered[1], reordered[2] = reordered[2], reordered[1]

	response, err := BuildTXTResponsePacket(query, "x.v.example.com", reordered)
	if err != nil {
		t.Fatalf("BuildTXTResponsePacket returned error: %v", err)
	}

	packet, err := ExtractVPNResponse(response, false)
	if err != nil {
		t.Fatalf("ExtractVPNResponse returned error: %v", err)
	}
	if packet.PacketType != Enums.PACKET_MTU_DOWN_RES {
		t.Fatalf("unexpected packet type: got=%d want=%d", packet.PacketType, Enums.PACKET_MTU_DOWN_RES)
	}
	if len(packet.Payload) != 700 {
		t.Fatalf("unexpected payload len: got=%d want=%d", len(packet.Payload), 700)
	}
}

func TestBuildTXTAnswerChunksRejectsTooManyChunks(t *testing.T) {
	rawFrame, err := VpnProto.BuildRaw(VpnProto.BuildOptions{
		SessionID:   7,
		PacketType:  Enums.PACKET_MTU_DOWN_RES,
		StreamID:    1,
		SequenceNum: 2,
		Payload:     bytes.Repeat([]byte{0xEF}, 70000),
	})
	if err != nil {
		t.Fatalf("BuildRaw returned error: %v", err)
	}

	_, err = buildTXTAnswerChunks(rawFrame, false)
	if err == nil {
		t.Fatal("expected chunk overflow error, got nil")
	}
	if !errors.Is(err, ErrTXTAnswerTooLarge) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func stringsOf(ch byte, count int) string {
	buf := make([]byte, count)
	for i := range buf {
		buf[i] = ch
	}
	return string(buf)
}

func TestDescribeResponseWithoutTunnelPayloadEmptyNoError(t *testing.T) {
	query, err := BuildTXTQuestionPacket("x.v.example.com", Enums.DNS_RECORD_TYPE_TXT, 4096)
	if err != nil {
		t.Fatalf("BuildTXTQuestionPacket returned error: %v", err)
	}

	response, err := BuildEmptyNoErrorResponse(query)
	if err != nil {
		t.Fatalf("BuildEmptyNoErrorResponse returned error: %v", err)
	}

	summary := DescribeResponseWithoutTunnelPayload(response)
	for _, want := range []string{
		"RCODE=0",
		"QD=1",
		"AN=0",
		"QName=x.v.example.com",
		"Answers=none",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q: %s", want, summary)
		}
	}
}
