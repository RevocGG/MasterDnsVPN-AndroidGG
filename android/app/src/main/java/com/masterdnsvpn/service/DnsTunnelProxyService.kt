package com.masterdnsvpn.service

import android.app.NotificationManager
import android.app.Service
import android.content.Intent
import android.content.pm.ServiceInfo
import android.net.TrafficStats
import android.os.Build
import android.os.IBinder
import androidx.core.app.ServiceCompat
import com.masterdnsvpn.bridge.GoMobileBridge
import com.masterdnsvpn.bridge.ProfileConfigMapper
import com.masterdnsvpn.log.LogEntry
import com.masterdnsvpn.log.LogLevel
import com.masterdnsvpn.log.LogManager
import com.masterdnsvpn.profile.ProfileRepository
import dagger.hilt.android.AndroidEntryPoint
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import javax.inject.Inject

@AndroidEntryPoint
class DnsTunnelProxyService : Service() {

    companion object {
        const val EXTRA_PROFILE_ID = "profile_id"
        const val EXTRA_PROFILE_NAME = "profile_name"
        const val EXTRA_META_ID = "meta_id"
        const val EXTRA_META_PROFILE_IDS = "meta_profile_ids"
        const val EXTRA_BALANCER_STRATEGY = "balancer_strategy"
        const val EXTRA_SOCKS_PORT = "socks_port"
    }

    @Inject lateinit var bridge: GoMobileBridge
    @Inject lateinit var repo: ProfileRepository
    @Inject lateinit var tunnelStateManager: TunnelStateManager
    @Inject lateinit var logManager: LogManager

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private var activeProfileId: String? = null
    private var activeMetaId: String? = null
    private var activeSubProfileIds: List<String> = emptyList()
    private var activeProfileName: String = "Proxy"

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        val metaId = intent?.getStringExtra(EXTRA_META_ID)
        val metaProfileIds = intent?.getStringArrayListExtra(EXTRA_META_PROFILE_IDS)
        val singleProfileId = intent?.getStringExtra(EXTRA_PROFILE_ID)
        val profileName = intent?.getStringExtra(EXTRA_PROFILE_NAME) ?: "Proxy"

