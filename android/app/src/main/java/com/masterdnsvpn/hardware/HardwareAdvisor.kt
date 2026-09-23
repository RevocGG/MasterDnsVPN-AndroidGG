package com.masterdnsvpn.hardware

import android.app.ActivityManager
import android.content.Context
import androidx.annotation.StringRes
import com.masterdnsvpn.R
import com.masterdnsvpn.profile.ProfileEntity

/**
 * Represents a single setting that may cause hardware pressure on the current device.
 *
 * [labelRes]/[reasonRes] are localized string resources so warnings follow the
 * user's selected app language; [reasonArgs] carries the dynamic values
 * (current value, recommended value, device numbers…).
 * [applyTo] is a pure function that returns a new [ProfileEntity] with the recommended
 * value applied. It is only held in memory (never serialized).
 */
data class ProfileWarning(
    val fieldKey: String,
    @StringRes val labelRes: Int,
    @StringRes val reasonRes: Int,
    val reasonArgs: List<String>,
    val currentValue: String,
    val recommendedValue: String,
    val applyTo: (ProfileEntity) -> ProfileEntity,
)

/**
 * Compares a profile's settings against the current device's CPU core count and RAM,
 * and returns warnings for settings that are likely to cause CPU or RAM pressure.
 *
 * Only settings with a direct, measurable impact on hardware performance are checked.
 * Settings that purely affect network behaviour (timeouts, retry counts, etc.) are skipped.
 */
object HardwareAdvisor {

