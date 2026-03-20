// ==============================================================================
// MasterDnsVPN
// Author: MasterkinG32
// Github: https://github.com/masterking32
// Year: 2026
// ==============================================================================

package client

import (
	"context"
	"errors"
	"sync"
	"time"

	"masterdnsvpn-go/internal/arq"
	Enums "masterdnsvpn-go/internal/enums"
	VpnProto "masterdnsvpn-go/internal/vpnproto"
)

var ErrStream0RuntimeStopped = errors.New("stream 0 runtime stopped")

var (
	stream0DNSRetryBaseDelay       = 350 * time.Millisecond
	stream0DNSRetryMaxDelay        = 2 * time.Second
	stream0DNSOnlyWarmDuration     = 60 * time.Second
	stream0DNSOnlyWarmPingInterval = time.Second
	stream0PingIdleHighThreshold   = 10 * time.Second
	stream0PingIdleMediumThreshold = 5 * time.Second
	stream0DNSOnlyPingInterval     = 30 * time.Second
	stream0PingHighIdleInterval    = 3 * time.Second
	stream0PingMediumIdleInterval  = time.Second
	stream0PingBusyInterval        = 200 * time.Millisecond
	stream0DNSOnlyWarmMaxSleep     = 500 * time.Millisecond
	stream0PingDNSOnlyMaxSleep     = time.Second
	stream0PingHighIdleMaxSleep    = 500 * time.Millisecond
	stream0PingMediumIdleMaxSleep  = 200 * time.Millisecond
	stream0PingBusyMaxSleep        = 180 * time.Millisecond
)

type stream0DNSRequestState struct {
	fragments map[uint8]*stream0DNSFragmentState
}

type stream0DNSFragmentState struct {
	packet     arq.QueuedPacket
	createdAt  time.Time
	retryAt    time.Time
	retryDelay time.Duration
	retryCount int
	scheduled  bool
}

type stream0Runtime struct {
	client    *Client
	scheduler *arq.Scheduler

	mu               sync.Mutex
	running          bool
	ctx              context.Context
	cancel           context.CancelFunc
	wg               sync.WaitGroup
	wakeCh           chan struct{}
	dnsRequests      map[uint16]*stream0DNSRequestState
	dnsActivitySeen  bool
	lastDataActivity time.Time
	lastPingTime     time.Time
}

func newStream0Runtime(client *Client) *stream0Runtime {
	now := time.Now()
	return &stream0Runtime{
		client:           client,
		scheduler:        arq.NewScheduler(1),
		wakeCh:           make(chan struct{}, 1),
		dnsRequests:      make(map[uint16]*stream0DNSRequestState, 16),
		lastDataActivity: now,
		lastPingTime:     now,
	}
}

func (r *stream0Runtime) Start(parent context.Context) error {
	if r == nil {
		return ErrStream0RuntimeStopped
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running {
		return nil
	}

	if parent == nil {
		parent = context.Background()
	}
	r.ctx, r.cancel = context.WithCancel(parent)
	r.running = true
	r.lastDataActivity = time.Now()
	r.lastPingTime = r.lastDataActivity
	r.scheduler.SetMaxPackedBlocks(r.client.MaxPackedBlocks())
	r.wg.Add(2)
	go r.txLoop()
	go r.pingLoop()
	return nil
}

func (r *stream0Runtime) IsRunning() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running
}

func (r *stream0Runtime) SetMaxPackedBlocks(limit int) {
	if r == nil {
		return
	}
	r.scheduler.SetMaxPackedBlocks(limit)
}

func (r *stream0Runtime) NotifyDNSActivity() {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.dnsActivitySeen = true
	r.lastDataActivity = time.Now()
	r.mu.Unlock()
}

func (r *stream0Runtime) QueueMainPacket(packet arq.QueuedPacket) bool {
	if r == nil || !r.IsRunning() {
		return false
	}
	if packet.Priority == 0 {
		packet.Priority = arq.DefaultPriorityForPacket(packet.PacketType)
	}
	if !r.scheduler.Enqueue(arq.QueueTargetMain, packet) {
		return false
	}
	r.notifyWake()
	return true
}

