package vpnproto

import (
	"encoding/binary"
	"fmt"
	"math"
)

const (
	SessionAcceptBasePayloadSize   = 7
	SessionAcceptPolicyPayloadSize = 13
	SessionAcceptPayloadSize       = SessionAcceptBasePayloadSize + SessionAcceptPolicyPayloadSize

	SessionPolicyScaledMin = 0.05
	SessionPolicyScaledMax = 1.0
)

// SessionAcceptPayload defines the full SESSION_ACCEPT payload:
//
//	byte 0      : session ID
//	byte 1      : session cookie
//	byte 2      : upload/download compression pair
//	bytes 3-6   : verify code
//	byte 7      : lower nibble = max packet duplication, upper nibble = max setup duplication
//	byte 8      : max upload MTU
//	bytes 9-10  : max download MTU
//	byte 11     : max RX/TX workers
//	byte 12     : min ping aggressive interval (scaled 0..255 => 0.05..1.00s)
//	byte 13     : max packets per batch
//	bytes 14-15 : max ARQ window size
//	byte 16     : max ARQ data NACK max gap
//	bytes 17-18 : min compression min size
//	byte 19     : min ARQ initial RTO (scaled 0..255 => 0.05..1.00s)
type SessionAcceptPayload struct {
	SessionID           uint8
	SessionCookie       uint8
	CompressionPair     uint8
	VerifyCode          [4]byte
	ClientPolicy        SessionAcceptClientPolicy
	HasClientPolicySync bool
}

type SessionAcceptClientSettings struct {
	PacketDuplicationCount      int
	SetupPacketDuplicationCount int
	MaxUploadMTU                int
	MaxDownloadMTU              int
	RXTXWorkers                 int
	PingAggressiveInterval      float64
	MaxPacketsPerBatch          int
	ARQWindowSize               int
	ARQDataNackMaxGap           int
	CompressionMinSize          int
	ARQInitialRTOSeconds        float64
	ARQControlInitialRTOSeconds float64
	ARQMaxRTOSeconds            float64
	ARQControlMaxRTOSeconds     float64
}

// SessionAcceptClientPolicy defines the server-side client limits/minimums
// appended to SESSION_ACCEPT after the legacy 7-byte prefix.
//
// Wire layout after the first 7 bytes:
//
//	byte 7  : lower nibble = max packet duplication, upper nibble = max setup duplication
//	byte 8  : max upload MTU (uint8)
//	bytes 9-10  : max download MTU (uint16, big endian)
//	byte 11 : max RX/TX workers (uint8)
//	byte 12 : min ping aggressive interval, scaled 0..255 => 0.05..1.00 seconds
//	byte 13 : max packets per batch (uint8)
//	bytes 14-15 : max ARQ window size (uint16, big endian)
//	byte 16 : max ARQ data NACK max gap (uint8)
//	bytes 17-18 : min compression min-size (uint16, big endian)
//	byte 19 : min ARQ initial RTO, scaled 0..255 => 0.05..1.00 seconds
type SessionAcceptClientPolicy struct {
	MaxPacketDuplicationCount int
	MaxSetupDuplicationCount  int
	MaxUploadMTU              int
	MaxDownloadMTU            int
	MaxRxTxWorkers            int
	MinPingAggressiveInterval float64
	MaxPacketsPerBatch        int
	MaxARQWindowSize          int
	MaxARQDataNackMaxGap      int
	MinCompressionMinSize     int
	MinARQInitialRTOSeconds   float64
}

func EncodeSessionAcceptPayload(payload SessionAcceptPayload) []byte {
	size := SessionAcceptBasePayloadSize
	if payload.HasClientPolicySync {
		size = SessionAcceptPayloadSize
	}

	buf := make([]byte, size)
	buf[0] = payload.SessionID
	buf[1] = payload.SessionCookie
	buf[2] = payload.CompressionPair
	copy(buf[3:7], payload.VerifyCode[:])

	if payload.HasClientPolicySync {
		policy := EncodeSessionAcceptClientPolicy(payload.ClientPolicy)
		copy(buf[SessionAcceptBasePayloadSize:], policy[:])
	}

	return buf
}

func DecodeSessionAcceptPayload(payload []byte) (SessionAcceptPayload, error) {
	if len(payload) < SessionAcceptBasePayloadSize {
		return SessionAcceptPayload{}, fmt.Errorf("session accept payload too short: got=%d want>=%d", len(payload), SessionAcceptBasePayloadSize)
	}

	result := SessionAcceptPayload{
		SessionID:       payload[0],
		SessionCookie:   payload[1],
		CompressionPair: payload[2],
	}
	copy(result.VerifyCode[:], payload[3:7])

	if len(payload) >= SessionAcceptPayloadSize {
		policy, err := DecodeSessionAcceptClientPolicy(payload[SessionAcceptBasePayloadSize:SessionAcceptPayloadSize])
		if err != nil {
			return SessionAcceptPayload{}, err
		}
		result.ClientPolicy = policy
		result.HasClientPolicySync = true
	}

	return result, nil
}

