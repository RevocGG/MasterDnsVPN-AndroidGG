package com.masterdnsvpn.settings

import android.content.Context
import android.content.SharedPreferences
import com.google.gson.Gson
import com.google.gson.reflect.TypeToken
import dagger.hilt.android.qualifiers.ApplicationContext
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Global resolver-list selection for the Home-screen resolver card.
 *
 * The user may select one or several named resolver lists; when a profile
 * starts, the resolver text of ALL selected lists is merged (deduplicated,
 * first occurrence wins) and used instead of the profile's own resolversText.
 *
 * Stored in the "app_settings" prefs (same file as language + Disable IPv6).
 * An empty selection means "use each profile's own resolver text" — the
 * classic behavior, so nothing changes until the user opts in.
 */
@Singleton
class ResolverSelectionPrefs @Inject constructor(
    @ApplicationContext private val ctx: Context,
) {
    companion object {
        private const val PREFS_NAME = "app_settings"
        private const val KEY_SELECTED = "resolver_selected_ids"

        /** Merge lines from multiple resolver texts, deduplicated, order kept. */
        fun mergeResolverTexts(texts: List<String>): String =
            texts.flatMap { it.lines() }
                .map { it.trim() }
                .filter { it.isNotBlank() && !it.startsWith("#") }
                .distinct()
                .joinToString("\n")
    }

    private val prefs: SharedPreferences =
        ctx.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
    private val gson = Gson()

    /** IDs of the selected resolver lists; empty = profiles use their own. */
    val selectedIds: List<String>
        get() {
            val json = prefs.getString(KEY_SELECTED, null) ?: return emptyList()
            return try {
                gson.fromJson(json, object : TypeToken<List<String>>() {}.type)
                    ?: emptyList()
            } catch (_: Exception) {
                emptyList()
            }
        }

    fun setSelectedIds(ids: List<String>) {
        prefs.edit().putString(KEY_SELECTED, gson.toJson(ids)).apply()
    }

    fun isSelected(id: String): Boolean = selectedIds.contains(id)
}