func (r *stream0Runtime) QueueDNSRequest(payload []byte) error {
	if r == nil || !r.IsRunning() {
		return ErrStream0RuntimeStopped
	}
	if r.client == nil || !r.client.SessionReady() {
		return ErrSessionInitFailed
	}

	fragments, err := r.client.fragmentQueuedMainPayload(Enums.PACKET_DNS_QUERY_REQ, payload)
	if err != nil {
		return err
	}

	sequenceNum := r.client.nextMainSequence()
	now := time.Now()
	state := &stream0DNSRequestState{
		fragments: make(map[uint8]*stream0DNSFragmentState, len(fragments)),
	}

	for fragmentID, fragmentPayload := range fragments {
		packet := arq.QueuedPacket{
			PacketType:      Enums.PACKET_DNS_QUERY_REQ,
			StreamID:        0,
			SequenceNum:     sequenceNum,
			FragmentID:      uint8(fragmentID),
			TotalFragments:  uint8(len(fragments)),
			CompressionType: r.client.uploadCompression,
			Payload:         fragmentPayload,
			Priority:        arq.DefaultPriorityForPacket(Enums.PACKET_DNS_QUERY_REQ),
		}
		if !r.scheduler.Enqueue(arq.QueueTargetMain, packet) {
			r.mu.Lock()
			delete(r.dnsRequests, sequenceNum)
			r.mu.Unlock()
			return ErrTunnelDNSDispatchFailed
		}
		state.fragments[uint8(fragmentID)] = &stream0DNSFragmentState{
			packet:     packet,
			createdAt:  now,
			retryAt:    now.Add(stream0DNSRetryBaseDelay),
			retryDelay: stream0DNSRetryBaseDelay,
			scheduled:  true,
		}
	}

	r.mu.Lock()
	if !r.running {
		r.mu.Unlock()
		return ErrStream0RuntimeStopped
	}
	r.dnsRequests[sequenceNum] = state
	r.dnsActivitySeen = true
	r.lastDataActivity = now
	r.mu.Unlock()

	r.notifyWake()
	return nil
}

func (r *stream0Runtime) QueuePing() bool {
	if r == nil || !r.IsRunning() {
		return false
	}
	if r.scheduler.PendingPings() > 0 {
		return false
	}

	payload, err := buildClientPingPayload()
	if err != nil {
		return false
	}

	if !r.scheduler.Enqueue(arq.QueueTargetMain, arq.QueuedPacket{
		PacketType: Enums.PACKET_PING,
		Payload:    payload,
		Priority:   arq.DefaultPriorityForPacket(Enums.PACKET_PING),
	}) {
		return false
	}
	r.notifyWake()
	return true
}

func (r *stream0Runtime) QueueStreamPacket(streamID uint16, packetType uint8, sequenceNum uint16, payload []byte) bool {
	if r == nil || !r.IsRunning() || streamID == 0 {
		return false
	}
	if !r.scheduler.Enqueue(arq.QueueTargetStream, arq.QueuedPacket{
		PacketType:  packetType,
		StreamID:    streamID,
		SequenceNum: sequenceNum,
		Payload:     payload,
		Priority:    arq.DefaultPriorityForPacket(packetType),
	}) {
		return false
	}
	r.notifyWake()
	return true
}

func (r *stream0Runtime) txLoop() {
	defer r.wg.Done()
	for {
		select {
		case <-r.ctx.Done():
			r.failAllPending()
			return
		case <-r.wakeCh:
		}

		for {
			result, ok := r.scheduler.Dequeue()
			if !ok {
				break
			}
			r.processDequeue(result.Packet)
		}
	}
}

