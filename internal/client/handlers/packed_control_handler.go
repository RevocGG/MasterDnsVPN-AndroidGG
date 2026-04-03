// ==============================================================================
// MasterDnsVPN
// Author: MasterkinG32
// Github: https://github.com/masterking32
// Year: 2026
// ==============================================================================

package handlers

import (
	"net"

	Enums "masterdnsvpn-go/internal/enums"
	VpnProto "masterdnsvpn-go/internal/vpnproto"
)

func init() {
	RegisterHandler(Enums.PACKET_PACKED_CONTROL_BLOCKS, handlePackedControlBlocks)
}

func handlePackedControlBlocks(c ClientContext, packet VpnProto.Packet, addr *net.UDPAddr) error {
	payload := packet.Payload
	const blockSize = 7

	for i := 0; i+blockSize <= len(payload); i += blockSize {
		pType := payload[i]
		streamID := uint16(payload[i+1])<<8 | uint16(payload[i+2])
		seqNum := uint16(payload[i+3])<<8 | uint16(payload[i+4])
		fragID := payload[i+5]
		totalFrag := payload[i+6]

		innerPacket := VpnProto.Packet{
			SessionID:       packet.SessionID,
			PacketType:      pType,
			SessionCookie:   packet.SessionCookie,
			HasStreamID:     true,
			StreamID:        streamID,
			HasSequenceNum:  true,
			SequenceNum:     seqNum,
			HasFragmentInfo: true,
			FragmentID:      fragID,
			TotalFragments:  totalFrag,
		}

		if c.PreprocessInboundPacket(innerPacket) {
			continue
		}

		handler := dispatchTable[innerPacket.PacketType]
		var err error
		if handler != nil {
			err = handler(c, innerPacket, addr)
		} else {
			err = handleGenericPacket(c, innerPacket, addr)
		}

		if err != nil {
			c.Log().Debugf("Error dispatching packed block (Type %d, Stream %d): %v", pType, streamID, err)
		}
	}

	return nil
}
