package com.masterdnsvpn.hotspot

import android.content.Context
import android.net.wifi.WifiManager
import java.net.NetworkInterface

/**
 * Utility that finds the IP address the Android device is using as a
 * Wi-Fi hotspot AP (soft-AP) interface.
 *
 * Android does not expose a public API for this, so we fall back to
 * scanning network interfaces for known hotspot subnet prefixes.
 * On most AOSP/OEM builds the hotspot interface is named "wlan0",
 * "ap0", "swlan0", or similar, and lives in 192.168.43.x or
 * 192.168.49.x.
 */
object HotspotManager {

    private val HOTSPOT_PREFIXES = listOf("192.168.43.", "192.168.49.", "192.168.1.")

    /**
     * Returns the hotspot AP IP address (e.g. "192.168.43.1") if the
     * device is currently broadcasting a hotspot, or null otherwise.
     */
    fun getHotspotIp(ctx: Context): String? {
        // Primary: try WifiManager AP IP (Android 10+ hides actual API but
        // getConnectionInfo still works for legacy and some OEMs)
        try {
            val wm = ctx.applicationContext
                .getSystemService(Context.WIFI_SERVICE) as WifiManager
            @Suppress("DEPRECATION")
            val info = wm.connectionInfo
            if (info != null) {
                val ip = info.ipAddress
                if (ip != 0) {
                    // connectionInfo gives the STA IP, not AP IP — skip
                }
            }
        } catch (_: Exception) {}

        // Fallback: scan all network interfaces
        try {
            val ifaces = NetworkInterface.getNetworkInterfaces() ?: return null
            for (iface in ifaces.asSequence()) {
                if (!iface.isUp || iface.isLoopback) continue
                for (addr in iface.inetAddresses.asSequence()) {
                    if (addr.isLoopbackAddress) continue
                    val hostAddr = addr.hostAddress ?: continue
                    if (hostAddr.contains(':')) continue // skip IPv6
                    for (prefix in HOTSPOT_PREFIXES) {
                        if (hostAddr.startsWith(prefix)) return hostAddr
                    }
                }
            }
        } catch (_: Exception) {}

        return null
    }

    /**
     * Returns true when a hotspot AP IP is detectable, which is a
     * reasonable proxy for "hotspot is enabled".
     */
    fun isHotspotLikelyEnabled(ctx: Context): Boolean = getHotspotIp(ctx) != null

    /** Default port for the hotspot SOCKS5 relay. */
    const val DEFAULT_HOTSPOT_PORT = 8090
}
