package com.masterdnsvpn.service

import com.masterdnsvpn.bridge.GoMobileBridge
import com.masterdnsvpn.gomobile.mobile.Stats
import com.masterdnsvpn.profile.MetaProfileEntity
import com.masterdnsvpn.profile.ProfileRepository
import kotlinx.coroutines.*
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Monitors latency/loss metrics across all profiles in a [MetaProfileEntity]
 * and automatically switches to the best candidate.
 *
 * Balancing strategies (mirror of internal/client/balancer.go constants):
 *   0 = RR Default   (rotate on connect)
 *   1 = Random
 *   2 = Round-Robin  (rotate on each start call)
 *   3 = Least-Loss   (lowest validResolverCount / resolverCount ratio degradation)
 *   4 = Lowest-Latency (highest validResolverCount)
 *
 * The monitor polls stats every [POLL_INTERVAL_MS] and calls [onSwitch] when
 * the active profile should change.
 */
@Singleton
class MetaProfileBalancer @Inject constructor(
    private val repo: ProfileRepository,
    private val bridge: GoMobileBridge,
) {
    companion object {
        private const val POLL_INTERVAL_MS = 3_000L
        /** Below this fraction of valid resolvers a profile is considered degraded. */
        private const val DEGRADED_THRESHOLD = 0.5
    }

    private var monitorJob: Job? = null
    private var rrIndex = 0

    /**
     * Start monitoring [meta]. When a better profile is detected,
     * [onSwitch] is called with the new profile ID.
     */
    fun start(
        meta: MetaProfileEntity,
        scope: CoroutineScope,
        onSwitch: suspend (newProfileId: String) -> Unit,
    ) {
        stop()
        monitorJob = scope.launch {
            val profileIds = meta.profileIds.split(",").filter { it.isNotBlank() }
            if (profileIds.isEmpty()) return@launch

            while (isActive) {
                delay(POLL_INTERVAL_MS)
                val stats: Map<String, Stats?> = profileIds.associateWith { id ->
                    if (bridge.isRunning(id)) bridge.getStats(id) else null
                }

                val runningIds = stats.filterValues { it?.isRunning == true }.keys.toList()
                if (runningIds.isEmpty()) continue

                val best = when (meta.balancingStrategy) {
                    1 -> profileIds.random() // Random
                    2 -> { // Round-Robin
                        val idx = rrIndex % runningIds.size
                        rrIndex++
                        runningIds[idx]
                    }
                    3 -> { // Least-Loss — most valid resolvers / total
                        runningIds.maxByOrNull { id ->
                            val s = stats[id] ?: return@maxByOrNull 0.0
                            if (s.resolverCount == 0L) 0.0
                            else s.validResolverCount.toDouble() / s.resolverCount.toDouble()
                        }
                    }
                    4 -> { // Lowest-Latency — highest valid resolver count
                        runningIds.maxByOrNull { id -> stats[id]?.validResolverCount ?: 0L }
                    }
                    else -> null // 0 = default, no auto-switch
                } ?: continue

                // Check if the currently "active" profile is degraded
                val activeId = runningIds.firstOrNull() ?: continue
                val activeStats = stats[activeId]
                val isDegraded = activeStats != null &&
                    activeStats.resolverCount > 0L &&
                    activeStats.validResolverCount.toDouble() / activeStats.resolverCount.toDouble() < DEGRADED_THRESHOLD

                if (isDegraded && best != activeId) {
                    onSwitch(best)
                }
            }
        }
    }

    fun stop() {
        monitorJob?.cancel()
        monitorJob = null
    }
}