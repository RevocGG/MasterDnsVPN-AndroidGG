// ==============================================================================
// MasterDnsVPN
// Author: MasterkinG32
// Github: https://github.com/masterking32
// Year: 2026
// ==============================================================================

package client

import (
	"context"
	"math"
	"slices"
	"time"

	"masterdnsvpn-go/internal/logger"
)

type resolverHealthEvent struct {
	At      time.Time
	Success bool
}

type resolverHealthState struct {
	Events       []resolverHealthEvent
	SuccessCount int
}

type resolverRecheckState struct {
	FailCount   int
	NextAt      time.Time
	WasValidOne bool
}

type resolverDisabledState struct {
	DisabledAt  time.Time
	NextRetryAt time.Time
	RetryCount  int
	Cause       string
}

type resolverRecheckCandidate struct {
	key             string
	nextAt          time.Time
	failCount       int
	runtimePriority bool
}

func (c *Client) initResolverRecheckMeta() {
	if c == nil {
		return
	}
	now := c.now()
	nextInactive := now.Add(c.recheckInactiveInterval())

	c.resolverHealthMu.Lock()
	defer c.resolverHealthMu.Unlock()

	for _, conn := range c.connections {
		if conn.Key == "" {
			continue
		}
		if _, ok := c.resolverHealth[conn.Key]; !ok {
			c.resolverHealth[conn.Key] = &resolverHealthState{
				Events: make([]resolverHealthEvent, 0, 8),
			}
		}
		meta := c.resolverRecheck[conn.Key]
		if !conn.IsValid && meta.NextAt.IsZero() {
			meta.NextAt = nextInactive
		}
		c.resolverRecheck[conn.Key] = meta
	}
}

func (c *Client) runResolverHealthLoop(ctx context.Context) {
	for {
		if ctx != nil {
			select {
			case <-ctx.Done():
				c.resolverHealthMu.Lock()
				c.healthRuntimeRun = false
				c.resolverHealthMu.Unlock()
				return
			default:
			}
		}

		now := c.now()
		c.runResolverAutoDisable(now)
		c.runResolverRecheckBatch(ctx, now)

		waitFor := c.nextResolverHealthWait(now)
		timer := time.NewTimer(waitFor)
		if ctx == nil {
			<-timer.C
			continue
		}
		select {
		case <-ctx.Done():
			timer.Stop()
			c.resolverHealthMu.Lock()
			c.healthRuntimeRun = false
			c.resolverHealthMu.Unlock()
			return
		case <-timer.C:
		}
	}
}

func (c *Client) resolverHealthDebugEnabled() bool {
	return c != nil && c.log != nil && c.log.Enabled(logger.LevelDebug)
}

func (c *Client) nextResolverHealthWait(now time.Time) time.Duration {
	waitFor := 2 * time.Second
	if c == nil {
		return waitFor
	}
	if c.cfg.AutoDisableTimeoutServers {
		waitFor = minDuration(waitFor, c.autoDisableCheckInterval())
	}
	if !c.cfg.RecheckInactiveEnabled || !c.successMTUChecks {
		return clampResolverHealthWait(waitFor)
	}

	c.resolverHealthMu.Lock()
	defer c.resolverHealthMu.Unlock()

	for key, meta := range c.resolverRecheck {
		conn := c.connectionPtrByKey(key)
		if conn == nil || conn.IsValid || meta.NextAt.IsZero() {
			continue
		}
		dueIn := time.Until(meta.NextAt)
		if !meta.NextAt.After(now) {
			dueIn = 250 * time.Millisecond
		}
		waitFor = minDuration(waitFor, dueIn)
	}
	return clampResolverHealthWait(waitFor)
}

func clampResolverHealthWait(waitFor time.Duration) time.Duration {
	if waitFor < 250*time.Millisecond {
		return 250 * time.Millisecond
	}
	if waitFor > 5*time.Second {
		return 5 * time.Second
	}
	return waitFor
}

func (c *Client) noteResolverSend(serverKey string) {
	if c == nil || serverKey == "" {
		return
	}
	c.balancer.ReportSend(serverKey)
}

func (c *Client) noteResolverSuccess(serverKey string, rtt time.Duration) {
	if c == nil || serverKey == "" {
		return
	}
	c.balancer.ReportSuccess(serverKey, rtt)
	c.recordResolverHealthEvent(serverKey, true, c.now())
}

