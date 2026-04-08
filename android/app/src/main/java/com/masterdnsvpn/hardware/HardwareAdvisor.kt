package com.masterdnsvpn.hardware

import android.app.ActivityManager
import android.content.Context
import com.masterdnsvpn.profile.ProfileEntity

/**
 * Represents a single setting that may cause hardware pressure on the current device.
 *
 * [applyTo] is a pure function that returns a new [ProfileEntity] with the recommended
 * value applied. It is only held in memory (never serialized).
 */
data class ProfileWarning(
    val fieldKey: String,
    val fieldLabel: String,
    val currentValue: String,
    val recommendedValue: String,
    val reason: String,
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

        // ── 1. RX/TX Workers vs CPU cores ─────────────────────────────────────────
        // Each worker is a goroutine that continuously reads/writes UDP sockets.
        // More goroutines than cores = thread contention = permanent CPU overhead.
        if (profile.rxTxWorkers > cpuCores) {
            val rec = minOf(cpuCores, 4)
            warnings += ProfileWarning(
                fieldKey = "rxTxWorkers",
                fieldLabel = "RX/TX Workers",
                currentValue = profile.rxTxWorkers.toString(),
                recommendedValue = rec.toString(),
                reason = "Current value (${profile.rxTxWorkers}) exceeds device CPU core count ($cpuCores). This causes thread contention and permanent CPU overhead.",
                applyTo = { it.copy(rxTxWorkers = rec) },
            )
        }

        // ── 2. Tunnel Process Workers vs CPU cores ──────────────────────────────────
        // Each worker is a goroutine that decrypts and dispatches packets.
        // More goroutines than cores = thread contention = permanent CPU overhead.
        if (profile.tunnelProcessWorkers > cpuCores) {
            val rec = minOf(cpuCores, 4)
            warnings += ProfileWarning(
                fieldKey = "tunnelProcessWorkers",
                fieldLabel = "Tunnel Process Workers",
                currentValue = profile.tunnelProcessWorkers.toString(),
                recommendedValue = rec.toString(),
                reason = "Current value (${profile.tunnelProcessWorkers}) exceeds device CPU core count ($cpuCores). This causes thread contention and permanent CPU overhead.",
                applyTo = { it.copy(tunnelProcessWorkers = rec) },
            )
        }

        // ── Checks that only apply to weak/very-weak devices ────────────────────────
        if (isWeakDevice) {

            // 3. TX Channel Size — pure RAM allocation
            if (profile.txChannelSize > 6000) {
                warnings += ProfileWarning(
                    fieldKey = "txChannelSize",
                    fieldLabel = "TX Channel Buffer",
                    currentValue = profile.txChannelSize.toString(),
                    recommendedValue = "4096",
                    reason = "Large buffer wastes memory on devices with limited RAM (${totalRamMb}MB).",
                    applyTo = { it.copy(txChannelSize = 4096) },
                )
            }

            // 4. RX Channel Size — pure RAM allocation
            if (profile.rxChannelSize > 6000) {
                warnings += ProfileWarning(
                    fieldKey = "rxChannelSize",
                    fieldLabel = "RX Channel Buffer",
                    currentValue = profile.rxChannelSize.toString(),
                    recommendedValue = "4096",
                    reason = "Large buffer wastes memory on devices with limited RAM (${totalRamMb}MB).",
                    applyTo = { it.copy(rxChannelSize = 4096) },
                )
            }

            // 5. ARQ Window Size — in-flight packet buffers held in RAM
            val arqThreshold = if (isVeryWeakDevice) 400 else 700
            val arqRec = if (isVeryWeakDevice) 300 else 500
            if (profile.arqWindowSize > arqThreshold) {
                warnings += ProfileWarning(
                    fieldKey = "arqWindowSize",
                    fieldLabel = "ARQ Window Size",
                    currentValue = profile.arqWindowSize.toString(),
                    recommendedValue = arqRec.toString(),
                    reason = "In-flight packets are held in RAM. Reducing this value lowers RAM pressure on your device (${totalRamMb}MB).",
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
                        fieldLabel = "Local DNS Cache Max Records",
                        currentValue = profile.localDnsCacheMaxRecords.toString(),
                        recommendedValue = localRec.toString(),
                        reason = "Each DNS record is stored in RAM. $localRec records is sufficient for your device (${totalRamMb}MB RAM).",
                        applyTo = { it.copy(localDnsCacheMaxRecords = localRec) },
                    )
                }
            }

            // 7. Resolver UDP connection pool — file descriptors + memory per socket
            if (profile.resolverUdpConnectionPoolSize > 64) {
                val rec = if (isVeryWeakDevice) 24 else 32
                warnings += ProfileWarning(
                    fieldKey = "resolverUdpConnectionPoolSize",
                    fieldLabel = "UDP Connection Pool Size",
                    currentValue = profile.resolverUdpConnectionPoolSize.toString(),
                    recommendedValue = rec.toString(),
                    reason = "Each connection occupies a system socket. Reducing this value lowers file descriptor and RAM pressure.",
                    applyTo = { it.copy(resolverUdpConnectionPoolSize = rec) },
                )
            }

            // 10. Ping aggressive interval — sub-150ms means >6 pings/sec per resolver
            if (profile.pingAggressiveIntervalSeconds < 0.150) {
                val pingsPerSec = (1.0 / profile.pingAggressiveIntervalSeconds).toInt()
                warnings += ProfileWarning(
                    fieldKey = "pingAggressiveIntervalSeconds",
                    fieldLabel = "Aggressive Ping Interval",
                    currentValue = "%.3fs".format(profile.pingAggressiveIntervalSeconds),
                    recommendedValue = "0.3s",
                    reason = "Interval of ${profile.pingAggressiveIntervalSeconds}s means more than $pingsPerSec pings/sec, stressing a weak CPU.",
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
                    fieldLabel = "Upload Compression (ZLIB)",
                    currentValue = "ZLIB",
                    recommendedValue = "Disabled",
                    reason = "ZLIB consumes significant CPU per packet and is not recommended on devices with fewer than 4 cores.",
                    applyTo = { it.copy(uploadCompressionType = 0) },
                )
            }
            if (profile.downloadCompressionType == 3) {
                warnings += ProfileWarning(
                    fieldKey = "downloadCompressionType",
                    fieldLabel = "Download Compression (ZLIB)",
                    currentValue = "ZLIB",
                    recommendedValue = "Disabled",
                    reason = "ZLIB consumes significant CPU per packet and is not recommended on devices with fewer than 4 cores.",
                    applyTo = { it.copy(downloadCompressionType = 0) },
                )
            }
        }

        // ── 9. Log Level — applies on ALL devices ─────────────────────────────────
        // DEBUG/TRACE generates a large string per packet and writes to disk continuously.
        if (profile.logLevel.uppercase() in setOf("DEBUG", "TRACE")) {
            warnings += ProfileWarning(
                fieldKey = "logLevel",
                fieldLabel = "Log Level",
                currentValue = profile.logLevel,
                recommendedValue = "INFO",
                reason = "${profile.logLevel} level generates and writes large strings per packet, heavily consuming device CPU and I/O.",
                applyTo = { it.copy(logLevel = "INFO") },
            )
        }

        return warnings
    }
}
