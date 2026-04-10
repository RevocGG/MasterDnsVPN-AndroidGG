package com.masterdnsvpn.service

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import androidx.core.app.NotificationCompat
import com.masterdnsvpn.MainActivity
import com.masterdnsvpn.R

object TunnelNotification {

    const val CHANNEL_ID = "masterdnsvpn_tunnel"
    const val NOTIFICATION_ID = 1001       // TUN/VPN service
    const val PROXY_NOTIFICATION_ID = 1002 // SOCKS5 proxy service

    fun createChannel(ctx: Context) {
        val nm = ctx.getSystemService(NotificationManager::class.java)
        if (nm.getNotificationChannel(CHANNEL_ID) != null) return
        val ch = NotificationChannel(
            CHANNEL_ID,
            ctx.getString(R.string.notif_channel_tunnel),
            NotificationManager.IMPORTANCE_LOW,
        )
        ch.setShowBadge(false)
        nm.createNotificationChannel(ch)
    }

    fun build(ctx: Context, profileName: String, mode: String): Notification {
        createChannel(ctx)
        return baseBuilder(ctx, profileName, mode)
            .setContentText(ctx.getString(R.string.notif_connecting))
            .build()
    }

    fun buildWithSpeed(
        ctx: Context,
        profileName: String,
        mode: String,
        upBytesPerSec: Long,
        downBytesPerSec: Long,
    ): Notification {
        createChannel(ctx)
        return baseBuilder(ctx, profileName, mode)
            .setContentText("\u2191 ${formatSpeed(upBytesPerSec)}   \u2193 ${formatSpeed(downBytesPerSec)}")
            .build()
    }

    private fun baseBuilder(ctx: Context, profileName: String, mode: String): NotificationCompat.Builder {
        val contentIntent = PendingIntent.getActivity(
            ctx, 0,
            Intent(ctx, MainActivity::class.java),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
        )

        val disconnectIntent = PendingIntent.getBroadcast(
            ctx, 1,
            Intent(TunnelControlReceiver.ACTION_STOP).apply {
                setPackage(ctx.packageName)
            },
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
        )

        return NotificationCompat.Builder(ctx, CHANNEL_ID)
            .setSmallIcon(R.drawable.ic_notification)
            .setContentTitle("$profileName \u00b7 $mode")
            .setOngoing(true)
            .setContentIntent(contentIntent)
            .addAction(
                android.R.drawable.ic_delete,
                ctx.getString(R.string.notif_action_disconnect),
                disconnectIntent,
            )
    }

    private fun formatSpeed(bytesPerSec: Long): String {
        if (bytesPerSec <= 0) return "0 B/s"
        return when {
            bytesPerSec < 1024 -> "$bytesPerSec B/s"
            bytesPerSec < 1024 * 1024 -> "${bytesPerSec / 1024} KB/s"
            else -> String.format("%.1f MB/s", bytesPerSec / (1024.0 * 1024.0))
        }
    }
}