func (c *Client) noteResolverTimeout(serverKey string) {
	if c == nil || serverKey == "" {
		return
	}
	c.recordResolverHealthEvent(serverKey, false, c.now())
}

func (c *Client) recordResolverHealthEvent(serverKey string, success bool, now time.Time) {
	if c == nil || serverKey == "" {
		return
	}

	c.resolverHealthMu.Lock()
	defer c.resolverHealthMu.Unlock()

	state := c.resolverHealth[serverKey]
	if state == nil {
		state = &resolverHealthState{Events: make([]resolverHealthEvent, 0, 8)}
		c.resolverHealth[serverKey] = state
	}
	state.Events = append(state.Events, resolverHealthEvent{At: now, Success: success})
	if success {
		state.SuccessCount++
	}
	c.pruneResolverHealthLocked(state, now)
}

func (c *Client) pruneResolverHealthLocked(state *resolverHealthState, now time.Time) {
	if state == nil || len(state.Events) == 0 {
		return
	}
	cutoff := now.Add(-c.autoDisableTimeoutWindow())
	dropCount := 0
	droppedSuccess := 0
	for dropCount < len(state.Events) && state.Events[dropCount].At.Before(cutoff) {
		if state.Events[dropCount].Success {
			droppedSuccess++
		}
		dropCount++
	}
	if dropCount == 0 {
		return
	}
	state.SuccessCount -= droppedSuccess
	if state.SuccessCount < 0 {
		state.SuccessCount = 0
	}
	state.Events = append(state.Events[:0], state.Events[dropCount:]...)
}

func (c *Client) runResolverAutoDisable(now time.Time) {
	if c == nil || !c.cfg.AutoDisableTimeoutServers || c.balancer.ValidCount() <= 1 {
		return
	}

	candidates := make([]string, 0, len(c.connections))
	c.resolverHealthMu.Lock()
	for key, state := range c.resolverHealth {
		if state == nil {
			continue
		}
		c.pruneResolverHealthLocked(state, now)
		if len(state.Events) < c.autoDisableMinObservations() {
			continue
		}
		if state.SuccessCount != 0 {
			continue
		}
		if state.Events[len(state.Events)-1].At.Sub(state.Events[0].At) < c.autoDisableTimeoutWindow() {
			continue
		}
		candidates = append(candidates, key)
	}
	c.resolverHealthMu.Unlock()

	for _, key := range candidates {
		if c.balancer.ValidCount() <= 1 {
			return
		}
		c.disableResolverConnection(key, "100% timeout window")
	}
}

func (c *Client) disableResolverConnection(serverKey string, cause string) bool {
	if c == nil || serverKey == "" {
		return false
	}
	conn := c.connectionPtrByKey(serverKey)
	if conn == nil || !conn.IsValid || c.balancer.ValidCount() <= 1 {
		return false
	}
	if !c.SetConnectionValidity(serverKey, false) {
		return false
	}

	now := c.now()
	nextAt := now.Add(maxDuration(5*time.Second, c.recheckServerInterval()*2))

	c.resolverHealthMu.Lock()
	meta := c.resolverRecheck[serverKey]
	meta.WasValidOne = true
	meta.NextAt = nextAt
	c.resolverRecheck[serverKey] = meta
	c.runtimeDisabled[serverKey] = resolverDisabledState{
		DisabledAt:  now,
		NextRetryAt: nextAt,
		RetryCount:  meta.FailCount,
		Cause:       cause,
	}
	delete(c.resolverHealth, serverKey)
	c.resolverHealthMu.Unlock()

	c.balancer.ResetServerStats(serverKey)
	if c.log != nil {
		c.log.Warnf(
			"\U0001F6D1 <yellow>DNS server <cyan>%s</cyan> disabled due to: <red>%s</red></yellow>",
			conn.ResolverLabel,
			cause,
		)
	}
	c.appendMTURemovedServerLine(conn, cause)
	return true
}

func (c *Client) reactivateResolverConnection(serverKey string) bool {
	if c == nil || serverKey == "" {
		return false
	}
	conn := c.connectionPtrByKey(serverKey)
	if conn == nil || conn.IsValid {
		return false
	}
	if !c.SetConnectionValidity(serverKey, true) {
		return false
	}

	c.resolverHealthMu.Lock()
	delete(c.runtimeDisabled, serverKey)
	delete(c.resolverHealth, serverKey)
	c.resolverRecheck[serverKey] = resolverRecheckState{WasValidOne: true}
	c.resolverHealthMu.Unlock()

	c.balancer.ResetServerStats(serverKey)
	if c.log != nil {
		c.log.Infof(
			"\U0001F504 <green>DNS server <cyan>%s</cyan> re-activated after successful recheck.</green>",
			conn.ResolverLabel,
		)
	}
	c.appendMTUAddedServerLine(conn)
	return true
}

