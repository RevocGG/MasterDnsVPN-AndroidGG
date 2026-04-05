package com.masterdnsvpn.bridge

import com.masterdnsvpn.gomobile.mobile.Mobile
import com.masterdnsvpn.gomobile.mobile.MobileConfig
import com.masterdnsvpn.gomobile.mobile.MobileLogCallback
import com.masterdnsvpn.gomobile.mobile.MobileProtectCallback
import com.masterdnsvpn.gomobile.mobile.Stats
import com.masterdnsvpn.log.LogEntry
import com.masterdnsvpn.log.LogLevel
import com.masterdnsvpn.log.LogManager
import java.text.SimpleDateFormat
import java.util.Locale
import java.util.TimeZone
import java.util.concurrent.ConcurrentHashMap
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Thin Kotlin wrapper around the gomobile-generated [Mobile] class.
 *
 * Responsibilities:
 *  - Translate [ProfileConfig] → gomobile [MobileConfig]
 *  - Register [LogManager] as the gomobile log callback
 *  - Register [SocketProtector] as the gomobile protect callback
 *  - Offer suspend-friendly lifecycle methods to the service layer
 *
 * The gomobile .aar is loaded automatically when the class is first used.
 */
@Singleton
class GoMobileBridge @Inject constructor(
    private val logManager: LogManager,
) {
    private var logCallbackRegistered = false

    /**
     * Domains to redact from Go runtime log messages — keyed by profileId.
     * Populated for identity-locked profiles so their domain names never
     * appear in the user-visible log screen.
     */
    private val lockedProfileDomains = ConcurrentHashMap<String, Set<String>>()

    /** Register domains that must be redacted from Go logs for [profileId]. */
    fun setLockedDomains(profileId: String, domains: Collection<String>) {
        val filtered = domains.filter { it.isNotBlank() }.toSet()
        if (filtered.isNotEmpty()) lockedProfileDomains[profileId] = filtered
    }

    /** Remove domain redaction for [profileId] (call when profile stops). */
    fun clearLockedDomains(profileId: String) {
        lockedProfileDomains.remove(profileId)
    }

    private fun redact(message: String): String {
        if (lockedProfileDomains.isEmpty()) return message
        var result = message
        for (domains in lockedProfileDomains.values) {
            for (domain in domains) {
                if (domain.isNotBlank() && result.contains(domain, ignoreCase = true)) {
                    result = result.replace(domain, "[redacted]", ignoreCase = true)
                }
            }
        }
        return result
    }

    // UTC parser for Go timestamps: "2006/01/02 15:04:05"
    private val goTimestampFmt = SimpleDateFormat("yyyy/MM/dd HH:mm:ss", Locale.US).apply {
        timeZone = TimeZone.getTimeZone("UTC")
    }

    // ------------------------------------------------------------------
    // Callback wiring
    // ------------------------------------------------------------------

    /**
     * Register the Android log callback.  Safe to call multiple times.
     */
    fun registerLogCallback() {
        if (logCallbackRegistered) return
        Mobile.setLogCallback(object : MobileLogCallback {
            override fun onLog(level: Long, timestamp: String, message: String) {
                val logLevel = when (level.toInt()) {
                    0 -> LogLevel.DEBUG
                    1 -> LogLevel.INFO
                    2 -> LogLevel.WARN
                    3 -> LogLevel.ERROR
                    else -> LogLevel.INFO
                }
                val epochMs = try {
                    goTimestampFmt.parse(timestamp)?.time ?: System.currentTimeMillis()
                } catch (_: Exception) {
                    System.currentTimeMillis()
                }
                logManager.append(LogEntry(level = logLevel, timestamp = timestamp, message = redact(message), epochMs = epochMs))
            }
        })
        logCallbackRegistered = true
        // Emit a diagnostic entry so the log screen shows something even before Go logs arrive
        logManager.appendSystem(LogLevel.INFO, "Log system ready")
    }

    /**
     * Register the VPN socket-protect callback.
     * Call this before [startInstance] when using TUN mode,
     * and clear it after stopping.
     */
    fun registerProtectCallback(protectCallback: MobileProtectCallback?) {
        Mobile.setProtectCallback(protectCallback)
    }

    // ------------------------------------------------------------------
    // Lifecycle
    // ------------------------------------------------------------------

    /**
     * Write config files and start the Go tunnel for [profileId].
     *
     * @param profileId    Unique identifier (Room UUID string).
     * @param profileDir   Absolute path to a writable directory in
     *                     [android.content.Context.getFilesDir].
     * @param config       All settings mapped from the Room ProfileEntity.
     * @param resolversText  Raw resolver list (same format as client_resolvers.txt).
     * @throws RuntimeException propagated from gomobile on failure.
     */
    fun startInstance(
        profileId: String,
        profileDir: String,
        config: MobileConfig,
        resolversText: String,
    ) {
        registerLogCallback()
        config.profileDir = profileDir
        config.resolversInline = resolversText
        logManager.appendSystem(LogLevel.INFO, "Starting tunnel: $profileId")
        Mobile.startInstance(profileId, config)
    }

    /**
     * Stop the tunnel for [profileId].
     */
    fun stopInstance(profileId: String) {
        Mobile.stopInstance(profileId)
    }

    /** Force-stop a single profile immediately. */
    fun forceStopInstance(profileId: String) {
        try { Mobile.stopInstance(profileId) } catch (_: Exception) {}
    }

    /** Force-stop ALL running profiles immediately. */
    fun forceStopAllInstances() {
        try { Mobile.stopAll() } catch (_: Exception) {}
    }

    /** Stop all running profiles (called on service destroy). */
    fun stopAll() {
        Mobile.stopAll()
    }

    // ------------------------------------------------------------------
    // TUN bridge (tun2socks)
    // ------------------------------------------------------------------

    /**
     * Start the tun2socks bridge that forwards all TUN traffic through the
     * SOCKS5 proxy already running via [startInstance].
     *
     * Must be called AFTER [startInstance] so the proxy port is ready.
     *
     * @param tunFd      Raw fd from [android.os.ParcelFileDescriptor.getFd].
     * @param mtu        MTU of the TUN interface (1500 is the default).
     * @param listenAddr "host:port" of the running SOCKS5 proxy, e.g. "127.0.0.1:1080".
     */
    fun startTunBridge(tunFd: Int, mtu: Int, listenAddr: String) {
        Mobile.startTunBridge(tunFd, mtu, listenAddr)
    }

    /** Stop the tun2socks bridge. Safe to call if the bridge was never started. */
    fun stopTunBridge() {
        try {
            Mobile.stopTunBridge()
        } catch (_: Exception) {}
    }

    /** Returns true if the profile tunnel is active. */
    fun isRunning(profileId: String): Boolean = Mobile.isRunning(profileId)

    /** Returns a live stats snapshot, or null if not running. */
    fun getStats(profileId: String): Stats? {
        return try {
            Mobile.getStats(profileId)
        } catch (_: Exception) {
            null
        }
    }

    /** Returns the last error string from the Go layer, or null. */
    fun getLastError(profileId: String): String? {
        val msg = Mobile.getLastError(profileId)
        return msg.takeIf { it.isNotEmpty() }
    }

    // ------------------------------------------------------------------
    // Default config factory
    // ------------------------------------------------------------------

    /** Returns a [MobileConfig] populated with the same defaults as the desktop client. */
    fun newDefaultConfig(): MobileConfig = Mobile.newDefaultConfig()

    /** Returns current TUN bridge bandwidth (upload, download) in bytes. */
    fun getBandwidth(): Pair<Long, Long> {
        return try {
            val stats = Mobile.getBandwidth()
            Pair(stats.uploadBytes, stats.downloadBytes)
        } catch (_: Exception) {
            Pair(0L, 0L)
        }
    }

    // ------------------------------------------------------------------
    // SOCKS Load Balancer
    // ------------------------------------------------------------------

    /**
     * Start a SOCKS5 load balancer that distributes connections across
     * multiple upstream SOCKS proxies.
     *
     * @param listenAddr   "host:port" to listen on, e.g. "127.0.0.1:10800"
     * @param upstreamCSV  comma-separated upstream "host:port" addresses
     * @param strategy     balancing strategy (0=RR, 1=Random, 2=RoundRobin, 3=LeastConn, 4=LowestLatency)
     * @return the actual listen address
     */
    fun startSocksBalancer(listenAddr: String, upstreamCSV: String, strategy: Int): String? {
        return try {
            Mobile.startSocksBalancer(listenAddr, upstreamCSV, strategy.toLong())
        } catch (e: Exception) {
            logManager.appendSystem(LogLevel.ERROR, "Balancer start failed: ${e.message}")
            null
        }
    }

    /** Stop the SOCKS5 load balancer. */
    fun stopSocksBalancer() {
        try { Mobile.stopSocksBalancer() } catch (_: Exception) {}
    }

    /** Returns true if the SOCKS5 balancer is active. */
    fun isSocksBalancerRunning(): Boolean {
        return try { Mobile.isSocksBalancerRunning() } catch (_: Exception) { false }
    }

    /** Returns true if the TUN bridge (tun2socks) is currently running. */
    fun isTunBridgeRunning(): Boolean {
        return try { Mobile.isTunBridgeRunning() } catch (_: Exception) { false }
    }

    /** Hot-swap the upstream list without restarting the balancer. */
    fun updateBalancerUpstreams(upstreamCSV: String) {
        try { Mobile.updateBalancerUpstreams(upstreamCSV) } catch (_: Exception) {}
    }

    // ------------------------------------------------------------------
    // Hotspot proxy
    // ------------------------------------------------------------------

    /**
     * Start a transparent SOCKS5 relay on 0.0.0.0:[listenPort] that
     * forwards connections to [upstreamSocksAddr].
     * Devices on the Wi-Fi hotspot can use <hotspot-IP>:<listenPort> as proxy.
     *
     * @return the listening address, or null on failure.
     */
    fun startHotspotProxy(listenPort: Int, upstreamSocksAddr: String): String? {
        return try {
            Mobile.startHotspotProxy(listenPort, upstreamSocksAddr)
        } catch (e: Exception) {
            logManager.appendSystem(LogLevel.ERROR, "startHotspotProxy failed: ${e.message}")
            null
        }
    }

    /** Stop the hotspot relay. */
    fun stopHotspotProxy() {
        try { Mobile.stopHotspotProxy() } catch (_: Exception) {}
    }

    /** Returns true if the hotspot relay is active. */
    fun isHotspotProxyRunning(): Boolean {
        return try { Mobile.isHotspotProxyRunning() } catch (_: Exception) { false }
    }

    /** Returns the port the PAC file HTTP server is listening on (0 if not running). */
    fun getHotspotPacPort(): Int {
        return try { Mobile.getHotspotPacPort().toInt() } catch (_: Exception) { 0 }
    }

    /**
     * Sets the SOCKS5 address embedded in the PAC file served to hotspot clients.
     * Call this after discovering the hotspot AP IP, format "ip:port".
     */
    fun setHotspotPacSocksAddr(addr: String) {
        try { Mobile.setHotspotPacSocksAddr(addr) } catch (_: Exception) {}
    }
}