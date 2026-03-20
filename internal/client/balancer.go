// ==============================================================================
// MasterDnsVPN
// Author: MasterkinG32
// Github: https://github.com/masterking32
// Year: 2026
// ==============================================================================

package client

import (
	"sync"
	"sync/atomic"
	"time"
)

const (
	BalancingRoundRobinDefault = 0
	BalancingRandom            = 1
	BalancingRoundRobin        = 2
	BalancingLeastLoss         = 3
	BalancingLowestLatency     = 4
)

type Balancer struct {
	strategy  int
	rrCounter atomic.Uint64
	rngState  atomic.Uint64

	mu       sync.Mutex
	snapshot atomic.Pointer[balancerSnapshot]
}

type connectionStats struct {
	sent         atomic.Uint64
	acked        atomic.Uint64
	rttMicrosSum atomic.Uint64
	rttCount     atomic.Uint64
}

type balancerSnapshot struct {
	connections []*Connection
	valid       []int
	indexByKey  map[string]int
	statsByKey  map[string]*connectionStats
}

func NewBalancer(strategy int) *Balancer {
	b := &Balancer{strategy: strategy}
	b.rngState.Store(seedRNG())
	return b
}

func (b *Balancer) SetConnections(connections []*Connection) {
	b.mu.Lock()
	defer b.mu.Unlock()

	indexByKey := make(map[string]int, len(connections))
	statsByKey := make(map[string]*connectionStats, len(connections))
	valid := make([]int, 0, len(connections))

	for idx, conn := range connections {
		if conn == nil {
			continue
		}
		indexByKey[conn.Key] = idx
		statsByKey[conn.Key] = &connectionStats{}
		if conn.IsValid {
			valid = append(valid, idx)
		}
	}

	b.snapshot.Store(&balancerSnapshot{
		connections: connections,
		valid:       valid,
		indexByKey:  indexByKey,
		statsByKey:  statsByKey,
	})
}

func (b *Balancer) ValidCount() int {
	snap := b.snapshot.Load()
	if snap == nil {
		return 0
	}
	return len(snap.valid)
}

func (b *Balancer) SetConnectionValidity(key string, valid bool) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	snap := b.snapshot.Load()
	if snap == nil {
		return false
	}

	idx, ok := snap.indexByKey[key]
	if !ok {
		return false
	}

	conn := snap.connections[idx]
	if conn == nil || conn.IsValid == valid {
		return ok
	}

	conn.IsValid = valid
	b.snapshot.Store(&balancerSnapshot{
		connections: snap.connections,
		valid:       rebuildValidIndices(snap.connections),
		indexByKey:  snap.indexByKey,
		statsByKey:  snap.statsByKey,
	})
	return true
}

func (b *Balancer) RefreshValidConnections() {
	b.mu.Lock()
	defer b.mu.Unlock()

	snap := b.snapshot.Load()
	if snap == nil {
		return
	}

	b.snapshot.Store(&balancerSnapshot{
		connections: snap.connections,
		valid:       rebuildValidIndices(snap.connections),
		indexByKey:  snap.indexByKey,
		statsByKey:  snap.statsByKey,
	})
}

func (b *Balancer) ReportSend(serverKey string) {
	if stats := b.statsForKey(serverKey); stats != nil {
		stats.sent.Add(1)
	}
}

func (b *Balancer) ReportSuccess(serverKey string, rtt time.Duration) {
	stats := b.statsForKey(serverKey)
	if stats == nil {
		return
	}

	stats.acked.Add(1)
	if rtt > 0 {
		stats.rttMicrosSum.Add(uint64(rtt / time.Microsecond))
		stats.rttCount.Add(1)
	}

	sent := stats.sent.Load()
	if sent <= 1000 {
		return
	}

	stats.sent.Store(sent / 2)
	stats.acked.Store(stats.acked.Load() / 2)
	stats.rttMicrosSum.Store(stats.rttMicrosSum.Load() / 2)
	stats.rttCount.Store(stats.rttCount.Load() / 2)
}

func (b *Balancer) ResetServerStats(serverKey string) {
	stats := b.statsForKey(serverKey)
	if stats == nil {
		return
	}

	stats.sent.Store(0)
	stats.acked.Store(0)
	stats.rttMicrosSum.Store(0)
	stats.rttCount.Store(0)
}

func (b *Balancer) GetBestConnection() (Connection, bool) {
	snap := b.snapshot.Load()
	if snap == nil || len(snap.valid) == 0 {
		return Connection{}, false
	}

	switch b.strategy {
	case BalancingRandom:
		idx := snap.valid[b.nextRandom()%uint64(len(snap.valid))]
		return derefConnection(snap.connections, idx)
	case BalancingLeastLoss:
		return b.bestScoredConnection(snap, b.lossScore)
	case BalancingLowestLatency:
		return b.bestScoredConnection(snap, b.latencyScore)
	default:
		pos := int(b.rrCounter.Add(1)-1) % len(snap.valid)
		return derefConnection(snap.connections, snap.valid[pos])
	}
}

func (b *Balancer) GetUniqueConnections(requiredCount int) []Connection {
	snap := b.snapshot.Load()
	if snap == nil {
		return nil
	}

	count := normalizeRequiredCount(len(snap.valid), requiredCount, 1)
	if count == 0 {
		return nil
	}
	if count == 1 {
		best, ok := b.GetBestConnection()
		if !ok {
			return nil
		}
		return []Connection{best}
	}

	switch b.strategy {
	case BalancingRandom:
		return b.selectRandom(snap, count)
	case BalancingLeastLoss:
		return b.selectLowestScore(snap, count, b.lossScore)
	case BalancingLowestLatency:
		return b.selectLowestScore(snap, count, b.latencyScore)
	default:
		return b.selectRoundRobin(snap, count)
	}
}