func (r *stream0Runtime) pingLoop() {
	defer r.wg.Done()
	for {
		select {
		case <-r.ctx.Done():
			return
		default:
		}

		now := time.Now()
		if r.client != nil && r.client.dnsResponses != nil {
			r.client.dnsResponses.Purge(now, r.client.localDNSFragmentTimeout())
		}
		retrySleep := r.queueDueDNSRetries(now)
		shouldPing, pingSleep := r.nextPingSchedule(now)
		if shouldPing {
			if r.QueuePing() {
				r.mu.Lock()
				r.lastPingTime = time.Now()
				r.mu.Unlock()
			}
		}

		sleepFor := minPositiveDuration(retrySleep, pingSleep)
		if sleepFor <= 0 {
			sleepFor = 100 * time.Millisecond
		}

		timer := time.NewTimer(sleepFor)
		select {
		case <-r.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (r *stream0Runtime) nextPingSchedule(now time.Time) (bool, time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	hasPendingDNS := r.client != nil && r.client.hasPendingDNSWork()
	activeStreams := 0
	if r.client != nil {
		activeStreams = r.client.activeStreamCount()
	}
	if activeStreams == 0 {
		if !hasPendingDNS {
			return false, time.Second
		}
		pingInterval := stream0DNSOnlyPingInterval
		maxSleep := stream0PingDNSOnlyMaxSleep
		if now.Sub(r.lastDataActivity) < stream0DNSOnlyWarmDuration {
			pingInterval = stream0DNSOnlyWarmPingInterval
			maxSleep = stream0DNSOnlyWarmMaxSleep
		}
		timeSinceLastPing := now.Sub(r.lastPingTime)
		if timeSinceLastPing >= pingInterval {
			return true, pingInterval
		}
		sleepFor := pingInterval - timeSinceLastPing
		if sleepFor > maxSleep {
			sleepFor = maxSleep
		}
		return false, sleepFor
	}

	if !r.dnsActivitySeen {
		return false, time.Second
	}

	idleTime := now.Sub(r.lastDataActivity)
	pingInterval := stream0PingBusyInterval
	maxSleep := stream0PingBusyMaxSleep
	if idleTime >= stream0PingIdleHighThreshold {
		pingInterval = stream0PingHighIdleInterval
		maxSleep = stream0PingHighIdleMaxSleep
	} else if idleTime >= stream0PingIdleMediumThreshold {
		pingInterval = stream0PingMediumIdleInterval
		maxSleep = stream0PingMediumIdleMaxSleep
	}

	timeSinceLastPing := now.Sub(r.lastPingTime)
	if timeSinceLastPing >= pingInterval {
		return true, pingInterval
	}

	sleepFor := pingInterval - timeSinceLastPing
	if sleepFor > maxSleep {
		sleepFor = maxSleep
	}
	return false, sleepFor
}

func (r *stream0Runtime) processDequeue(packet arq.QueuedPacket) {
	response, err := r.client.sendScheduledPacket(packet)
	if err != nil {
		r.handleDequeueFailure(packet, time.Now())
		return
	}

	dnsRequestAcked := false
	now := time.Now()
	switch response.PacketType {
	case Enums.PACKET_PACKED_CONTROL_BLOCKS:
		r.noteServerDataActivity()
		if err := r.client.handlePackedServerControlBlocks(response.Payload, time.Second); err != nil && r.client.log != nil {
			r.client.log.Debugf(
				"🧵 <yellow>Packed Control Handling Failed</yellow> <magenta>|</magenta> <cyan>%v</cyan>",
				err,
			)
		}
	case Enums.PACKET_DNS_QUERY_REQ_ACK:
		r.noteServerDataActivity()
		dnsRequestAcked = r.ackDNSRequestFragment(response)
	case Enums.PACKET_DNS_QUERY_RES:
		r.noteServerDataActivity()
		if err := r.client.handleInboundDNSResponseFragment(response); err != nil && r.client.log != nil {
			r.client.log.Debugf(
				"\U0001F9E9 <yellow>DNS Response Fragment Handling Failed</yellow> <magenta>|</magenta> <cyan>%v</cyan>",
				err,
			)
		}
	case Enums.PACKET_STREAM_DATA_ACK, Enums.PACKET_STREAM_FIN_ACK, Enums.PACKET_STREAM_RST_ACK:
		r.noteServerDataActivity()
		r.client.noteStreamProgress(response.StreamID)
		if stream, ok := r.client.getStream(response.StreamID); ok {
			ackClientStreamTX(stream, response.SequenceNum, now)
			notifyStreamWake(stream)
		}
	case Enums.PACKET_PONG:
		r.noteServerDataActivity()
	case Enums.PACKET_STREAM_DATA, Enums.PACKET_STREAM_FIN, Enums.PACKET_STREAM_RST:
		r.noteServerDataActivity()
		if err := r.client.handleFollowUpServerPacket(response, time.Second); err != nil && r.client.log != nil {
			r.client.log.Debugf(
				"🧵 <yellow>Stream Runtime Packet Handling Failed</yellow> <magenta>|</magenta> <cyan>%v</cyan>",
				err,
			)
		}
	case 0:
	default:
		if response.PacketType != 0 {
			r.noteServerDataActivity()
			if err := r.client.handleAsyncServerPacket(response, time.Second); err != nil && !errors.Is(err, ErrSessionDropped) && r.client.log != nil {
				r.client.log.Debugf(
					"🧵 <yellow>Main Queue Packet Handling Failed</yellow> <magenta>|</magenta> <cyan>%v</cyan>",
					err,
				)
			}
		}
	}

	if r.client != nil && r.client.reconnectPending.Load() {
		return
	}

	switch {
	case packet.StreamID != 0:
		if !isResolvedStreamPacketResponse(packet, response) {
			r.rescheduleStreamPacket(packet.StreamID, packet.SequenceNum)
		}
	case packet.PacketType == Enums.PACKET_DNS_QUERY_REQ:
		if !dnsRequestAcked {
			r.rescheduleDNSRequestFragment(packet, now)
		}
	}
}

func (r *stream0Runtime) handleDequeueFailure(packet arq.QueuedPacket, now time.Time) {
	if r != nil && r.client != nil && r.client.reconnectPending.Load() {
		return
	}
	switch {
	case packet.StreamID != 0:
		r.rescheduleStreamPacket(packet.StreamID, packet.SequenceNum)
	case packet.PacketType == Enums.PACKET_DNS_QUERY_REQ:
		r.rescheduleDNSRequestFragment(packet, now)
	}
}

func (r *stream0Runtime) ackDNSRequestFragment(packet VpnProto.Packet) bool {
	if packet.PacketType != Enums.PACKET_DNS_QUERY_REQ_ACK || !packet.HasSequenceNum {
		return false
	}
	totalFragments := packet.TotalFragments
	if totalFragments == 0 {
		totalFragments = 1
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	state := r.dnsRequests[packet.SequenceNum]
	if state == nil {
		return false
	}
	fragment := state.fragments[packet.FragmentID]
	if fragment == nil {
		return false
	}
	if fragment.packet.TotalFragments != totalFragments {
		return false
	}
	delete(state.fragments, packet.FragmentID)
	if len(state.fragments) == 0 {
		delete(r.dnsRequests, packet.SequenceNum)
	}
	return true
}

func (r *stream0Runtime) rescheduleDNSRequestFragment(packet arq.QueuedPacket, now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()

	state := r.dnsRequests[packet.SequenceNum]
	if state == nil {
		return
	}
	fragment := state.fragments[packet.FragmentID]
	if fragment == nil {
		return
	}
	if now.Sub(fragment.createdAt) >= r.client.localDNSFragmentTimeout() {
		delete(state.fragments, packet.FragmentID)
		if len(state.fragments) == 0 {
			delete(r.dnsRequests, packet.SequenceNum)
		}
		return
	}
	delay := fragment.retryDelay
	if delay <= 0 {
		delay = stream0DNSRetryBaseDelay
	}
	fragment.scheduled = false
	fragment.retryAt = now.Add(delay)
	fragment.retryCount++
	delay *= 2
	if delay > stream0DNSRetryMaxDelay {
		delay = stream0DNSRetryMaxDelay
	}
	fragment.retryDelay = delay
}

func (r *stream0Runtime) queueDueDNSRetries(now time.Time) time.Duration {
	if r == nil || r.client == nil {
		return time.Second
	}

	timeout := r.client.localDNSFragmentTimeout()
	nextWait := time.Second
	due := make([]arq.QueuedPacket, 0, 4)

	r.mu.Lock()
	for sequenceNum, state := range r.dnsRequests {
		if state == nil || len(state.fragments) == 0 {
			delete(r.dnsRequests, sequenceNum)
			continue
		}
		for fragmentID, fragment := range state.fragments {
			if fragment == nil {
				delete(state.fragments, fragmentID)
				continue
			}
			if now.Sub(fragment.createdAt) >= timeout {
				delete(state.fragments, fragmentID)
				continue
			}
			if fragment.scheduled {
				continue
			}
			if !fragment.retryAt.After(now) {
				due = append(due, fragment.packet)
				fragment.scheduled = true
				continue
			}
			nextWait = minPositiveDuration(nextWait, fragment.retryAt.Sub(now))
		}
		if len(state.fragments) == 0 {
			delete(r.dnsRequests, sequenceNum)
		}
	}
	r.mu.Unlock()

	if len(due) == 0 {
		return nextWait
	}

	for _, packet := range due {
		if r.scheduler.Enqueue(arq.QueueTargetMain, packet) {
			continue
		}
		r.mu.Lock()
		state := r.dnsRequests[packet.SequenceNum]
		if state != nil {
			if fragment := state.fragments[packet.FragmentID]; fragment != nil {
				fragment.scheduled = false
				fragment.retryAt = now.Add(100 * time.Millisecond)
			}
		}
		r.mu.Unlock()
	}
	r.notifyWake()
	return 50 * time.Millisecond
}

func (r *stream0Runtime) noteServerDataActivity() {
	r.mu.Lock()
	r.lastDataActivity = time.Now()
	r.mu.Unlock()
}

func (r *stream0Runtime) notifyWake() {
	select {
	case r.wakeCh <- struct{}{}:
	default:
	}
}

func (r *stream0Runtime) failAllPending() {
	r.mu.Lock()
	r.dnsRequests = make(map[uint16]*stream0DNSRequestState, 4)
	r.running = false
	r.mu.Unlock()
}

func (r *stream0Runtime) ResetForReconnect() {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.dnsRequests = make(map[uint16]*stream0DNSRequestState, 4)
	r.dnsActivitySeen = false
	now := time.Now()
	r.lastDataActivity = now
	r.lastPingTime = now
	r.mu.Unlock()
	if r.scheduler != nil {
		r.scheduler.HandleSessionReset()
	}
}

func (r *stream0Runtime) rescheduleStreamPacket(streamID uint16, sequenceNum uint16) {
	if r == nil || r.client == nil {
		return
	}
	stream, ok := r.client.getStream(streamID)
	if !ok || stream == nil {
		return
	}
	rescheduleClientStreamTX(stream, sequenceNum)
	notifyStreamWake(stream)
}

func isResolvedStreamPacketResponse(sent arq.QueuedPacket, response VpnProto.Packet) bool {
	switch response.PacketType {
	case Enums.PACKET_STREAM_DATA_ACK:
		return sent.PacketType == Enums.PACKET_STREAM_DATA && response.StreamID == sent.StreamID && response.SequenceNum == sent.SequenceNum
	case Enums.PACKET_STREAM_FIN_ACK:
		return sent.PacketType == Enums.PACKET_STREAM_FIN && response.StreamID == sent.StreamID && response.SequenceNum == sent.SequenceNum
	case Enums.PACKET_STREAM_RST_ACK:
		return sent.PacketType == Enums.PACKET_STREAM_RST && response.StreamID == sent.StreamID && response.SequenceNum == sent.SequenceNum
	case Enums.PACKET_STREAM_SYN_ACK:
		return sent.PacketType == Enums.PACKET_STREAM_SYN && response.StreamID == sent.StreamID && response.SequenceNum == sent.SequenceNum
	case Enums.PACKET_SOCKS5_SYN_ACK:
		return sent.PacketType == Enums.PACKET_SOCKS5_SYN && response.StreamID == sent.StreamID && response.SequenceNum == sent.SequenceNum
	default:
		return false
	}
}

func minPositiveDuration(current time.Duration, candidate time.Duration) time.Duration {
	if candidate <= 0 {
		return current
	}
	if current <= 0 || candidate < current {
		return candidate
	}
	return current
}
