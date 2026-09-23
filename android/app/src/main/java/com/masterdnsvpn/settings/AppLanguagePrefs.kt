package com.masterdnsvpn.settings

import android.content.Context
import android.content.SharedPreferences
import android.content.res.Configuration
import android.os.LocaleList
import androidx.appcompat.app.AppCompatDelegate
import androidx.core.os.LocaleListCompat
import dagger.hilt.android.qualifiers.ApplicationContext
import java.util.Locale
import javax.inject.Inject
import javax.inject.Singleton

/**
 * App language selection ("Language" setting).
 *
 * The app uses [androidx.activity.ComponentActivity] (Compose-only, no
 * AppCompat theme), where `AppCompatDelegate.setApplicationLocales` does NOT
 * apply locales by itself. Instead the activity:
 *  1. overrides its base context in [com.masterdnsvpn.MainActivity.attachBaseContext]
 *     with the saved locale (so the very first frame is localized), and
 *  2. calls [android.app.Activity.recreate] right after the user picks a new
 *     language so every screen re-reads localized resources.
 *
 * Supported languages:
 *  - en  English (default)
 *  - fa  Persian (فارسی) — RTL
 *  - zh  Chinese (中文)
 *  - ru  Russian (Русский)
 */
@Singleton
class AppLanguagePrefs @Inject constructor(
    @ApplicationContext private val ctx: Context,
) {
    companion object {
        private const val PREFS_NAME = "app_settings"
        private const val KEY_LANGUAGE = "app_language"
        private const val KEY_DISABLE_IPV6 = "disable_ipv6"

        /** Language tags offered in Settings, in display order. English first (default). */
        val SUPPORTED = listOf("en", "fa", "zh", "ru")

        val DEFAULT = "en"

        /** Default state of the global "Disable IPv6" toggle (ON). */
        const val DISABLE_IPV6_DEFAULT = true
    }

    enum class Lang(val tag: String, val nativeName: String) {
        EN("en", "English"),
        FA("fa", "فارسی"),
        ZH("zh", "中文"),
        RU("ru", "Русский");

        companion object {
            fun fromTag(tag: String): Lang =
                entries.firstOrNull { it.tag == tag.substringBefore('-') } ?: EN
        }
    }

    private val prefs: SharedPreferences =
        ctx.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)

    /** The persisted language tag; [DEFAULT] when never set. */
    var tag: String
        get() = prefs.getString(KEY_LANGUAGE, DEFAULT) ?: DEFAULT
        private set(value) = prefs.edit().putString(KEY_LANGUAGE, value).apply()

    /** The active language as a [Lang] enum (defaults to English). */
    val lang: Lang get() = Lang.fromTag(tag)

    /** True when the active language lays out right-to-left (Persian). */
    val isRtl: Boolean get() = lang == Lang.FA

    // ── Global "Disable IPv6" toggle (app Settings, applies to the TUN bridge)
    // ON (default): the DNS tunnel has no IPv6 egress, so AAAA answers are
    // stripped and IPv6 CONNECTs are fast-RST'd — apps like YouTube fall back
    // to IPv4 instantly instead of timing out with SOCKS5 code 3.

    var disableIPv6: Boolean
        get() = prefs.getBoolean(KEY_DISABLE_IPV6, DISABLE_IPV6_DEFAULT)
        set(value) = prefs.edit().putBoolean(KEY_DISABLE_IPV6, value).apply()

    /** Push the saved toggle into the Go TUN bridge (call before StartTunBridge). */
    fun applyDisableIPv6ToBridge() {
        try {
            com.masterdnsvpn.gomobile.mobile.Mobile.setTunDisableIPv6(disableIPv6)
        } catch (_: Exception) {
            // Bridge not loaded yet (non-TUN flows) — StartTunBridge callers apply it.
        }
    }

    /**
     * Wrap an activity base context with the saved locale. Called from
     * [com.masterdnsvpn.MainActivity.attachBaseContext] before onCreate.
     */
    fun wrapContext(base: Context): Context {
        val locale = Locale.forLanguageTag(tag)
        Locale.setDefault(locale)

        val config = Configuration(base.resources.configuration)
        config.setLocale(locale)
        config.setLocales(LocaleList(locale))
        // setLocales does NOT derive layout direction — set it explicitly so
        // Persian (fa) lays out right-to-left across every Compose screen.
        config.setLayoutDirection(locale)
        return base.createConfigurationContext(config)
    }

    /**
     * Change the app language. Returns true when the tag actually changed
     * (the caller should then call `recreate()` on the activity).
     */
    fun setLanguage(newTag: String): Boolean {
        val normalized = Lang.fromTag(newTag).tag
        if (normalized == tag) return false
        tag = normalized

        // Keep androidx per-app locales in sync so system UI (permission
        // dialogs, share sheets) uses the same language where supported.
        try {
            AppCompatDelegate.setApplicationLocales(
                LocaleListCompat.forLanguageTags(normalized)
            )
        } catch (_: Exception) {
            // Not fatal — in-app resources are already handled by wrapContext.
        }
        return true
    }
}