func (b *Balancer) GetAllValidConnections() []Connection {
	snap := b.snapshot.Load()
	if snap == nil || len(snap.valid) == 0 {
		return nil
	}
	return snapshotConnections(snap.connections, snap.valid)
}

func rebuildValidIndices(connections []*Connection) []int {
	valid := make([]int, 0, len(connections))
	for idx, conn := range connections {
		if conn != nil && conn.IsValid {
			valid = append(valid, idx)
		}
	}
	return valid
}

func (b *Balancer) statsForKey(serverKey string) *connectionStats {
	snap := b.snapshot.Load()
	if snap == nil {
		return nil
	}
	return snap.statsByKey[serverKey]
}

func normalizeRequiredCount(validCount, requiredCount, defaultIfInvalid int) int {
	if validCount <= 0 {
		return 0
	}
	if requiredCount <= 0 {
		requiredCount = defaultIfInvalid
	}
	if requiredCount > validCount {
		return validCount
	}
	return requiredCount
}

func (b *Balancer) selectRoundRobin(snap *balancerSnapshot, count int) []Connection {
	start := int(b.rrCounter.Add(uint64(count)) - uint64(count))
	selected := make([]Connection, 0, count)
	for i := range count {
		conn, ok := derefConnection(snap.connections, snap.valid[(start+i)%len(snap.valid)])
		if ok {
			selected = append(selected, conn)
		}
	}
	return selected
}

func (b *Balancer) selectRandom(snap *balancerSnapshot, count int) []Connection {
	order := make([]int, len(snap.valid))
	copy(order, snap.valid)

	for i := range count {
		j := i + int(b.nextRandom()%uint64(len(order)-i))
		order[i], order[j] = order[j], order[i]
	}

	return snapshotConnections(snap.connections, order[:count])
}

func (b *Balancer) selectLowestScore(snap *balancerSnapshot, count int, scorer func(*balancerSnapshot, int) float64) []Connection {
	bestIdx := make([]int, 0, count)
	bestScores := make([]float64, 0, count)

	for _, idx := range snap.valid {
		score := scorer(snap, idx)
		insertPos := len(bestScores)

		for i := 0; i < len(bestScores); i++ {
			if score < bestScores[i] {
				insertPos = i
				break
			}
		}

		if len(bestScores) < count {
			bestScores = append(bestScores, 0)
			bestIdx = append(bestIdx, 0)
		} else if insertPos == len(bestScores) {
			continue
		}

		copy(bestScores[insertPos+1:], bestScores[insertPos:])
		copy(bestIdx[insertPos+1:], bestIdx[insertPos:])
		bestScores[insertPos] = score
		bestIdx[insertPos] = idx

		if len(bestScores) > count {
			bestScores = bestScores[:count]
			bestIdx = bestIdx[:count]
		}
	}

	return snapshotConnections(snap.connections, bestIdx)
}

func snapshotConnections(connections []*Connection, indices []int) []Connection {
	selected := make([]Connection, 0, len(indices))
	for _, idx := range indices {
		if idx < 0 || idx >= len(connections) || connections[idx] == nil {
			continue
		}
		selected = append(selected, *connections[idx])
	}
	return selected
}

func (b *Balancer) bestScoredConnection(snap *balancerSnapshot, scorer func(*balancerSnapshot, int) float64) (Connection, bool) {
	bestIndex := -1
	bestScore := 0.0
	for _, idx := range snap.valid {
		score := scorer(snap, idx)
		if bestIndex == -1 || score < bestScore {
			bestIndex = idx
			bestScore = score
		}
	}
	if bestIndex < 0 {
		return Connection{}, false
	}
	return derefConnection(snap.connections, bestIndex)
}

func derefConnection(connections []*Connection, idx int) (Connection, bool) {
	if idx < 0 || idx >= len(connections) || connections[idx] == nil {
		return Connection{}, false
	}
	return *connections[idx], true
}

func (b *Balancer) lossScore(snap *balancerSnapshot, idx int) float64 {
	stats := statsByIndex(snap, idx)
	if stats == nil {
		return 0.5
	}
	sent := stats.sent.Load()
	if sent < 5 {
		return 0.5
	}

	acked := stats.acked.Load()
	loss := 1.0 - (float64(acked) / float64(sent))
	if loss < 0 {
		return 0
	}
	if loss > 1 {
		return 1
	}
	return loss
}

func (b *Balancer) latencyScore(snap *balancerSnapshot, idx int) float64 {
	stats := statsByIndex(snap, idx)
	if stats == nil {
		return 999000.0
	}
	count := stats.rttCount.Load()
	if count < 5 {
		return 999000.0
	}
	return float64(stats.rttMicrosSum.Load()) / float64(count)
}

func statsByIndex(snap *balancerSnapshot, idx int) *connectionStats {
	if snap == nil || idx < 0 || idx >= len(snap.connections) || snap.connections[idx] == nil {
		return nil
	}
	return snap.statsByKey[snap.connections[idx].Key]
}

func (b *Balancer) nextRandom() uint64 {
	for {
		current := b.rngState.Load()
		next := xorshift64(current)
		if b.rngState.CompareAndSwap(current, next) {
			return next
		}
	}
}

func seedRNG() uint64 {
	seed := uint64(time.Now().UnixNano())
	if seed == 0 {
		return 0x9e3779b97f4a7c15
	}
	return seed
}

func xorshift64(v uint64) uint64 {
	if v == 0 {
		v = 0x9e3779b97f4a7c15
	}
	v ^= v << 13
	v ^= v >> 7
	v ^= v << 17
	return v
}