func (c *Client) scheduleResolverRecheckFailure(serverKey string, runtimePriority bool, now time.Time) {
	if c == nil || serverKey == "" {
		return
	}

	c.resolverHealthMu.Lock()
	defer c.resolverHealthMu.Unlock()

	meta := c.resolverRecheck[serverKey]
	meta.FailCount++

	var base time.Duration
	if runtimePriority {
		base = maxDuration(10*time.Second, c.recheckServerInterval()*2)
	} else {
		base = maxDuration(30*time.Second, c.recheckInactiveInterval()/4)
	}

	pow := math.Pow(1.8, float64(min(meta.FailCount, 6)))
	delay := time.Duration(float64(base) * pow)
	if delay > c.recheckInactiveInterval() {
		delay = c.recheckInactiveInterval()
	}
	delay += deterministicResolverJitter(serverKey, delay)
	meta.NextAt = now.Add(delay)
	c.resolverRecheck[serverKey] = meta

	if state, ok := c.runtimeDisabled[serverKey]; ok {
		state.NextRetryAt = meta.NextAt
		state.RetryCount = meta.FailCount
		c.runtimeDisabled[serverKey] = state
	}
}

func deterministicResolverJitter(serverKey string, delay time.Duration) time.Duration {
	if serverKey == "" || delay <= 0 {
		return 0
	}
	maxJitter := minDuration(2*time.Second, time.Duration(float64(delay)*0.15))
	if maxJitter <= 0 {
		return 0
	}
	var hash uint64 = 1469598103934665603
	for i := 0; i < len(serverKey); i++ {
		hash ^= uint64(serverKey[i])
		hash *= 1099511628211
	}
	return time.Duration(hash % uint64(maxJitter))
}

func (c *Client) runResolverRecheckBatch(ctx context.Context, now time.Time) {
	if c == nil || !c.cfg.RecheckInactiveEnabled || !c.successMTUChecks {
		return
	}

	candidates := c.collectDueResolverRechecks(now)
	if len(candidates) == 0 {
		return
	}

	limit := c.recheckBatchSize()
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}

	for _, candidate := range candidates {
		conn := c.connectionPtrByKey(candidate.key)
		if conn == nil || conn.IsValid {
			continue
		}

		if c.resolverHealthDebugEnabled() {
			c.log.Debugf(
				"\U0001F50D <green>Rechecking inactive resolver: <cyan>%s</cyan> (Priority: <cyan>%t</cyan>, Failures: <cyan>%d</cyan>)</green>",
				conn.ResolverLabel,
				candidate.runtimePriority,
				candidate.failCount,
			)
		}

		if c.recheckResolverConnection(ctx, conn) {
			c.reactivateResolverConnection(candidate.key)
			continue
		}

		c.scheduleResolverRecheckFailure(candidate.key, candidate.runtimePriority, now)
		if c.resolverHealthDebugEnabled() {
			c.resolverHealthMu.Lock()
			nextAt := c.resolverRecheck[candidate.key].NextAt
			failCount := c.resolverRecheck[candidate.key].FailCount
			c.resolverHealthMu.Unlock()
			c.log.Debugf(
				"\u23ED\uFE0F <yellow>Inactive resolver recheck failed: <cyan>%s</cyan> (Failures: <cyan>%d</cyan>, Next Retry: <cyan>%s</cyan>)</yellow>",
				conn.ResolverLabel,
				failCount,
				maxDuration(0, time.Until(nextAt)).Round(time.Second),
			)
		}
	}
}

