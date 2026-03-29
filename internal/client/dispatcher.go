// ==============================================================================
// MasterDnsVPN
// Author: MasterkinG32
// Github: https://github.com/masterking32
// Year: 2026
// ==============================================================================

package client

import (
	"context"
	"sort"
	"time"

	Enums "masterdnsvpn-go/internal/enums"
	VpnProto "masterdnsvpn-go/internal/vpnproto"
)

func (c *Client) selectTargetConnections(packetType uint8, streamID uint16) []Connection {
	connections, err := c.selectTargetConnectionsForPacket(packetType, streamID)
	if err != nil {
		return nil
	}

	return connections
}

// asyncStreamDispatcher cycles through all active streams using a fair Round-Robin algorithm
// and transmits the highest priority packets to the TX workers, packing control blocks when possible.
func (c *Client) asyncStreamDispatcher(ctx context.Context) {
	c.log.Debugf("Stream Dispatcher started")
	defer c.asyncWG.Done()

	var rrCursor int32 = -1
	var cachedVersion uint64
	var cachedIDs []int32
	var cachedStreams map[uint16]*Stream_client
	idlePoll := c.cfg.DispatcherIdlePollInterval()
	idleTimer := time.NewTimer(idlePoll)
	defer idleTimer.Stop()

	waitForWork := func() bool {
		select {
		case <-ctx.Done():
			return false
		case <-c.txSignal:
		case <-c.txSpaceSignal:
		case <-idleTimer.C:
		}
		if !idleTimer.Stop() {
			select {
			case <-idleTimer.C:
			default:
			}
		}
		idleTimer.Reset(idlePoll)
		return true
	}

dispatchLoop:
	for {
		currentVersion := c.streamSetVersion.Load()
		if currentVersion != cachedVersion || cachedIDs == nil || cachedStreams == nil {
			c.streamsMu.RLock()
			streamCount := len(c.active_streams)
			ids := make([]int32, 0, streamCount+1)
			streams := make(map[uint16]*Stream_client, streamCount)
			for id, stream := range c.active_streams {
				ids = append(ids, int32(id))
				streams[id] = stream
			}
			c.streamsMu.RUnlock()
			sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
			cachedIDs = ids
			cachedStreams = streams
			cachedVersion = currentVersion
		}

		ids := cachedIDs
		streams := cachedStreams

		if c.orphanQueue != nil && c.orphanQueue.FastSize() > 0 {
			ids = append(ids[:len(ids):len(ids)], -1)
		}

		if len(ids) == 0 {
			if !waitForWork() {
				return
			}
			continue
		}

		var selected *Stream_client
		var peekedItem *clientStreamTXPacket
		var peekedOK bool
		var selectedStreamID uint16
		var selectedID int32 = -2
		rrApplied := false

		startIndex := -1
		for i, id := range ids {
			if id >= rrCursor {
				startIndex = i
				break
			}
		}
		if startIndex == -1 {
			startIndex = 0
		}

		for i := 0; i < len(ids); i++ {
			idx := (startIndex + i) % len(ids)
			id := ids[idx]

			if id == -1 {
				if c.orphanQueue == nil || c.orphanQueue.FastSize() == 0 {
					continue
				}
				p, _, ok := c.orphanQueue.Peek()
				if !ok {
					continue
				}

				peekedItem = &clientStreamTXPacket{
					PacketType:     p.PacketType,
					SequenceNum:    p.SequenceNum,
					FragmentID:     p.FragmentID,
					TotalFragments: p.TotalFragments,
					Payload:        nil,
				}

				selectedStreamID = p.StreamID
				selectedID = -1
				peekedOK = true
			} else {
				s := streams[uint16(id)]
				if s == nil || s.txQueue == nil {
					continue
				}
				peekedItem, _, peekedOK = s.txQueue.Peek()
				if peekedOK {
					selectedStreamID = uint16(id)
					selectedID = int32(id)
					selected = s
				}
			}

			if peekedOK && peekedItem != nil {
				if !rrApplied {
					rrCursor = id + 1
					rrApplied = true
				}

				if id == 0 && peekedItem.PacketType == Enums.PACKET_PING {
					hasOtherWork := false
					for _, otherID := range ids {
						if otherID == 0 {
							continue
						}
						if otherID == -1 {
							if c.orphanQueue != nil && c.orphanQueue.FastSize() > 0 {
								hasOtherWork = true
								break
							}
							continue
						}
						os := streams[uint16(otherID)]
						if os != nil && os.txQueue != nil && os.txQueue.FastSize() > 0 {
							hasOtherWork = true
							break
						}
					}
					if hasOtherWork {
						peekedItem = nil
						peekedOK = false
						continue
					}
				}

				break
			}
		}

		if selectedID == -2 || peekedItem == nil {
			if !waitForWork() {
				return
			}
			continue
		}

		conns := c.selectTargetConnections(peekedItem.PacketType, selectedStreamID)
		if len(conns) == 0 {
			if !waitForWork() {
				return
			}
			continue dispatchLoop
		}

		if !c.txChannelHasCapacity(len(conns)) {
			if !waitForWork() {
				return
			}
			continue dispatchLoop
		}

		var item *clientStreamTXPacket
		var ok bool
		if selected != nil {
			item, _, ok = selected.PopNextTXPacket()
			if !ok || item == nil {
				continue dispatchLoop
			}
		} else {
			p, _, ok := c.orphanQueue.Pop(func(p VpnProto.Packet) uint64 {
				return Enums.PacketTypeStreamKey(p.StreamID, p.PacketType)
			})
			if !ok {
				continue dispatchLoop
			}
			item = &clientStreamTXPacket{
				PacketType:     p.PacketType,
				SequenceNum:    p.SequenceNum,
				FragmentID:     p.FragmentID,
				TotalFragments: p.TotalFragments,
				Payload:        nil,
			}
		}

		if selected != nil &&
			(item.PacketType == Enums.PACKET_STREAM_DATA || item.PacketType == Enums.PACKET_STREAM_RESEND) &&
			!c.shouldTransmitQueuedStreamPacket(selected, item) {
			selected.ReleaseTXPacket(item)
			select {
			case c.txSignal <- struct{}{}:
			default:
			}
			continue dispatchLoop
		}

		var finalPacket asyncPacket
		wasPacked := false
		maxBlocks := c.maxPackedBlocks
		if maxBlocks < 1 {
			maxBlocks = 1
		}

		if VpnProto.IsPackableControlPacket(item.PacketType, len(item.Payload)) && maxBlocks > 1 {
			payload := make([]byte, 0, maxBlocks*VpnProto.PackedControlBlockSize)
			payload = VpnProto.AppendPackedControlBlock(payload, item.PacketType, selectedStreamID, item.SequenceNum, item.FragmentID, item.TotalFragments)
			blocks := 1

			if selected != nil {
				for blocks < maxBlocks {
					popped, poppedOK := selected.txQueue.PopAnyIf(2, func(p *clientStreamTXPacket) bool {
						return VpnProto.IsPackableControlPacket(p.PacketType, len(p.Payload))
					}, func(p *clientStreamTXPacket) uint64 {
						return Enums.PacketIdentityKey(selected.StreamID, p.PacketType, p.SequenceNum, p.FragmentID)
					})
					if !poppedOK {
						break
					}
					selected.NoteTXPacketDequeued(popped)
					payload = VpnProto.AppendPackedControlBlock(payload, popped.PacketType, selected.StreamID, popped.SequenceNum, popped.FragmentID, popped.TotalFragments)
					blocks++
					selected.ReleaseTXPacket(popped)
				}
			} else if selectedID == -1 {
				for blocks < maxBlocks {
					popped, poppedOK := c.orphanQueue.PopAnyIf(2, func(p VpnProto.Packet) bool {
						return VpnProto.IsPackableControlPacket(p.PacketType, 0)
					}, func(p VpnProto.Packet) uint64 {
						return Enums.PacketTypeStreamKey(p.StreamID, p.PacketType)
					})
					if !poppedOK {
						break
					}
					payload = VpnProto.AppendPackedControlBlock(payload, popped.PacketType, popped.StreamID, popped.SequenceNum, popped.FragmentID, popped.TotalFragments)
					blocks++
				}
			}

			if blocks < maxBlocks {
				for _, otherID := range ids {
					if blocks >= maxBlocks {
						break
					}
					if otherID == selectedID {
						continue
					}

					if otherID == -1 {
						for blocks < maxBlocks {
							popped, poppedOK := c.orphanQueue.PopAnyIf(2, func(p VpnProto.Packet) bool {
								return VpnProto.IsPackableControlPacket(p.PacketType, 0)
							}, func(p VpnProto.Packet) uint64 {
								return Enums.PacketTypeStreamKey(p.StreamID, p.PacketType)
							})
							if !poppedOK {
								break
							}
							payload = VpnProto.AppendPackedControlBlock(payload, popped.PacketType, popped.StreamID, popped.SequenceNum, popped.FragmentID, popped.TotalFragments)
							blocks++
						}
						continue
					}

					otherStream := streams[uint16(otherID)]
					if otherStream == nil || otherStream.txQueue == nil {
						continue
					}
					for blocks < maxBlocks {
						popped, poppedOK := otherStream.txQueue.PopAnyIf(2, func(p *clientStreamTXPacket) bool {
							return VpnProto.IsPackableControlPacket(p.PacketType, len(p.Payload))
						}, func(p *clientStreamTXPacket) uint64 {
							return Enums.PacketIdentityKey(uint16(otherID), p.PacketType, p.SequenceNum, p.FragmentID)
						})
						if !poppedOK {
							break
						}
						otherStream.NoteTXPacketDequeued(popped)
						payload = VpnProto.AppendPackedControlBlock(payload, popped.PacketType, uint16(otherID), popped.SequenceNum, popped.FragmentID, popped.TotalFragments)
						blocks++
						otherStream.ReleaseTXPacket(popped)
					}
				}
			}

			if blocks > 1 {
				finalPacket.packetType = Enums.PACKET_PACKED_CONTROL_BLOCKS
				finalPacket.payload = payload
				wasPacked = true
				if selected != nil {
					selected.ReleaseTXPacket(item)
				}
			} else {
				finalPacket.packetType = item.PacketType
				finalPacket.payload = item.Payload
			}
		} else {
			finalPacket.packetType = item.PacketType
			finalPacket.payload = item.Payload
		}

		c.pingManager.NotifyPacket(finalPacket.packetType, false)
		finalPacket.streamID = selectedStreamID

		opts := VpnProto.BuildOptions{
			SessionID:     c.sessionID,
			SessionCookie: c.sessionCookie,
			PacketType:    finalPacket.packetType,
			CompressionType: func() uint8 {
				if wasPacked {
					return c.uploadCompression
				}
				return item.CompressionType
			}(),
			Payload: finalPacket.payload,
		}

		if wasPacked {
			opts.StreamID = 0
		} else {
			opts.StreamID = selectedStreamID
			opts.SequenceNum = item.SequenceNum
			opts.FragmentID = item.FragmentID
			opts.TotalFragments = item.TotalFragments
		}

		encoded, err := c.buildEncodedAutoWithCompressionTrace(opts)
		if err != nil {
			if !wasPacked && selected != nil {
				selected.ReleaseTXPacket(item)
			}
			continue dispatchLoop
		}

		packetByDomain := make(map[string][]byte, len(conns))
		// var isLogged bool = false
		for _, conn := range conns {
			domain := conn.Domain
			if domain == "" {
				domain = c.cfg.Domains[0]
			}

			dnsPacket, ok := packetByDomain[domain]
			if !ok {
				dnsPacket, err = buildTunnelTXTQuestion(domain, encoded)
				if err != nil {
					continue
				}
				packetByDomain[domain] = dnsPacket
			}

			pkt := finalPacket
			pkt.conn = conn
			pkt.payload = dnsPacket

			select {
			case c.txChannel <- pkt:
				// if !isLogged && pkt.packetType != Enums.PACKET_PING {
				// 	packedSummary := ""
				// 	if opts.PacketType == Enums.PACKET_PACKED_CONTROL_BLOCKS {
				// 		packedSummary = " | " + VpnProto.DescribePackedControlBlocks(opts.Payload, 4)
				// 	}
				// 	c.logOutboundPacket(opts.PacketType, opts.SessionID, len(opts.Payload), opts.StreamID, opts.SequenceNum, opts.FragmentID, opts.TotalFragments, packedSummary)
				// }
				// isLogged = true
			default:
				c.log.Warnf("TX channel filled before enqueue completed | Packet: %s | Stream: %d", Enums.PacketTypeName(finalPacket.packetType), selectedStreamID)
			}
		}

		if !wasPacked && selected != nil {
			selected.ReleaseTXPacket(item)
		}

		select {
		case c.txSignal <- struct{}{}:
		default:
		}
	}
}