        if (singleProfileId == null && metaId == null) return START_NOT_STICKY

        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
            ServiceCompat.startForeground(
                this,
                TunnelNotification.NOTIFICATION_ID,
                TunnelNotification.build(this, profileName, "SOCKS5 Proxy"),
                ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE,
            )
        } else {
            startForeground(
                TunnelNotification.NOTIFICATION_ID,
                TunnelNotification.build(this, profileName, "SOCKS5 Proxy"),
            )
        }

        activeProfileName = profileName
        startSpeedMonitor()

        if (metaId != null && metaProfileIds != null) {
            // ── Meta profile mode: start all sub-profiles with SOCKS balancer ──
            activeMetaId = metaId
            activeSubProfileIds = metaProfileIds.toList()
            val strategy = intent?.getIntExtra(EXTRA_BALANCER_STRATEGY, 0) ?: 0
            val socksPort = intent?.getIntExtra(EXTRA_SOCKS_PORT, 0) ?: 0
            tunnelStateManager.onMetaStarted(metaId)

            scope.launch {
                try {
                    val upstreamAddrs = mutableListOf<String>()
                    for (pid in metaProfileIds) {
                        if (!isActive) break
                        val profile = repo.getProfileForTunnel(pid) ?: continue
                        val dir = "${filesDir.absolutePath}/profiles/${profile.id}"
                        java.io.File(dir).mkdirs()
                        try {
                            val cfg = ProfileConfigMapper.toMobileConfig(profile)
                            bridge.startInstance(profile.id, dir, cfg, profile.resolversText)
                            if (profile.identityLocked) bridge.setLockedDomains(profile.id, profile.domains.split(","))
                            tunnelStateManager.onTunnelStarted(profile.id)
                            upstreamAddrs.add("${cfg.listenIP}:${cfg.listenPort}")
                            logManager.append(LogEntry(level = LogLevel.INFO, timestamp = "system", message = "Meta sub-profile started (SOCKS): ${profile.name} on ${cfg.listenIP}:${cfg.listenPort}"))
                        } catch (e: kotlinx.coroutines.CancellationException) {
                            throw e
                        } catch (e: Exception) {
                            logManager.appendSystem(LogLevel.ERROR, "Failed: ${profile.name}: ${e.message}")
                        }
                    }

                    if (!isActive) return@launch

                    // Wait for proxies to initialize
                    delay(1000)

                    // Start SOCKS balancer on a meta port — always start even with a single
                    // upstream so that the configured socksPort is always bound.
                    if (upstreamAddrs.isNotEmpty()) {
                        val csv = upstreamAddrs.joinToString(",")
                        val bindAddr = if (socksPort > 0) "127.0.0.1:$socksPort" else "127.0.0.1:0"
                        val addr = bridge.startSocksBalancer(bindAddr, csv, strategy)
                        if (addr != null) {
                            logManager.append(LogEntry(level = LogLevel.INFO, timestamp = "system", message = "Meta SOCKS balancer on $addr (strategy=$strategy, ${upstreamAddrs.size} upstreams)"))
                        }
                    }
                } catch (e: kotlinx.coroutines.CancellationException) {
                    throw e
                } catch (e: Exception) {
                    logManager.appendSystem(LogLevel.ERROR, "Meta proxy startup error: ${e.message}")
                }
            }
        } else if (singleProfileId != null) {
            // ── Single profile mode (existing behavior) ──
            activeProfileId = singleProfileId
            tunnelStateManager.onTunnelStarted(singleProfileId)

            scope.launch {
                try {
                    val profile = repo.getProfileForTunnel(singleProfileId) ?: run {
                        tunnelStateManager.onTunnelStopped(singleProfileId)
                        stopSelf()
                        return@launch
                    }
                    val profileDir = "${filesDir.absolutePath}/profiles/${profile.id}"
                    java.io.File(profileDir).mkdirs()
                    logManager.appendSystem(LogLevel.INFO, "Proxy service starting: ${profile.name}")

                    if (profile.identityLocked) bridge.setLockedDomains(profile.id, profile.domains.split(","))
                    bridge.startInstance(
                        profileId = profile.id,
                        profileDir = profileDir,
                        config = ProfileConfigMapper.toMobileConfig(profile),
                        resolversText = profile.resolversText,
                    )
                } catch (e: kotlinx.coroutines.CancellationException) {
                    throw e  // Never swallow coroutine cancellation
                } catch (e: Exception) {
                    logManager.appendSystem(LogLevel.ERROR, "Tunnel startup error: ${e.message}")
                    tunnelStateManager.onTunnelStopped(singleProfileId)
                    stopSelf()
                }
            }
        }

        return START_NOT_STICKY
    }

    private fun startSpeedMonitor() {
        scope.launch {
            val uid = android.os.Process.myUid()
            val baseRx = TrafficStats.getUidRxBytes(uid)
            val baseTx = TrafficStats.getUidTxBytes(uid)
            var prevRx = baseRx
            var prevTx = baseTx
            val nm = getSystemService(NotificationManager::class.java)
            while (isActive) {
                delay(1_000)
                if (!isActive) break
                val rx = TrafficStats.getUidRxBytes(uid)
                val tx = TrafficStats.getUidTxBytes(uid)
                val rxSpeed = (rx - prevRx).coerceAtLeast(0L)
                val txSpeed = (tx - prevTx).coerceAtLeast(0L)
                prevRx = rx
                prevTx = tx
                val totalRx = (rx - baseRx).coerceAtLeast(0L)
                val totalTx = (tx - baseTx).coerceAtLeast(0L)
                val notif = TunnelNotification.buildWithSpeed(
                    this@DnsTunnelProxyService,
                    activeProfileName,
                    "SOCKS5 Proxy",
                    rxSpeed,
                    txSpeed,
                    totalRx,
                    totalTx,
                )
                nm.notify(TunnelNotification.NOTIFICATION_ID, notif)
            }
            // Speed monitor coroutine exited — cancel the notification so it never gets stuck
            nm.cancel(TunnelNotification.NOTIFICATION_ID)
        }
    }

    override fun onDestroy() {
        // Stop balancer first
        try { bridge.stopSocksBalancer() } catch (_: Exception) {}
        // Stop meta sub-profiles
        if (activeMetaId != null) {
            for (subId in activeSubProfileIds) {
                try { bridge.stopInstance(subId) } catch (_: Exception) {}
                bridge.clearLockedDomains(subId)
                tunnelStateManager.onTunnelStopped(subId)
            }
            tunnelStateManager.onMetaStopped(activeMetaId!!)
        }
        // Stop single profile
        activeProfileId?.let { id ->
            try { bridge.stopInstance(id) } catch (_: Exception) {}
            bridge.clearLockedDomains(id)
            tunnelStateManager.onTunnelStopped(id)
        }
        stopForeground(STOP_FOREGROUND_REMOVE)
        getSystemService(NotificationManager::class.java)
            ?.cancel(TunnelNotification.NOTIFICATION_ID)
        scope.cancel()
        super.onDestroy()
    }

    override fun onBind(intent: Intent?): IBinder? = null
}