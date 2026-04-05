package com.masterdnsvpn.service

import android.app.NotificationManager
import android.content.Intent
import android.content.pm.ServiceInfo
import android.net.TrafficStats
import android.net.VpnService
import android.os.Build
import android.os.ParcelFileDescriptor
import androidx.core.app.ServiceCompat
import com.masterdnsvpn.bridge.GoMobileBridge
import com.masterdnsvpn.bridge.ProfileConfigMapper
import com.masterdnsvpn.bridge.SocketProtector
import com.masterdnsvpn.log.LogEntry
import com.masterdnsvpn.log.LogLevel
import com.masterdnsvpn.log.LogManager
import com.masterdnsvpn.profile.ProfileEntity
import com.masterdnsvpn.profile.ProfileRepository
import com.masterdnsvpn.settings.AppSelectionPrefs
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
class DnsTunnelVpnService : VpnService() {

    companion object {
        const val EXTRA_PROFILE_ID = "profile_id"
        const val EXTRA_PROFILE_NAME = "profile_name"
        const val EXTRA_META_ID = "meta_id"
        const val EXTRA_META_PROFILE_IDS = "meta_profile_ids"
        const val EXTRA_BALANCER_STRATEGY = "balancer_strategy"
        const val EXTRA_SOCKS_PORT = "socks_port"
        const val ACTION_STOP = "com.masterdnsvpn.action.STOP_VPN"
    }

    @Inject lateinit var bridge: GoMobileBridge
    @Inject lateinit var repo: ProfileRepository
    @Inject lateinit var tunnelStateManager: TunnelStateManager
    @Inject lateinit var logManager: LogManager
    @Inject lateinit var appSelectionPrefs: AppSelectionPrefs

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private var vpnInterface: ParcelFileDescriptor? = null
    /** Set to true before intentional stops so crash detection doesn't fire. */
    @Volatile private var intentionalStop = false
    private var activeProfileId: String? = null
    private var activeMetaId: String? = null
    private var activeSubProfileIds: List<String> = emptyList()
    private var activeProfileName: String = "VPN"

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        // Handle stop request from TunnelController or notification button
        if (intent?.action == ACTION_STOP) {
            performStop()
            return START_NOT_STICKY
        }

        val metaId = intent?.getStringExtra(EXTRA_META_ID)
        val metaProfileIds = intent?.getStringArrayListExtra(EXTRA_META_PROFILE_IDS)
        val singleProfileId = intent?.getStringExtra(EXTRA_PROFILE_ID)
        val profileName = intent?.getStringExtra(EXTRA_PROFILE_NAME) ?: "VPN"

