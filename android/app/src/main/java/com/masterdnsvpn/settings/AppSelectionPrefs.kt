package com.masterdnsvpn.settings

import android.content.Context
import android.content.SharedPreferences
import dagger.hilt.android.qualifiers.ApplicationContext
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Stores which apps should be tunnelled through the VPN.
 *
 * [mode]:
 *  - ALL    → all apps use VPN (default)
 *  - INCLUDE → only selected apps use VPN (allowed list)
 *  - EXCLUDE → all apps EXCEPT selected use VPN (disallowed list)
 *
 * [selectedPackages]: the set of package names chosen by the user.
 */
@Singleton
class AppSelectionPrefs @Inject constructor(
    @ApplicationContext private val ctx: Context,
) {
    companion object {
        private const val PREFS_NAME = "app_selection"
        private const val KEY_MODE = "mode"
        private const val KEY_PACKAGES = "packages"
    }

    enum class Mode { ALL, INCLUDE, EXCLUDE }

    private val prefs: SharedPreferences =
        ctx.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)

    var mode: Mode
        get() = try { Mode.valueOf(prefs.getString(KEY_MODE, Mode.ALL.name)!!) } catch (_: Exception) { Mode.ALL }
        set(value) = prefs.edit().putString(KEY_MODE, value.name).apply()

    var selectedPackages: Set<String>
        get() = prefs.getStringSet(KEY_PACKAGES, emptySet()) ?: emptySet()
        set(value) = prefs.edit().putStringSet(KEY_PACKAGES, value).apply()
}