    fun check(ctx: Context, profile: ProfileEntity): List<ProfileWarning> {
        val cpuCores = Runtime.getRuntime().availableProcessors()
        val am = ctx.applicationContext.getSystemService(Context.ACTIVITY_SERVICE) as ActivityManager
        val memInfo = ActivityManager.MemoryInfo()
        am.getMemoryInfo(memInfo)
        val totalRamMb = (memInfo.totalMem / (1024L * 1024L)).toInt()

        // Device tiers based on combined CPU + RAM signal
        val isWeakDevice = cpuCores <= 4 || totalRamMb <= 3072
        val isVeryWeakDevice = cpuCores <= 2 || totalRamMb <= 1536

        val warnings = mutableListOf<ProfileWarning>()

        // Safe maximum workers: 1.5× core count keeps CPU usage at or below ~70%.
        // Beyond this ratio, goroutine scheduling overhead pushes CPU load past 70%,
        // causing thermal throttling, battery drain, and foreground app jank.
        // Example: 8 cores → safe max = 12. Values up to 12 are allowed without warning.
        val safeMaxWorkers = (cpuCores * 1.5).toInt().coerceAtLeast(cpuCores)

        // ── 1. RX/TX Workers vs CPU cores ─────────────────────────────────────────
        if (profile.rxTxWorkers > safeMaxWorkers) {
            warnings += ProfileWarning(
                fieldKey = "rxTxWorkers",
                labelRes = R.string.warn_label_rx_tx_workers,
                reasonRes = R.string.warn_reason_workers,
                reasonArgs = listOf(profile.rxTxWorkers.toString(), cpuCores.toString(), safeMaxWorkers.toString()),
                currentValue = profile.rxTxWorkers.toString(),
                recommendedValue = safeMaxWorkers.toString(),
                applyTo = { it.copy(rxTxWorkers = safeMaxWorkers) },
            )
        }

        // ── 2. Tunnel Process Workers vs CPU cores ──────────────────────────────────
        if (profile.tunnelProcessWorkers > safeMaxWorkers) {
            warnings += ProfileWarning(
                fieldKey = "tunnelProcessWorkers",
                labelRes = R.string.warn_label_tunnel_workers,
                reasonRes = R.string.warn_reason_workers,
                reasonArgs = listOf(profile.tunnelProcessWorkers.toString(), cpuCores.toString(), safeMaxWorkers.toString()),
                currentValue = profile.tunnelProcessWorkers.toString(),
                recommendedValue = safeMaxWorkers.toString(),
                applyTo = { it.copy(tunnelProcessWorkers = safeMaxWorkers) },
            )
        }

        // ── Checks that only apply to weak/very-weak devices ────────────────────────
        if (isWeakDevice) {

            // 3. TX Channel Size — pure RAM allocation
            if (profile.txChannelSize > 6000) {
                warnings += ProfileWarning(
                    fieldKey = "txChannelSize",
                    labelRes = R.string.warn_label_tx_buffer,
                    reasonRes = R.string.warn_reason_buffer_ram,
                    reasonArgs = listOf(totalRamMb.toString()),
                    currentValue = profile.txChannelSize.toString(),
                    recommendedValue = "4096",
                    applyTo = { it.copy(txChannelSize = 4096) },
                )
            }

            // 4. RX Channel Size — pure RAM allocation
            if (profile.rxChannelSize > 6000) {
                warnings += ProfileWarning(
                    fieldKey = "rxChannelSize",
                    labelRes = R.string.warn_label_rx_buffer,
                    reasonRes = R.string.warn_reason_buffer_ram,
                    reasonArgs = listOf(totalRamMb.toString()),
                    currentValue = profile.rxChannelSize.toString(),
                    recommendedValue = "4096",
                    applyTo = { it.copy(rxChannelSize = 4096) },
                )
            }

            // 5. ARQ Window Size — in-flight packet buffers held in RAM
            val arqThreshold = if (isVeryWeakDevice) 400 else 700
            val arqRec = if (isVeryWeakDevice) 300 else 500
            if (profile.arqWindowSize > arqThreshold) {
                warnings += ProfileWarning(
                    fieldKey = "arqWindowSize",
                    labelRes = R.string.warn_label_arq_window,
                    reasonRes = R.string.warn_reason_arq_ram,
                    reasonArgs = listOf(totalRamMb.toString()),
                    currentValue = profile.arqWindowSize.toString(),
                    recommendedValue = arqRec.toString(),
                    applyTo = { it.copy(arqWindowSize = arqRec) },
                )
            }

            // 6. Local DNS Cache — only meaningful when local DNS is enabled
            if (profile.localDnsEnabled) {
                val localThreshold = if (isVeryWeakDevice) 3000 else 5000
                val localRec = if (isVeryWeakDevice) 2000 else 3000
                if (profile.localDnsCacheMaxRecords > localThreshold) {
                    warnings += ProfileWarning(
                        fieldKey = "localDnsCacheMaxRecords",
                        labelRes = R.string.warn_label_dns_cache,
                        reasonRes = R.string.warn_reason_dns_cache_ram,
                        reasonArgs = listOf(localRec.toString(), totalRamMb.toString()),
                        currentValue = profile.localDnsCacheMaxRecords.toString(),
                        recommendedValue = localRec.toString(),
                        applyTo = { it.copy(localDnsCacheMaxRecords = localRec) },
                    )
                }
            }

            // 7. Resolver UDP connection pool — file descriptors + memory per socket
            if (profile.resolverUdpConnectionPoolSize > 64) {
                val rec = if (isVeryWeakDevice) 24 else 32
                warnings += ProfileWarning(
                    fieldKey = "resolverUdpConnectionPoolSize",
                    labelRes = R.string.warn_label_udp_pool,
                    reasonRes = R.string.warn_reason_udp_pool,
                    reasonArgs = listOf(),
                    currentValue = profile.resolverUdpConnectionPoolSize.toString(),
                    recommendedValue = rec.toString(),
                    applyTo = { it.copy(resolverUdpConnectionPoolSize = rec) },
                )
            }

            // 10. Ping aggressive interval — sub-150ms means >6 pings/sec per resolver
            if (profile.pingAggressiveIntervalSeconds < 0.150) {
                val pingsPerSec = (1.0 / profile.pingAggressiveIntervalSeconds).toInt()
                warnings += ProfileWarning(
                    fieldKey = "pingAggressiveIntervalSeconds",
                    labelRes = R.string.warn_label_ping_interval,
                    reasonRes = R.string.warn_reason_ping_interval,
                    reasonArgs = listOf(
                        "%.3f".format(profile.pingAggressiveIntervalSeconds),
                        pingsPerSec.toString(),
                    ),
                    currentValue = "%.3fs".format(profile.pingAggressiveIntervalSeconds),
                    recommendedValue = "0.3s",
                    applyTo = { it.copy(pingAggressiveIntervalSeconds = 0.3) },
                )
            }
        }

        // ── 8. ZLIB compression — CPU per packet on low-core devices ─────────────
        // TypeZLIB = 3 (from internal/compression/types.go)
        if (cpuCores <= 4) {
            if (profile.uploadCompressionType == 3) {
                warnings += ProfileWarning(
                    fieldKey = "uploadCompressionType",
                    labelRes = R.string.warn_label_upload_zlib,
                    reasonRes = R.string.warn_reason_zlib,
                    reasonArgs = listOf(),
                    currentValue = "ZLIB",
                    recommendedValue = "Disabled",
                    applyTo = { it.copy(uploadCompressionType = 0) },
                )
            }
            if (profile.downloadCompressionType == 3) {
                warnings += ProfileWarning(
                    fieldKey = "downloadCompressionType",
                    labelRes = R.string.warn_label_download_zlib,
                    reasonRes = R.string.warn_reason_zlib,
                    reasonArgs = listOf(),
                    currentValue = "ZLIB",
                    recommendedValue = "Disabled",
                    applyTo = { it.copy(downloadCompressionType = 0) },
                )
            }
        }

        // ── 9. Log Level — applies on ALL devices ─────────────────────────────────
        // DEBUG/TRACE generates a large string per packet and writes to disk continuously.
        if (profile.logLevel.uppercase() in setOf("DEBUG", "TRACE")) {
            warnings += ProfileWarning(
                fieldKey = "logLevel",
                labelRes = R.string.warn_label_log_level,
                reasonRes = R.string.warn_reason_log_level,
                reasonArgs = listOf(profile.logLevel),
                currentValue = profile.logLevel,
                recommendedValue = "INFO",
                applyTo = { it.copy(logLevel = "INFO") },
            )
        }

        return warnings
    }
}