        // Must have either a single profile or a meta profile
        if (singleProfileId == null && metaId == null) return START_NOT_STICKY

        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
            ServiceCompat.startForeground(
                this,
                TunnelNotification.NOTIFICATION_ID,
                TunnelNotification.build(this, profileName, "VPN"),
                ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE,
            )
        } else {
            startForeground(
                TunnelNotification.NOTIFICATION_ID,
                TunnelNotification.build(this, profileName, "VPN"),
            )
        }

        activeProfileName = profileName
        bridge.registerProtectCallback(SocketProtector(this))
        startSpeedMonitor()

        if (metaId != null && metaProfileIds != null) {
            // ── Meta profile mode: start all sub-profiles with balancer ──
            activeMetaId = metaId
            activeSubProfileIds = metaProfileIds.toList()
            val strategy = intent?.getIntExtra(EXTRA_BALANCER_STRATEGY, 0) ?: 0
            val socksPort = intent?.getIntExtra(EXTRA_SOCKS_PORT, 0) ?: 0
            tunnelStateManager.onMetaStarted(metaId)

            scope.launch {
                val profiles = metaProfileIds.mapNotNull { repo.getProfileForTunnel(it) }
                if (profiles.isEmpty()) {
                    tunnelStateManager.onMetaStopped(metaId)
                    stopSelf()
                    return@launch
                }

                // Use first profile for TUN interface settings
                val primary = profiles.first()
                val tunFd = buildTunInterface(primary) ?: run {
                    tunnelStateManager.onMetaStopped(metaId)
                    stopSelf()
                    return@launch
                }
                vpnInterface = tunFd

                // Start ALL sub-profile Go instances
                val upstreamAddrs = mutableListOf<String>()
                for (p in profiles) {
                    try {
                        val dir = "${filesDir.absolutePath}/profiles/${p.id}"
                        java.io.File(dir).mkdirs()
                        val cfg = ProfileConfigMapper.toMobileConfig(p)
                        if (p.identityLocked) bridge.setLockedDomains(p.id, p.domains.split(","))
                        bridge.startInstance(p.id, dir, cfg, p.resolversText)
                        tunnelStateManager.onTunnelStarted(p.id)
                        upstreamAddrs.add("${cfg.listenIP}:${cfg.listenPort}")
                        logManager.appendSystem(LogLevel.INFO, "Meta sub-profile started: ${p.name} on ${cfg.listenIP}:${cfg.listenPort}")
                    } catch (e: Exception) {
                        logManager.appendSystem(LogLevel.ERROR, "Failed to start sub-profile ${p.name}: ${e.message}")
                    }
                }

                if (upstreamAddrs.isEmpty()) {
                    tunnelStateManager.onMetaStopped(metaId)
                    stopSelf()
                    return@launch
                }

                // Poll until the first sub-profile SOCKS proxy is reachable (max 10s)
                val firstAddr = upstreamAddrs.first().split(":")
                val firstHost = firstAddr[0]
                val firstPort = firstAddr.getOrNull(1)?.toIntOrNull() ?: 1080
                var ready = false
                var attempts = 0
                while (attempts < 50 && !ready && isActive) {
                    attempts++
                    try {
                        java.net.Socket().use { s ->
                            s.connect(java.net.InetSocketAddress(firstHost, firstPort), 200)
                            ready = true
                        }
                    } catch (e: java.io.IOException) { delay(200) }
                }

                // Start SOCKS balancer that distributes across all sub-profiles
                val balancerAddr = if (upstreamAddrs.size > 1) {
                    val csv = upstreamAddrs.joinToString(",")
                    val bindAddr = if (socksPort > 0) "127.0.0.1:$socksPort" else "127.0.0.1:0"
                    val addr = bridge.startSocksBalancer(bindAddr, csv, strategy)
                    if (addr != null) {
                        logManager.append(LogEntry(level = LogLevel.INFO, timestamp = "system", message = "SOCKS balancer started on $addr (strategy=$strategy, upstreams=${upstreamAddrs.size})"))
                        addr
                    } else {
                        // Fallback to primary if balancer fails
                        upstreamAddrs.first()
                    }
                } else {
                    upstreamAddrs.first()
                }

                // Attach TUN bridge to the balancer (or single upstream)
                if (!isActive) return@launch  // Service was stopped while starting sub-profiles
                try {
                    logManager.append(LogEntry(level = LogLevel.INFO, timestamp = "system", message = "TUN bridge started → $balancerAddr (meta, ${profiles.size} profiles)"))
                    bridge.startTunBridge(tunFd.fd, 1500, balancerAddr)
                    // startTunBridge is now blocking — returns when the bridge exits.
                    // If the stop was not intentional (no performStop called), it's a crash.
                    if (isActive && !intentionalStop) {
                        logManager.appendSystem(LogLevel.ERROR, "TUN bridge (meta) exited unexpectedly — stopping tunnel")
                        activeMetaId?.let { tunnelStateManager.onMetaStopped(it) }
                        performStop()
                    }
                } catch (e: kotlinx.coroutines.CancellationException) {
                    throw e
                } catch (e: Exception) {
                    logManager.appendSystem(LogLevel.ERROR, "TUN bridge error: ${e.message}")
                    activeMetaId?.let { tunnelStateManager.onMetaStopped(it) }
                    performStop()
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

                    val tunFd = buildTunInterface(profile) ?: run {
                        tunnelStateManager.onTunnelStopped(singleProfileId)
                        stopSelf()
                        return@launch
                    }
                    vpnInterface = tunFd

                    val profileDir = "${filesDir.absolutePath}/profiles/${profile.id}"
                    java.io.File(profileDir).mkdirs()
                    logManager.appendSystem(LogLevel.INFO, "VPN service starting: ${profile.name}")

                    val cfg = ProfileConfigMapper.toMobileConfig(profile)
                    if (profile.identityLocked) bridge.setLockedDomains(profile.id, profile.domains.split(","))
                    bridge.startInstance(
                        profileId = profile.id,
                        profileDir = profileDir,
                        config = cfg,
                        resolversText = profile.resolversText,
                    )
                    // Poll until SOCKS proxy is reachable (max 10 s), abort early if stopped
                    val socksAddr = "${cfg.listenIP}:${cfg.listenPort}"
                    var ready = false
                    var attempts = 0
                    while (attempts < 50 && !ready && isActive) {
                        attempts++
                        try {
                            java.net.Socket().use { s ->
                                s.connect(java.net.InetSocketAddress(cfg.listenIP, cfg.listenPort.toInt()), 200)
                                ready = true
                            }
                        } catch (e: java.io.IOException) {
                            delay(200)
                        }
                    }
                    if (!isActive) return@launch  // Service was stopped while waiting
                    if (!ready) {
                        logManager.appendSystem(LogLevel.WARN, "SOCKS proxy not ready after 10s, starting TUN anyway")
                    }
                    logManager.appendSystem(LogLevel.INFO, "TUN bridge started → $socksAddr")
                    bridge.startTunBridge(tunFd.fd, 1500, socksAddr)
                    // startTunBridge is now blocking — returns when the bridge exits.
                    // If the stop was not intentional (no performStop called), it's a crash.
                    if (isActive && !intentionalStop) {
                        logManager.appendSystem(LogLevel.ERROR, "TUN bridge exited unexpectedly — stopping tunnel")
                        tunnelStateManager.onTunnelStopped(singleProfileId)
                        performStop()
                    }                } catch (e: kotlinx.coroutines.CancellationException) {
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

    /**
     * Graceful shutdown: stop the Go bridge and instance in a background coroutine,
     * then call stopSelf() so Android destroys the service cleanly.
     * Called from ACTION_STOP intent — runs *before* onDestroy().
     */
    private fun performStop() {
        intentionalStop = true
        scope.launch {
            try { bridge.stopTunBridge() } catch (_: Exception) {}
            try { bridge.stopSocksBalancer() } catch (_: Exception) {}

            // Stop meta sub-profiles if running
            if (activeMetaId != null) {
                for (subId in activeSubProfileIds) {
                    try { bridge.stopInstance(subId) } catch (_: Exception) {}
                    bridge.clearLockedDomains(subId)
                    tunnelStateManager.onTunnelStopped(subId)
                }
                tunnelStateManager.onMetaStopped(activeMetaId!!)
                activeMetaId = null
                activeSubProfileIds = emptyList()
            }

            // Stop single profile if running
            activeProfileId?.let { pid ->
                try { bridge.stopInstance(pid) } catch (_: Exception) {}
                bridge.clearLockedDomains(pid)
                tunnelStateManager.onTunnelStopped(pid)
                activeProfileId = null
            }

            // Close TUN fd BEFORE stopSelf() so Android removes the VPN icon immediately.
            // If we call stopSelf() first, onDestroy() closes it but the icon lingers.
            vpnInterface?.close()
            vpnInterface = null

            stopForeground(STOP_FOREGROUND_REMOVE)
            stopSelf()
        }
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
                    this@DnsTunnelVpnService,
                    activeProfileName,
                    "VPN",
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
        // Go cleanup is done in performStop() before onDestroy() is reached.
        // vpnInterface is already closed in performStop() — guard against double-close.
        bridge.registerProtectCallback(null)
        if (vpnInterface != null) {
            vpnInterface?.close()
            vpnInterface = null
        }
        scope.cancel()
        super.onDestroy()
    }

    override fun onRevoke() {
        // Called when user disconnects from system VPN settings or quick tile.
        // Must do full cleanup otherwise Go instances stay alive and freeze.
        performStop()
        super.onRevoke()
    }

    private fun buildTunInterface(profile: ProfileEntity): ParcelFileDescriptor? {
        return try {
            val dnsServer = if (profile.localDnsEnabled && profile.localDnsIP.isNotBlank())
                profile.localDnsIP else "8.8.8.8"

            val builder = Builder()
                .setSession("MasterDnsVPN")
                .addAddress("10.89.0.1", 32)
                .addDnsServer(dnsServer)
                .setMtu(1500)
                .addRoute("0.0.0.0", 0)
                .addRoute("::", 0)
                .setBlocking(true)  // gVisor fdbased requires blocking fd

            // Always exclude ourselves to avoid routing loop
            builder.addDisallowedApplication(packageName)

            // Per-app VPN filtering
            val mode = appSelectionPrefs.mode
            val selected = appSelectionPrefs.selectedPackages

            when (mode) {
                AppSelectionPrefs.Mode.INCLUDE -> {
                    // Only selected apps go through VPN.
                    // Always include ourselves implicitly via disallow (above).
                    for (pkg in selected) {
                        if (pkg != packageName) {
                            try { builder.addAllowedApplication(pkg) } catch (_: Exception) {}
                        }
                    }
                    // In INCLUDE mode Android ignores addDisallowedApplication,
                    // so re-add ourselves with addAllowedApplication only if the
                    // list is non-empty — otherwise no apps get the VPN at all.
                    // We need to NOT include packageName in the allowed list so
                    // our own SOCKS proxy traffic never loops back through VPN.
                    // Note: addAllowedApplication implicitly excludes everything
                    // else already — no extra work needed.
                }
                AppSelectionPrefs.Mode.EXCLUDE -> {
                    // All apps EXCEPT selected go through VPN
                    for (pkg in selected) {
                        if (pkg != packageName) {
                            try { builder.addDisallowedApplication(pkg) } catch (_: Exception) {}
                        }
                    }
                }
                AppSelectionPrefs.Mode.ALL -> {
                    // No per-app filter — all traffic goes through VPN
                }
            }

            builder.establish()
        } catch (e: Exception) {
            null
        }
    }
}