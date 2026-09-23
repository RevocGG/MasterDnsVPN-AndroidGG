// MasterDnsVPN  Android Mobile Adapter
// File: internal/mobile/best_match_aggregate.go
//
// Global "Best Match" aggregation for the Android app.
//
// The per-instance writer in resolver_stats.go writes into each running
// profile's own directory (profiles/<profileId>/best_match_resolvers.txt),
// which is transient — profile dirs are per-profile runtime config, not a
// stable store the Home card can rely on. The Home-screen card needs ONE
// persistent, always-fresh list, so the app layer calls UpdateBestMatchGlobal
// which:
//   - merges the live quality snapshots of ALL running instances,
//   - scores each resolver with the Best Match metric (see resolver_stats.go),
//   - throttled-rewrites the shared file  profiles/_best_match/best_match_resolvers.txt
//   - returns the scored rows (CSV) so the UI and the persisted list always
//     show identical, ranked data.
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

// bestMatchGlobalDir is the stable, profile-independent home of the shared
// Best Match file. Under the app's filesDir, next to profiles/.
const bestMatchGlobalDirName = "_best_match"

// bestMatchGlobalKeepCount caps the persisted global list length.
const bestMatchGlobalKeepCount = 100

// bestMatchPruneFloor keeps at least this many resolvers when pruning so a
// catastrophic network change never empties the list entirely.
const bestMatchPruneFloor = 10

var bestMatchGlobalMu sync.Mutex
var bestMatchGlobalLastWrite time.Time

// aggregateBestMatchSnapshot merges the quality snapshots of every running
// instance and returns scored, sorted candidates (best first).
func aggregateBestMatchSnapshot() []scoredResolver {
	instanceIDs := func() []string {
		instancesMu.Lock()
		defer instancesMu.Unlock()
		ids := make([]string, 0, len(instances))
		for id := range instances {
			ids = append(ids, id)
		}
		return ids
	}()

	merged := map[string]MobileResolverQuality{}
	for _, id := range instanceIDs {
		for _, q := range GetResolverQualitySnapshot(id) {
			addr := q.Resolver
			if q.Port > 0 {
				addr = fmt.Sprintf("%s:%d", q.Resolver, q.Port)
			}
			if prev, seen := merged[addr]; !seen || q.Sent > prev.Sent {
				merged[addr] = q
			}
		}
	}

	cands := make([]scoredResolver, 0, len(merged))
	for _, q := range merged {
		if s := ScoreResolverQuality(q); s >= 0 {
			addr := q.Resolver
			if q.Port > 0 {
				addr = fmt.Sprintf("%s:%d", q.Resolver, q.Port)
			}
			cands = append(cands, scoredResolver{addr: addr, score: s})
		}
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].score > cands[j].score })
	if len(cands) > bestMatchGlobalKeepCount {
		cands = cands[:bestMatchGlobalKeepCount]
	}
	return cands
}

// UpdateBestMatchGlobal evaluates all running instances, merges the result
// with the previously persisted list (so the card never flashes empty between
// runs), PRUNES resolvers that keep failing in live traffic and writes the
// shared Best Match file (throttled to bestMatchUpdateInterval). Returns the
// scored snapshot as CSV lines "addr,score%,rttMs" (empty when the engine has
// no data at all — e.g. nothing running yet).
func UpdateBestMatchGlobal(filesDir string) string {
	cands := aggregateBestMatchSnapshot()

	// Throttle only the file write — the scored CSV is cheap to always return.
	shouldWrite := func() bool {
		bestMatchGlobalMu.Lock()
		defer bestMatchGlobalMu.Unlock()
		if time.Since(bestMatchGlobalLastWrite) < bestMatchUpdateInterval {
			return false
		}
		bestMatchGlobalLastWrite = time.Now()
		return true
	}()

	if shouldWrite && filesDir != "" {
		var entries []scoredResolver
		if len(cands) > 0 {
			entries = cands
		} else {
			// Nothing eligible live (e.g. scan just started): keep the previous
			// list instead of wiping it.
			entries = readBestMatchEntries(filesDir)
		}
		if len(entries) > 0 {
			writeBestMatchFile(filesDir, entries)
		}
	}

	var out strings.Builder
	for i, c := range cands {
		if i > 0 {
			out.WriteByte('\n')
		}
		fmt.Fprintf(&out, "%s,%.0f", c.addr, c.score*100)
	}
	return out.String()
}

// readBestMatchEntries loads the persisted list as scored entries.
func readBestMatchEntries(filesDir string) []scoredResolver {
	lines := ReadBestMatchGlobal(filesDir)
	out := make([]scoredResolver, 0, len(lines))
	for _, ln := range lines {
		out = append(out, scoredResolver{addr: ln, score: 0})
	}
	return out
}

