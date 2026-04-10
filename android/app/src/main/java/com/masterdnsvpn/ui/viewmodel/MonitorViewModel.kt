package com.masterdnsvpn.ui.viewmodel

import android.app.ActivityManager
import android.content.Context
import android.net.TrafficStats
import android.os.Process
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.masterdnsvpn.bridge.GoMobileBridge
import com.masterdnsvpn.gomobile.mobile.Stats
import com.masterdnsvpn.profile.ProfileEntity
import com.masterdnsvpn.profile.ProfileRepository
import com.masterdnsvpn.service.TunnelStateManager
import dagger.hilt.android.lifecycle.HiltViewModel
import dagger.hilt.android.qualifiers.ApplicationContext
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.*
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import javax.inject.Inject

data class ProfileMonitorInfo(
    val profile: ProfileEntity,
    val stats: Stats?,
    val lastError: String?,
    val uploadBytes: Long = 0L,
    val downloadBytes: Long = 0L,
)

data class MonitorUiState(
    val cpuUsage: Float = 0f,
    val memoryUsageMb: Long = 0,
    val memoryTotalMb: Long = 0,
    val uploadBytes: Long = 0,
    val downloadBytes: Long = 0,
    val uploadSpeed: Long = 0,   // bytes/sec
    val downloadSpeed: Long = 0, // bytes/sec
    val activeProfiles: List<ProfileMonitorInfo> = emptyList(),
)

