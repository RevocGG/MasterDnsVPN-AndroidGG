package com.masterdnsvpn.service

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent

/**
 * Handles notification action: stop all tunnel services.
 */
class TunnelControlReceiver : BroadcastReceiver() {
    companion object {
        const val ACTION_STOP = "com.masterdnsvpn.action.STOP_TUNNEL"
    }

    override fun onReceive(context: Context, intent: Intent?) {
        if (intent?.action != ACTION_STOP) return
        context.stopService(Intent(context, DnsTunnelProxyService::class.java))
        // For VpnService, send ACTION_STOP so it can do graceful Go cleanup
        val vpnStop = Intent(context, DnsTunnelVpnService::class.java).apply {
            action = DnsTunnelVpnService.ACTION_STOP
        }
        context.startService(vpnStop)
    }
}