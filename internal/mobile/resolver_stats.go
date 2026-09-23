// MasterDnsVPN  Android Mobile Adapter
// File: internal/mobile/resolver_stats.go
//
// Android-side glue for the app's "Best Match" resolver list feature.
// Reads per-resolver quality counters from the running engine's balancer and
// persists the best-performing resolvers to <profileDir>/best_match_resolvers.txt
// so the app can offer an always-fresh, high-quality resolver list.
//
// Quality metric (same counters the core balancer uses for its strategies):
//   score = ackRate^2 × (1 / (1 + avgRTT/250ms))
//   - ackRate  = acked/sent with half-life decay in the core (recent wins)
//   - squared so a resolver dropping >20% of packets falls off fast
//   - RTT factor: 250ms reference — a 100ms resolver keeps ~0.71, a 1s one ~0.2
//   - resolvers with <8 samples are excluded (not enough evidence yet)
//   - a resolver that has been marked invalid by the core is excluded

package mobile

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// bestMatchMinSamples is the minimum number of observed packets before a
	// resolver is trusted for the Best Match list. Below this the evidence is
	// too weak (a resolver that answered 2/2 pings is not proven good).
	bestMatchMinSamples = 8

	// bestMatchReferenceRTTms anchors the RTT factor of the score.
	bestMatchReferenceRTTms = 250.0

	// bestMatchKeepCount caps the persisted list length.
	bestMatchKeepCount = 100

	// bestMatchUpdateInterval prevents hot-looping the file write.
	bestMatchUpdateInterval = 30 * time.Second
)

type MobileResolverQuality struct {
	Resolver string
	Port     int
	IsValid  bool
	Sent     uint64
	Acked    uint64
	Lost     uint64
	AvgRTTms float64
}

// GetResolverQualitySnapshot returns the per-resolver quality snapshot for a
// running instance (empty when the instance is not running).
func GetResolverQualitySnapshot(instanceID string) []MobileResolverQuality {
	instancesMu.Lock()
	h, ok := instances[instanceID]
	instancesMu.Unlock()
	if !ok || h == nil || h.cl == nil {
		return nil
	}
	raw := h.cl.Balancer().GetResolverQuality()
	out := make([]MobileResolverQuality, 0, len(raw))
	for _, q := range raw {
		out = append(out, MobileResolverQuality{
			Resolver: q.Resolver,
			Port:     q.Port,
			IsValid:  q.IsValid,
			Sent:     q.Sent,
			Acked:    q.Acked,
			Lost:     q.Lost,
			AvgRTTms: q.AvgRTTms,
		})
	}
	return out
}

// ScoreResolverQuality computes the Best Match score of one sample.
// Higher is better; -1 means "not eligible" (too few samples / invalid).
func ScoreResolverQuality(q MobileResolverQuality) float64 {
	if !q.IsValid || q.Sent < bestMatchMinSamples {
		return -1
	}
	ackRate := float64(q.Acked) / float64(q.Sent)
	if ackRate <= 0 {
		return -1
	}
	rttFactor := 1.0
	if q.AvgRTTms > 0 {
		rttFactor = bestMatchReferenceRTTms / (bestMatchReferenceRTTms + q.AvgRTTms)
	}
	return ackRate * ackRate * rttFactor
}

var bestMatchMu sync.Mutex
var bestMatchLastWrite = map[string]time.Time{}

// scoredResolver is one ranked Best Match row (0..1 score).
type scoredResolver struct {
	addr  string
	score float64
}

// UpdateBestMatchList evaluates the live resolver quality of a running
// instance and (throttled) rewrites the Best Match resolver file inside the
// profile directory. Called by the app while a profile is running.
func UpdateBestMatchList(instanceID string) (int, error) {
	instancesMu.Lock()
	h, ok := instances[instanceID]
	instancesMu.Unlock()
	if !ok || h == nil || h.cl == nil || h.profileDir == "" {
		return 0, fmt.Errorf("instance not running")
	}

	bestMatchMu.Lock()
	if last, seen := bestMatchLastWrite[instanceID]; seen && time.Since(last) < bestMatchUpdateInterval {
		bestMatchMu.Unlock()
		return -1, nil // throttled — not an error
	}
	bestMatchMu.Unlock()

	quality := GetResolverQualitySnapshot(instanceID)
	type scored struct {
		line  string
		score float64
	}
	cands := make([]scored, 0, len(quality))
	for _, q := range quality {
		if s := ScoreResolverQuality(q); s >= 0 {
			addr := q.Resolver
			if q.Port > 0 {
				addr = fmt.Sprintf("%s:%d", q.Resolver, q.Port)
			}
			cands = append(cands, scored{line: addr, score: s})
		}
	}
	if len(cands) == 0 {
		return 0, nil
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].score > cands[j].score })
	if len(cands) > bestMatchKeepCount {
		cands = cands[:bestMatchKeepCount]
	}

	var b strings.Builder
	b.WriteString("# Best Match resolvers — auto-updated from live quality stats\n")
	b.WriteString(fmt.Sprintf("# updated: %s | metric: ackRate^2 x rttFactor(250ms), min %d samples\n",
		time.Now().UTC().Format(time.RFC3339), bestMatchMinSamples))
	for _, c := range cands {
		b.WriteString(c.line)
		b.WriteByte('\n')
	}

	path := filepath.Join(h.profileDir, "best_match_resolvers.txt")
	if err := os.WriteFile(path, []byte(b.String()), 0o640); err != nil {
		return 0, err
	}

	bestMatchMu.Lock()
	bestMatchLastWrite[instanceID] = time.Now()
	bestMatchMu.Unlock()
	return len(cands), nil
}

// ReadBestMatchList returns the persisted Best Match resolver lines for a
// profile directory (nil when the file does not exist yet).
func ReadBestMatchList(profileDir string) []string {
	data, err := os.ReadFile(filepath.Join(profileDir, "best_match_resolvers.txt"))
	if err != nil {
		return nil
	}
	var lines []string
	for _, ln := range strings.Split(string(data), "\n") {
		ln = strings.TrimSpace(ln)
		if ln != "" && !strings.HasPrefix(ln, "#") {
			lines = append(lines, ln)
		}
	}
	return lines
}
