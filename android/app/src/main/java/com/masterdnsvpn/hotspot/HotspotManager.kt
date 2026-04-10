package com.masterdnsvpn.hotspot

import android.content.Context
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

    // Well-known hotspot AP interface names used by major Android OEMs.
    // Checked before IP-prefix scanning because interface name is more reliable
    // than subnet — home routers also live on 192.168.1.x.
    private val HOTSPOT_IFACE_NAMES = setOf(
        "ap0",       // Qualcomm (Pixel, many brands)
        "swlan0",    // Samsung
        "wlan1",     // Some MediaTek devices
        "softap0",   // AOSP generic
        "wlan0",     // AOSP single-radio AP mode
    )

    // IP prefixes used exclusively (or predominantly) for Android soft-AP.
    // 192.168.1.x intentionally excluded — too broad, matches home routers.
    private val HOTSPOT_PREFIXES = listOf(
        "192.168.43.",   // AOSP default since Android 2.x
        "192.168.49.",   // Pixel / newer AOSP
        "192.168.100.",  // Some Samsung / Xiaomi
        "10.0.0.",       // Xiaomi hotspot default
    )

    /**
     * Returns the hotspot AP IP address (e.g. "192.168.43.1") if the
     * device is currently broadcasting a hotspot, or null otherwise.
     *
     * Strategy:
     *  1. Scan by known hotspot interface names — most reliable.
     *  2. Fall back to subnet-prefix scanning for unlisted OEM names.
     */
    fun getHotspotIp(ctx: Context): String? {
        try {
            val ifaces = NetworkInterface.getNetworkInterfaces() ?: return null
            val all = ifaces.toList().filter { it.isUp && !it.isLoopback }

            // Pass 1: match by interface name
            for (iface in all) {
                val name = iface.name.lowercase()
                if (name in HOTSPOT_IFACE_NAMES) {
                    val ip = iface.inetAddresses.asSequence()
                        .firstOrNull { !it.isLoopbackAddress && !it.hostAddress.contains(':') }
                        ?.hostAddress
                    if (ip != null) return ip
                }
            }

            // Pass 2: match by IP prefix
            for (iface in all) {
                for (addr in iface.inetAddresses.asSequence()) {
                    if (addr.isLoopbackAddress) continue
                    val hostAddr = addr.hostAddress ?: continue
                    if (hostAddr.contains(':')) continue
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