func (c *Client) collectDueResolverRechecks(now time.Time) []resolverRecheckCandidate {
	if c == nil {
		return nil
	}

	c.resolverHealthMu.Lock()
	defer c.resolverHealthMu.Unlock()

	runtimeCandidates := make([]resolverRecheckCandidate, 0, len(c.runtimeDisabled))
	normalCandidates := make([]resolverRecheckCandidate, 0, len(c.connections))
	for key, meta := range c.resolverRecheck {
		conn := c.connectionPtrByKey(key)
		if conn == nil || conn.IsValid {
			continue
		}
		if !meta.NextAt.IsZero() && meta.NextAt.After(now) {
			continue
		}

		candidateValue := resolverRecheckCandidate{
			key:       key,
			nextAt:    meta.NextAt,
			failCount: meta.FailCount,
		}
		if state, ok := c.runtimeDisabled[key]; ok {
			candidateValue.runtimePriority = true
			candidateValue.nextAt = state.NextRetryAt
			candidateValue.failCount = state.RetryCount
			runtimeCandidates = append(runtimeCandidates, candidateValue)
			continue
		}
		normalCandidates = append(normalCandidates, candidateValue)
	}

	slices.SortFunc(runtimeCandidates, func(a, b resolverRecheckCandidate) int {
		if cmp := a.nextAt.Compare(b.nextAt); cmp != 0 {
			return cmp
		}
		if a.failCount < b.failCount {
			return -1
		}
		if a.failCount > b.failCount {
			return 1
		}
		if a.key < b.key {
			return -1
		}
		if a.key > b.key {
			return 1
		}
		return 0
	})
	slices.SortFunc(normalCandidates, func(a, b resolverRecheckCandidate) int {
		if a.failCount < b.failCount {
			return -1
		}
		if a.failCount > b.failCount {
			return 1
		}
		if cmp := a.nextAt.Compare(b.nextAt); cmp != 0 {
			return cmp
		}
		if a.key < b.key {
			return -1
		}
		if a.key > b.key {
			return 1
		}
		return 0
	})

	candidates := make([]resolverRecheckCandidate, 0, len(runtimeCandidates)+len(normalCandidates))
	candidates = append(candidates, runtimeCandidates...)
	candidates = append(candidates, normalCandidates...)
	return candidates
}

func (c *Client) recheckResolverConnection(ctx context.Context, conn *Connection) bool {
	if c == nil || conn == nil || c.syncedUploadMTU <= 0 || c.syncedDownloadMTU <= 0 {
		if conn != nil && c.resolverHealthDebugEnabled() {
			c.log.Debugf(
				"Cannot recheck connection <cyan>%s</cyan> because synced MTU values are not available.",
				conn.ResolverLabel,
			)
		}
		return false
	}
	if c.recheckConnectionFn != nil {
		return c.recheckConnectionFn(conn)
	}

	transport, err := newUDPQueryTransport(conn.ResolverLabel)
	if err != nil {
		return false
	}
	defer transport.conn.Close()

	upOK, err := c.sendUploadMTUProbe(ctx, conn, transport, c.syncedUploadMTU, mtuProbeOptions{Quiet: true})
	if err != nil || !upOK {
		return false
	}
	downOK, err := c.sendDownloadMTUProbe(ctx, conn, transport, c.syncedDownloadMTU, c.syncedUploadMTU, mtuProbeOptions{Quiet: true})
	if err != nil || !downOK {
		return false
	}

	conn.UploadMTUBytes = c.syncedUploadMTU
	conn.UploadMTUChars = c.encodedCharsForPayload(c.syncedUploadMTU)
	conn.DownloadMTUBytes = c.syncedDownloadMTU
	return true
}

func (c *Client) isRuntimeDisabledResolver(serverKey string) bool {
	if c == nil || serverKey == "" {
		return false
	}
	c.resolverHealthMu.Lock()
	_, ok := c.runtimeDisabled[serverKey]
	c.resolverHealthMu.Unlock()
	return ok
}

func (c *Client) autoDisableTimeoutWindow() time.Duration {
	return time.Duration(c.cfg.AutoDisableTimeoutWindow * float64(time.Second))
}

func (c *Client) autoDisableCheckInterval() time.Duration {
	return time.Duration(c.cfg.AutoDisableCheckInterval * float64(time.Second))
}

func (c *Client) autoDisableMinObservations() int {
	if c.cfg.AutoDisableMinObservations < 1 {
		return 1
	}
	return c.cfg.AutoDisableMinObservations
}

func (c *Client) recheckInactiveInterval() time.Duration {
	return time.Duration(c.cfg.RecheckInactiveInterval * float64(time.Second))
}

func (c *Client) recheckServerInterval() time.Duration {
	return time.Duration(c.cfg.RecheckServerInterval * float64(time.Second))
}

func (c *Client) recheckBatchSize() int {
	if c.cfg.RecheckBatchSize < 1 {
		return 1
	}
	return c.cfg.RecheckBatchSize
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}
