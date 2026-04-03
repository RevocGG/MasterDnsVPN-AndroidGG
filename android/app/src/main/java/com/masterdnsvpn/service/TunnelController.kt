package com.masterdnsvpn.service

import android.content.Context
import android.content.Intent
import android.net.VpnService
import androidx.activity.result.ActivityResultLauncher
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Coordinates starting and stopping the tunnel services.
 * Used by ViewModels; never directly by UI composables.
 */
@Singleton
class TunnelController @Inject constructor() {

    /**
     * Start SOCKS5 proxy mode for [profileId].
     * The service handles its own foreground notification.
     */
    fun startProxy(ctx: Context, profileId: String, profileName: String) {
        val intent = Intent(ctx, DnsTunnelProxyService::class.java).apply {
            putExtra(DnsTunnelProxyService.EXTRA_PROFILE_ID, profileId)
            putExtra(DnsTunnelProxyService.EXTRA_PROFILE_NAME, profileName)
        }
        ctx.startForegroundService(intent)
    }

    /**
     * Start TUN/VPN mode for [profileId].
     *
     * The caller MUST first check [VpnService.prepare] and request permission
     * via [vpnPermissionLauncher] if needed.  This function is only called
     * after permission is granted.
     */
    fun startVpn(ctx: Context, profileId: String, profileName: String) {
        val intent = Intent(ctx, DnsTunnelVpnService::class.java).apply {
            putExtra(DnsTunnelVpnService.EXTRA_PROFILE_ID, profileId)
            putExtra(DnsTunnelVpnService.EXTRA_PROFILE_NAME, profileName)
        }
        ctx.startForegroundService(intent)
    }

    /** Stop the SOCKS5 proxy service. */
    fun stopProxy(ctx: Context) {
        ctx.stopService(Intent(ctx, DnsTunnelProxyService::class.java))
    }

    /** Stop the VPN service. */
    fun stopVpn(ctx: Context) {
        // Send ACTION_STOP so the service can do graceful Go cleanup before stopping.
        // stopService() alone is not reliable for foreground VpnService.
        val intent = Intent(ctx, DnsTunnelVpnService::class.java).apply {
            action = DnsTunnelVpnService.ACTION_STOP
        }
        ctx.startService(intent)
    }

    /** Stop whichever service is running. */
    fun stopAll(ctx: Context) {
        stopProxy(ctx)
        stopVpn(ctx)
    }

    // ------------------------------------------------------------------
    // Meta profile lifecycle
    // ------------------------------------------------------------------

    /**
     * Start a meta profile in VPN/TUN mode.
     * The VPN service will start all sub-profiles inside the same Go runtime.
     */
    fun startMetaVpn(ctx: Context, metaId: String, metaName: String, profileIds: List<String>, strategy: Int = 0, socksPort: Int = 0) {
        val intent = Intent(ctx, DnsTunnelVpnService::class.java).apply {
            putExtra(DnsTunnelVpnService.EXTRA_META_ID, metaId)
            putExtra(DnsTunnelVpnService.EXTRA_PROFILE_NAME, metaName)
            putStringArrayListExtra(DnsTunnelVpnService.EXTRA_META_PROFILE_IDS, ArrayList(profileIds))
            putExtra(DnsTunnelVpnService.EXTRA_BALANCER_STRATEGY, strategy)
            putExtra(DnsTunnelVpnService.EXTRA_SOCKS_PORT, socksPort)
        }
        ctx.startForegroundService(intent)
    }

    /**
     * Start a meta profile in SOCKS proxy mode.
     * The proxy service will start all sub-profiles.
     */
    fun startMetaProxy(ctx: Context, metaId: String, metaName: String, profileIds: List<String>, strategy: Int = 0, socksPort: Int = 0) {
        val intent = Intent(ctx, DnsTunnelProxyService::class.java).apply {
            putExtra(DnsTunnelProxyService.EXTRA_META_ID, metaId)
            putExtra(DnsTunnelProxyService.EXTRA_PROFILE_NAME, metaName)
            putStringArrayListExtra(DnsTunnelProxyService.EXTRA_META_PROFILE_IDS, ArrayList(profileIds))
            putExtra(DnsTunnelProxyService.EXTRA_BALANCER_STRATEGY, strategy)
            putExtra(DnsTunnelProxyService.EXTRA_SOCKS_PORT, socksPort)
        }
        ctx.startForegroundService(intent)
    }

    /** Stop a running meta profile (stops whichever service is running). */
    fun stopMetaProfiles(ctx: Context, metaId: String) {
        stopAll(ctx)
    }

    /**
     * Returns a [VpnService.prepare] Intent if VPN permission is needed,
     * or null if already granted.
     */
    fun prepareVpnIntent(ctx: Context): Intent? = VpnService.prepare(ctx)
}