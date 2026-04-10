package com.masterdnsvpn.ui.viewmodel

import android.app.Application
import android.content.pm.ApplicationInfo
import android.content.pm.PackageManager
import android.graphics.Bitmap
import androidx.core.graphics.drawable.toBitmap
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import com.masterdnsvpn.settings.AppSelectionPrefs
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.util.concurrent.ConcurrentHashMap
import javax.inject.Inject

data class AppItem(
    val packageName: String,
    val label: String,
    val isSystem: Boolean,
    val selected: Boolean,
)

data class SettingsUiState(
    val mode: AppSelectionPrefs.Mode = AppSelectionPrefs.Mode.ALL,
    val apps: List<AppItem> = emptyList(),
    val searchQuery: String = "",
    val showSystemApps: Boolean = false,
    val loading: Boolean = true,
    val savedToPrefs: Boolean = true,
)

@HiltViewModel
class SettingsViewModel @Inject constructor(
    application: Application,
    private val appSelectionPrefs: AppSelectionPrefs,
) : AndroidViewModel(application) {

    private val _state = MutableStateFlow(SettingsUiState())
    val state: StateFlow<SettingsUiState> = _state

    private var allApps: List<Pair<String, String>> = emptyList()
    private var systemPackages: Set<String> = emptySet()
    private var selectedSet: MutableSet<String> = mutableSetOf()

    /** Cached app icons keyed by package name. Null value = load attempted but failed. */
    val iconCache = ConcurrentHashMap<String, Bitmap?>()

    init {
        viewModelScope.launch {
            val pm = application.packageManager
            val (apps, system) = withContext(Dispatchers.IO) {
                val selfPkg = application.packageName

                // Use launcher-intent query instead of getInstalledApplications to get
                // all user-visible apps regardless of FLAG_SYSTEM.
                // This ensures preinstalled apps like Chrome (com.android.chrome) that
                // have FLAG_SYSTEM but are user-launchable still appear in the list.
                val launcherIntent = android.content.Intent(android.content.Intent.ACTION_MAIN).apply {
                    addCategory(android.content.Intent.CATEGORY_LAUNCHER)
                }
                @Suppress("DEPRECATION")
                val resolved = pm.queryIntentActivities(launcherIntent, 0)
                    .mapNotNull { it.activityInfo?.applicationInfo }
                    .distinctBy { it.packageName }
                    .filter { it.packageName != selfPkg }

                val a = resolved
                    .map { info -> info.packageName to (info.loadLabel(pm).toString()) }
                    .sortedBy { it.second.lowercase() }

                // FLAG_SYSTEM is kept only as a display hint (shown in italic or badge).
                val s = resolved
                    .filter { it.flags and ApplicationInfo.FLAG_SYSTEM != 0 }
                    .map { it.packageName }
                    .toSet()

                a to s
            }
            allApps = apps
            systemPackages = system

            // Load per-app settings from global AppSelectionPrefs (what the VPN service reads).
            withContext(Dispatchers.IO) {
                selectedSet = appSelectionPrefs.selectedPackages.toMutableSet()
                _state.update { it.copy(mode = appSelectionPrefs.mode) }
            }

            rebuildList()

            viewModelScope.launch(Dispatchers.IO) {
                apps.forEach { (pkg, _) ->
                    if (!iconCache.containsKey(pkg)) {
                        val bmp = try {
                            pm.getApplicationIcon(pkg).toBitmap(40, 40)
                        } catch (_: Exception) { null }
                        iconCache[pkg] = bmp
                    }
                }
            }
        }
    }

    fun setMode(mode: AppSelectionPrefs.Mode) {
        _state.update { it.copy(mode = mode, savedToPrefs = false) }
    }

    fun setSearchQuery(query: String) {
        _state.value = _state.value.copy(searchQuery = query)
        rebuildList()
    }

    fun toggleSystemApps(show: Boolean) {
        _state.value = _state.value.copy(showSystemApps = show)
        rebuildList()
    }

    fun toggleApp(packageName: String) {
        if (selectedSet.contains(packageName)) {
            selectedSet.remove(packageName)
        } else {
            selectedSet.add(packageName)
        }
        _state.update { it.copy(savedToPrefs = false) }
        rebuildList()
    }

    fun selectAll() {
        val visible = getVisiblePackages()
        selectedSet.addAll(visible)
        _state.update { it.copy(savedToPrefs = false) }
        rebuildList()
    }

    fun deselectAll() {
        val visible = getVisiblePackages()
        selectedSet.removeAll(visible)
        _state.update { it.copy(savedToPrefs = false) }
        rebuildList()
    }

    /** Save per-app VPN settings to global AppSelectionPrefs (read by the VPN service). */
    fun save() {
        val mode = _state.value.mode
        val packages = selectedSet.toSet()
        viewModelScope.launch(Dispatchers.IO) {
            appSelectionPrefs.saveAll(mode, packages)
            _state.update { it.copy(savedToPrefs = true) }
        }
    }

    private fun getVisiblePackages(): Set<String> {
        val s = _state.value
        val q = s.searchQuery.trim()
        return allApps
            .filter { (pkg, label) ->
                val isSystem = pkg in systemPackages
                (s.showSystemApps || !isSystem) &&
                    (q.isBlank() || label.contains(q, ignoreCase = true) || pkg.contains(q, ignoreCase = true))
            }
            .map { it.first }
            .toSet()
    }

    private fun rebuildList() {
        val s = _state.value
        val q = s.searchQuery.trim()
        val filtered = allApps
            .filter { (pkg, label) ->
                val isSystem = pkg in systemPackages
                (s.showSystemApps || !isSystem) &&
                    (q.isBlank() || label.contains(q, ignoreCase = true) || pkg.contains(q, ignoreCase = true))
            }
            .map { (pkg, label) ->
                AppItem(
                    packageName = pkg,
                    label = label,
                    isSystem = pkg in systemPackages,
                    selected = pkg in selectedSet,
                )
            }
        _state.value = s.copy(apps = filtered, loading = false)
    }
}