@HiltViewModel
class MonitorViewModel @Inject constructor(
    @ApplicationContext private val appContext: Context,
    private val bridge: GoMobileBridge,
    private val repo: ProfileRepository,
    private val tunnelStateManager: TunnelStateManager,
    val bandwidthPrefs: com.masterdnsvpn.settings.ProfileBandwidthPrefs,
) : ViewModel() {

    private val _state = MutableStateFlow(MonitorUiState())
    val state: StateFlow<MonitorUiState> = _state.asStateFlow()

    private var prevUpload = 0L
    private var prevDownload = 0L
    /** Wire traffic — TrafficStats counters, updated every tick. */
    private var prevWireRx = 0L
    private var prevWireTx = 0L
    /** Accumulated bytes for SOCKS mode display (no TUN bridge counter available). */
    private var socksSessionTxAccum = 0L
    private var socksSessionRxAccum = 0L
    /** Track which profiles were running last tick, so we can flush on stop. */
    private var prevRunningIds = emptySet<String>()
    private var prevCpuTime = 0L
    private var prevWallTime = 0L

    init {
        // Initialize baseline
        prevCpuTime = android.os.Process.getElapsedCpuTime()
        prevWallTime = android.os.SystemClock.elapsedRealtime()
        val uid = android.os.Process.myUid()
        prevWireRx = TrafficStats.getUidRxBytes(uid).coerceAtLeast(0)
        prevWireTx = TrafficStats.getUidTxBytes(uid).coerceAtLeast(0)

        viewModelScope.launch(Dispatchers.IO) {
            while (isActive) {
                updateStats()
                delay(1_000)
            }
        }
    }

    private suspend fun updateStats() {
        // CPU usage
        val cpuUsage = measureCpuUsage()

        // Memory
        val am = appContext.getSystemService(Context.ACTIVITY_SERVICE) as ActivityManager
        val memInfo = ActivityManager.MemoryInfo()
        am.getMemoryInfo(memInfo)
        val usedMb = readProcMemMb()
        val totalMb = memInfo.totalMem / (1024 * 1024)

        // Wire traffic (actual network bytes this process sent/received, including protocol overhead).
        val uid = android.os.Process.myUid()
        val wireRx = TrafficStats.getUidRxBytes(uid).coerceAtLeast(0)
        val wireTx = TrafficStats.getUidTxBytes(uid).coerceAtLeast(0)
        val wireRxDelta = (wireRx - prevWireRx).coerceAtLeast(0)
        val wireTxDelta = (wireTx - prevWireTx).coerceAtLeast(0)

        // TUN bridge bandwidth (only non-zero when TUN mode VPN is active).
        val tunActive = bridge.isTunBridgeRunning()
        val (up, down) = bridge.getBandwidth()
        val tunUpDelta = (up - prevUpload).coerceAtLeast(0)
        val tunDownDelta = (down - prevDownload).coerceAtLeast(0)

        // Derive per-second speeds and session totals depending on mode.
        val uploadSpeed: Long
        val downloadSpeed: Long
        val sessionDisplayTx: Long  // cumulative for MonitorUiState tiles
        val sessionDisplayRx: Long

        if (tunActive) {
            // TUN mode: TUN bridge counters are authoritative for user data.
            uploadSpeed = tunUpDelta
            downloadSpeed = tunDownDelta
            sessionDisplayTx = up
            sessionDisplayRx = down
            // Reset SOCKS accumulators so they don't carry stale data if mode switches.
            socksSessionTxAccum = 0L
            socksSessionRxAccum = 0L
        } else {
            // SOCKS mode: no TUN bridge — use TrafficStats delta as best approximation.
            uploadSpeed = wireTxDelta
            downloadSpeed = wireRxDelta
            socksSessionTxAccum += wireTxDelta
            socksSessionRxAccum += wireRxDelta
            sessionDisplayTx = socksSessionTxAccum
            sessionDisplayRx = socksSessionRxAccum
        }

        // Persist per-profile bandwidth to SharedPreferences.
        val currentRunning = tunnelStateManager.runningProfileIds.value.toSet()
        if (currentRunning.isNotEmpty()) {
            if (tunActive) {
                // TUN mode: save TUN delta as session bytes, wire delta as protocol overhead.
                if (tunUpDelta > 0 || tunDownDelta > 0) {
                    val perUp = tunUpDelta / currentRunning.size
                    val perDown = tunDownDelta / currentRunning.size
                    currentRunning.forEach { id ->
                        bandwidthPrefs.addSessionBytes(id, perUp, perDown)
                    }
                }
                if (wireRxDelta > 0 || wireTxDelta > 0) {
                    val perWireUp = wireTxDelta / currentRunning.size
                    val perWireDown = wireRxDelta / currentRunning.size
                    currentRunning.forEach { id ->
                        bandwidthPrefs.addWireBytes(id, perWireUp, perWireDown)
                    }
                }
            } else {
                // SOCKS mode: wire bytes ARE the session data (no separate overhead layer).
                // We accumulate both session and wire bytes with the same delta so that
                // overhead = wireTotalBytes - totalUsageBytes stays at zero rather than
                // decreasing when totalUsageBytes catches up to a stale wireTotalBytes
                // left over from a previous TUN session.
                if (wireRxDelta > 0 || wireTxDelta > 0) {
                    val perUp = wireTxDelta / currentRunning.size
                    val perDown = wireRxDelta / currentRunning.size
                    currentRunning.forEach { id ->
                        bandwidthPrefs.addSessionBytes(id, perUp, perDown)
                        bandwidthPrefs.addWireBytes(id, perUp, perDown)
                    }
                }
            }
        }

        prevRunningIds = currentRunning
        prevUpload = up
        prevDownload = down
        prevWireRx = wireRx
        prevWireTx = wireTx

        // Active profiles
        val activeProfileIds = tunnelStateManager.runningProfileIds.value
        val profiles = if (activeProfileIds.isNotEmpty()) {
            activeProfileIds.mapNotNull { id ->
                val profile = repo.getProfile(id) ?: return@mapNotNull null
                val stats = try { bridge.getStats(id) } catch (_: Exception) { null }
                val error = try { bridge.getLastError(id) } catch (_: Exception) { null }
                ProfileMonitorInfo(
                    profile = profile,
                    stats = stats,
                    lastError = error,
                    uploadBytes = bandwidthPrefs.getUploadBytes(id),
                    downloadBytes = bandwidthPrefs.getDownloadBytes(id),
                )
            }
        } else emptyList()

        _state.value = MonitorUiState(
            cpuUsage = cpuUsage,
            memoryUsageMb = usedMb,
            memoryTotalMb = totalMb,
            uploadBytes = sessionDisplayTx,
            downloadBytes = sessionDisplayRx,
            uploadSpeed = uploadSpeed,
            downloadSpeed = downloadSpeed,
            activeProfiles = profiles,
        )
    }

    private fun readProcMemMb(): Long {
        return try {
            java.io.File("/proc/self/status").readLines()
                .firstOrNull { it.startsWith("VmRSS:") }
                ?.trim()?.split("\\s+".toRegex())?.getOrNull(1)?.toLong()?.div(1024L) ?: 0L
        } catch (_: Exception) {
            val rt = Runtime.getRuntime()
            (rt.totalMemory() - rt.freeMemory()) / (1024 * 1024)
        }
    }

    private fun measureCpuUsage(): Float {
        val currentCpu = android.os.Process.getElapsedCpuTime() // millis
        val currentWall = android.os.SystemClock.elapsedRealtime() // millis
        val cpuDelta = currentCpu - prevCpuTime
        val wallDelta = currentWall - prevWallTime
        prevCpuTime = currentCpu
        prevWallTime = currentWall
        return if (wallDelta > 0 && cpuDelta >= 0) {
            ((cpuDelta.toFloat() / wallDelta.toFloat()) * 100f).coerceIn(0f, 100f)
        } else 0f
    }
}
