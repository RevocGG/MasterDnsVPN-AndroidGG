package com.masterdnsvpn.service

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.core.stringPreferencesKey
import com.masterdnsvpn.profile.AppDatabase
import dagger.hilt.android.AndroidEntryPoint
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import javax.inject.Inject

/**
 * Starts the persistent VPN/Proxy after device boot or app update,
 * if the user had "always-on" enabled.
 *
 * The "reconnect on boot" preference is stored in DataStore under:
 *   KEY_RECONNECT_ON_BOOT → Boolean
 *   KEY_ACTIVE_PROFILE_ID → String (profile UUID)
 *   KEY_TUNNEL_MODE        → "SOCKS5" or "TUN"
 */
@AndroidEntryPoint
class BootReceiver : BroadcastReceiver() {

    companion object {
        val KEY_RECONNECT_ON_BOOT = booleanPreferencesKey("reconnect_on_boot")
        val KEY_ACTIVE_PROFILE_ID = stringPreferencesKey("active_profile_id")
        val KEY_TUNNEL_MODE = stringPreferencesKey("tunnel_mode")
    }

    @Inject lateinit var db: AppDatabase

    override fun onReceive(ctx: Context, intent: Intent) {
        if (intent.action != Intent.ACTION_BOOT_COMPLETED &&
            intent.action != "android.intent.action.MY_PACKAGE_REPLACED"
        ) return

        val appCtx = ctx.applicationContext
        CoroutineScope(Dispatchers.IO).launch {
            val prefs = appCtx.tunnelPrefsDataStore.data.first()
            val reconnect = prefs[KEY_RECONNECT_ON_BOOT] ?: false
            if (!reconnect) return@launch
            val profileId = prefs[KEY_ACTIVE_PROFILE_ID] ?: return@launch
            val mode = prefs[KEY_TUNNEL_MODE] ?: "SOCKS5"
            val profile = db.profileDao().getProfileById(profileId) ?: return@launch

            when (mode) {
                "TUN" -> {
                    // VpnService.prepare() returns null only if permission is already granted.
                    // On boot there is no UI to request missing permission, so skip auto-start
                    // silently rather than launching a service that will silently fail to
                    // establish the TUN interface.
                    if (android.net.VpnService.prepare(appCtx) != null) return@launch
                    val i = Intent(appCtx, DnsTunnelVpnService::class.java).apply {
                        putExtra(DnsTunnelVpnService.EXTRA_PROFILE_ID, profileId)
                        putExtra(DnsTunnelVpnService.EXTRA_PROFILE_NAME, profile.name)
                    }
                    appCtx.startForegroundService(i)
                }
                else -> {
                    val i = Intent(appCtx, DnsTunnelProxyService::class.java).apply {
                        putExtra(DnsTunnelProxyService.EXTRA_PROFILE_ID, profileId)
                        putExtra(DnsTunnelProxyService.EXTRA_PROFILE_NAME, profile.name)
                    }
                    appCtx.startForegroundService(i)
                }
            }
        }
    }
}