// writeBestMatchFile persists the ranked entries, pruning addresses that the
// live snapshot proves are failing while always keeping the healthy ones.
// Previously-persisted entries (score 0, merged from the old file) are kept at
// the tail so the list only shrinks when there is live evidence of failure.
func writeBestMatchFile(filesDir string, live []scoredResolver) {
	// Addresses proven bad right now: present in live snapshot with an
	// actively-failing quality record (acked == 0 after >= bestMatchMinSamples
	// sends, or marked invalid by the core). Also drop anything hard-failing
	// across instances (SendReported failures with zero acks).
	badAddrs := failingResolverAddrs()

	// Merge: live entries first (already ranked), then persisted entries that
	// are NOT proven bad and NOT already included (dedup, keep at tail).
	seen := make(map[string]struct{}, len(live))
	merged := make([]scoredResolver, 0, len(live)+bestMatchGlobalKeepCount)
	for _, c := range live {
		if _, dup := seen[c.addr]; dup {
			continue
		}
		seen[c.addr] = struct{}{}
		merged = append(merged, c)
	}
	pruned := 0
	for _, c := range readBestMatchEntries(filesDir) {
		if _, dup := seen[c.addr]; dup {
			continue
		}
		if _, isBad := badAddrs[c.addr]; isBad {
			pruned++
			continue // live-verified failure — remove from the list
		}
		seen[c.addr] = struct{}{}
		merged = append(merged, c)
	}

	// Never prune below the floor: keep the healthiest entries even if they
	// are flagged, otherwise a bad network could wipe the whole list.
	if len(merged) < bestMatchPruneFloor {
		pruned = 0
	}
	if len(merged) > bestMatchGlobalKeepCount {
		merged = merged[:bestMatchGlobalKeepCount]
	}

	var b strings.Builder
	b.WriteString("# Best Match resolvers — auto-updated from live quality stats\n")
	b.WriteString(fmt.Sprintf("# updated: %s | metric: ackRate^2 x rttFactor(250ms), min %d samples | pruned: %d\n",
		time.Now().UTC().Format(time.RFC3339), bestMatchMinSamples, pruned))
	for _, c := range merged {
		b.WriteString(c.addr)
		b.WriteByte('\n')
	}
	dir := filepath.Join(filesDir, bestMatchGlobalDirName)
	if err := os.MkdirAll(dir, 0o750); err == nil {
		_ = os.WriteFile(filepath.Join(dir, "best_match_resolvers.txt"), []byte(b.String()), 0o640)
	}
}

// failingResolverAddrs collects addresses with hard live evidence of failure.
// Two signals, both from instances whose scan has FINISHED (session ready):
//
//  1. Probed-and-rejected: !IsValid with zero live traffic — after the MTU
//     scan only resolvers that FAILED their probe stay unseeded, so these
//     were verified bad by the scan itself.
//  2. Died in live traffic: !IsValid with enough samples and a near-zero
//     ack rate (or literally zero acks) — the engine auto-disabled it after
//     repeated timeout observations.
//
// Transient timeouts with a healthy ack history never match, and instances
// still scanning never contribute (their unknowns must not be pruned).
func failingResolverAddrs() map[string]struct{} {
	bad := map[string]struct{}{}
	for _, id := range runningInstanceIDs() {
		stats := GetInstanceStats(id)
		if !stats.IsRunning || !stats.SessionReady {
			continue // scan in progress — no verdicts yet
		}
		for _, q := range GetResolverQualitySnapshot(id) {
			addr := q.Resolver
			if q.Port > 0 {
				addr = fmt.Sprintf("%s:%d", q.Resolver, q.Port)
			}
			if q.IsValid {
				continue
			}
			rejectedByScan := q.Sent == 0
			diedInTraffic := q.Sent >= bestMatchMinSamples &&
				(q.Acked == 0 || float64(q.Acked)/float64(q.Sent) < 0.1)
			if rejectedByScan || diedInTraffic {
				bad[addr] = struct{}{}
			}
		}
	}
	return bad
}

// runningInstanceIDs returns the IDs of the currently running instances.
func runningInstanceIDs() []string {
	instancesMu.Lock()
	defer instancesMu.Unlock()
	ids := make([]string, 0, len(instances))
	for id := range instances {
		ids = append(ids, id)
	}
	return ids
}

// ReadBestMatchGlobal returns the persisted shared Best Match resolver lines
// for the app's filesDir (nil when the file does not exist yet).
func ReadBestMatchGlobal(filesDir string) []string {
	data, err := os.ReadFile(filepath.Join(filesDir, bestMatchGlobalDirName, "best_match_resolvers.txt"))
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
