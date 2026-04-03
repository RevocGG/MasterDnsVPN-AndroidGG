package com.masterdnsvpn.settings

import android.content.Context
import android.content.SharedPreferences
import dagger.hilt.android.qualifiers.ApplicationContext
import javax.inject.Inject
import javax.inject.Singleton

@Singleton
class ProfileBandwidthPrefs @Inject constructor(
    @ApplicationContext ctx: Context,
) {
    private val prefs: SharedPreferences =
        ctx.getSharedPreferences("profile_bandwidth", Context.MODE_PRIVATE)

    /** Get total bytes (up + down) ever used by this profile. */
    fun getTotalBytes(profileId: String): Long =
        prefs.getLong("total_$profileId", 0L)

    fun getUploadBytes(profileId: String): Long =
        prefs.getLong("up_$profileId", 0L)

    fun getDownloadBytes(profileId: String): Long =
        prefs.getLong("down_$profileId", 0L)

    /** Add session bytes to the persisted total. */
    fun addSessionBytes(profileId: String, uploadDelta: Long, downloadDelta: Long) {
        if (uploadDelta <= 0 && downloadDelta <= 0) return
        prefs.edit()
            .putLong("up_$profileId", getUploadBytes(profileId) + uploadDelta)
            .putLong("down_$profileId", getDownloadBytes(profileId) + downloadDelta)
            .putLong("total_$profileId", getTotalBytes(profileId) + uploadDelta + downloadDelta)
            .apply()
    }

    // ── Wire traffic (actual bytes sent/received on the real network by MasterDNS) ──
    // This includes protocol overhead: packet duplication, ARQ retransmissions,
    // DNS encapsulation, encryption headers, etc.
    // Overhead = wire - actual (TUN).

    fun getWireTotalBytes(profileId: String): Long =
        prefs.getLong("wire_total_$profileId", 0L)

    fun getWireUploadBytes(profileId: String): Long =
        prefs.getLong("wire_up_$profileId", 0L)

    fun getWireDownloadBytes(profileId: String): Long =
        prefs.getLong("wire_down_$profileId", 0L)

    fun addWireBytes(profileId: String, uploadDelta: Long, downloadDelta: Long) {
        if (uploadDelta <= 0 && downloadDelta <= 0) return
        prefs.edit()
            .putLong("wire_up_$profileId", getWireUploadBytes(profileId) + uploadDelta)
            .putLong("wire_down_$profileId", getWireDownloadBytes(profileId) + downloadDelta)
            .putLong("wire_total_$profileId", getWireTotalBytes(profileId) + uploadDelta + downloadDelta)
            .apply()
    }

    fun resetProfile(profileId: String) {
        prefs.edit()
            .remove("up_$profileId")
            .remove("down_$profileId")
            .remove("total_$profileId")
            .remove("wire_up_$profileId")
            .remove("wire_down_$profileId")
            .remove("wire_total_$profileId")
            .apply()
    }
}
