// ==============================================================================
// MasterDnsVPN
// Author: MasterkinG32
// Github: https://github.com/masterking32
// Year: 2026
// ==============================================================================

package client

import (
	"context"
	"slices"
	"time"

	"masterdnsvpn-go/internal/arq"
	Enums "masterdnsvpn-go/internal/enums"
	VpnProto "masterdnsvpn-go/internal/vpnproto"
)

// asyncStreamDispatcher cycles through all active streams using a fair
// round-robin algorithm and hands prepared tasks to the encode queue.
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
		case <-c.dispatchSignal:
		case <-c.plannerQueueSpaceSignal:
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

	waitForPlannerCapacity := func(required int) bool {
		if required <= 0 {
			return true
		}
		for {
			if c.plannerQueueHasCapacity(required) {
				return true
			}

			select {
			case <-ctx.Done():
				return false
			case <-c.plannerQueueSpaceSignal:
			case <-idleTimer.C:
			}

			if !idleTimer.Stop() {
				select {
				case <-idleTimer.C:
				default:
				}
			}

			idleTimer.Reset(idlePoll)
		}
	}

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
			slices.Sort(ids)
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

		if !waitForPlannerCapacity(1) {
			if ctx.Err() != nil {
				return
			}
			continue
		}

		var item *clientStreamTXPacket
		var ok bool
		if selected != nil {
			item, _, ok = selected.PopNextTXPacket()
			if !ok || item == nil {
				continue
			}
		} else {
			p, _, ok := c.orphanQueue.Pop()
			if !ok {
				continue
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
			continue
		}

		var finalPacketType uint8
		var finalPayload []byte
		wasPacked := false
		maxBlocks := c.maxPackedBlocks
		if maxBlocks < 1 {
			maxBlocks = 1
		}

		if VpnProto.IsPackableControlPacket(item.PacketType, len(item.Payload)) && maxBlocks > 1 {
			payload := make([]byte, 0, maxBlocks*VpnProto.PackedControlBlockSize)
			payload = VpnProto.AppendPackedControlBlock(payload, item.PacketType, selectedStreamID, item.SequenceNum, item.FragmentID, item.TotalFragments)
			blocks := 1

			if selected != nil && selected.controlCount.Load() > 0 {
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
					if otherStream == nil || otherStream.txQueue == nil || otherStream.controlCount.Load() <= 0 {
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
				finalPacketType = Enums.PACKET_PACKED_CONTROL_BLOCKS
				finalPayload = payload
				wasPacked = true
				if selected != nil {
					selected.ReleaseTXPacket(item)
				}
				item = nil
			} else {
				finalPacketType = item.PacketType
				finalPayload = item.Payload
			}
		} else {
			finalPacketType = item.PacketType
			finalPayload = item.Payload
		}

		c.pingManager.NotifyPacket(finalPacketType, false)

		opts := VpnProto.BuildOptions{
			SessionID:     c.sessionID,
			SessionCookie: c.sessionCookie,
			PacketType:    finalPacketType,
			CompressionType: func() uint8 {
				if wasPacked {
					return c.uploadCompression
				}
				return item.CompressionType
			}(),
			Payload: finalPayload,
		}

		if wasPacked {
			opts.StreamID = 0
		} else {
			opts.StreamID = selectedStreamID
			opts.SequenceNum = item.SequenceNum
			opts.FragmentID = item.FragmentID
			opts.TotalFragments = item.TotalFragments
		}

		task := plannerTask{
			opts:      opts,
			dupCount:  c.runtimePacketDuplicationCount(finalPacketType),
			wasPacked: wasPacked,
			item:      item,
			selected:  selected,
		}

		select {
		case c.plannerQueue <- task:
		case <-ctx.Done():
			if !wasPacked && selected != nil {
				selected.ReleaseTXPacket(item)
			}
			return
		}
	}
}

func (c *Client) shouldTransmitQueuedStreamPacket(stream *Stream_client, item *clientStreamTXPacket) bool {
	if c == nil || stream == nil || item == nil {
		return false
	}

	if item.PacketType != Enums.PACKET_STREAM_DATA && item.PacketType != Enums.PACKET_STREAM_RESEND {
		return true
	}

	arqObj, ok := stream.Stream.(*arq.ARQ)
	if !ok || arqObj == nil {
		return false
	}

	return arqObj.HasPendingSequence(item.SequenceNum)
}