func ApplySessionAcceptClientPolicy(current SessionAcceptClientSettings, policy SessionAcceptClientPolicy) SessionAcceptClientSettings {
	current.PacketDuplicationCount = clampInt(current.PacketDuplicationCount, 1, maxInt(policy.MaxPacketDuplicationCount, 1))
	current.SetupPacketDuplicationCount = clampInt(
		current.SetupPacketDuplicationCount,
		current.PacketDuplicationCount,
		maxInt(policy.MaxSetupDuplicationCount, current.PacketDuplicationCount),
	)

	current.MaxUploadMTU = clampInt(current.MaxUploadMTU, 1, maxInt(policy.MaxUploadMTU, 1))
	current.MaxDownloadMTU = clampInt(current.MaxDownloadMTU, 1, maxInt(policy.MaxDownloadMTU, 1))
	current.RXTXWorkers = clampInt(current.RXTXWorkers, 1, maxInt(policy.MaxRxTxWorkers, 1))
	current.PingAggressiveInterval = clampFloat64(current.PingAggressiveInterval, policy.MinPingAggressiveInterval, 30.0)
	current.MaxPacketsPerBatch = clampInt(current.MaxPacketsPerBatch, 1, maxInt(policy.MaxPacketsPerBatch, 1))
	current.ARQWindowSize = clampInt(current.ARQWindowSize, 1, maxInt(policy.MaxARQWindowSize, 1))
	current.ARQDataNackMaxGap = clampInt(current.ARQDataNackMaxGap, 0, maxInt(policy.MaxARQDataNackMaxGap, 0))
	current.CompressionMinSize = maxInt(current.CompressionMinSize, maxInt(policy.MinCompressionMinSize, 1))
	current.ARQInitialRTOSeconds = clampFloat64(current.ARQInitialRTOSeconds, policy.MinARQInitialRTOSeconds, current.ARQMaxRTOSeconds)
	current.ARQControlInitialRTOSeconds = clampFloat64(current.ARQControlInitialRTOSeconds, policy.MinARQInitialRTOSeconds, current.ARQControlMaxRTOSeconds)
	return current
}

func EncodeSessionAcceptClientPolicy(policy SessionAcceptClientPolicy) [SessionAcceptPolicyPayloadSize]byte {
	var payload [SessionAcceptPolicyPayloadSize]byte

	payload[0] = byte((clampInt(policy.MaxSetupDuplicationCount, 0, 15) << 4) |
		clampInt(policy.MaxPacketDuplicationCount, 0, 15))
	payload[1] = byte(clampInt(policy.MaxUploadMTU, 0, 0xFF))
	binary.BigEndian.PutUint16(payload[2:4], uint16(clampInt(policy.MaxDownloadMTU, 0, 0xFFFF)))
	payload[4] = byte(clampInt(policy.MaxRxTxWorkers, 0, 0xFF))
	payload[5] = EncodeSessionScaledByte(policy.MinPingAggressiveInterval)
	payload[6] = byte(clampInt(policy.MaxPacketsPerBatch, 0, 0xFF))
	binary.BigEndian.PutUint16(payload[7:9], uint16(clampInt(policy.MaxARQWindowSize, 0, 0xFFFF)))
	payload[9] = byte(clampInt(policy.MaxARQDataNackMaxGap, 0, 0xFF))
	binary.BigEndian.PutUint16(payload[10:12], uint16(clampInt(policy.MinCompressionMinSize, 0, 0xFFFF)))
	payload[12] = EncodeSessionScaledByte(policy.MinARQInitialRTOSeconds)

	return payload
}

func DecodeSessionAcceptClientPolicy(payload []byte) (SessionAcceptClientPolicy, error) {
	if len(payload) < SessionAcceptPolicyPayloadSize {
		return SessionAcceptClientPolicy{}, fmt.Errorf("session accept policy payload too short: got=%d want=%d", len(payload), SessionAcceptPolicyPayloadSize)
	}

	return SessionAcceptClientPolicy{
		MaxPacketDuplicationCount: int(payload[0] & 0x0F),
		MaxSetupDuplicationCount:  int((payload[0] >> 4) & 0x0F),
		MaxUploadMTU:              int(payload[1]),
		MaxDownloadMTU:            int(binary.BigEndian.Uint16(payload[2:4])),
		MaxRxTxWorkers:            int(payload[4]),
		MinPingAggressiveInterval: DecodeSessionScaledByte(payload[5]),
		MaxPacketsPerBatch:        int(payload[6]),
		MaxARQWindowSize:          int(binary.BigEndian.Uint16(payload[7:9])),
		MaxARQDataNackMaxGap:      int(payload[9]),
		MinCompressionMinSize:     int(binary.BigEndian.Uint16(payload[10:12])),
		MinARQInitialRTOSeconds:   DecodeSessionScaledByte(payload[12]),
	}, nil
}

func EncodeSessionScaledByte(value float64) uint8 {
	clamped := clampFloat64(value, SessionPolicyScaledMin, SessionPolicyScaledMax)
	if SessionPolicyScaledMax <= SessionPolicyScaledMin {
		return 0
	}

	normalized := (clamped - SessionPolicyScaledMin) / (SessionPolicyScaledMax - SessionPolicyScaledMin)
	return uint8(math.Round(normalized * 255.0))
}

func DecodeSessionScaledByte(value uint8) float64 {
	normalized := float64(value) / 255.0
	return SessionPolicyScaledMin + normalized*(SessionPolicyScaledMax-SessionPolicyScaledMin)
}

func clampFloat64(value float64, minValue float64, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}

	if value > maxValue {
		return maxValue
	}

	return value
}

func clampInt(value int, minValue int, maxValue int) int {
	if value < minValue {
		return minValue
	}

	if value > maxValue {
		return maxValue
	}

	return value
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}

	return b
}
