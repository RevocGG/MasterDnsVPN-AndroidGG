package com.masterdnsvpn.bridge

import android.net.VpnService
import com.masterdnsvpn.gomobile.mobile.MobileProtectCallback

/**
 * Bridges Go protect callback to Android VpnService.protect(fd).
 */
class SocketProtector(
    private val vpnService: VpnService,
) : MobileProtectCallback {
    override fun protect(fd: Int): Boolean {
        return try {
            vpnService.protect(fd)
        } catch (_: Exception) {
            false
        }
    }
}