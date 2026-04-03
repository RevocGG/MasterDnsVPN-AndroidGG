package com.masterdnsvpn.ui.viewmodel

import android.app.Application
import android.content.pm.ApplicationInfo
import android.content.pm.PackageManager
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import com.masterdnsvpn.settings.AppSelectionPrefs
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
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
)

@HiltViewModel
class SettingsViewModel @Inject constructor(
    application: Application,
    private val prefs: AppSelectionPrefs,
) : AndroidViewModel(application) {

    private val _state = MutableStateFlow(SettingsUiState())
    val state: StateFlow<SettingsUiState> = _state

    // All installed apps (loaded once)
    private var allApps: List<Pair<String, String>> = emptyList()
    private var systemPackages: Set<String> = emptySet()
    private var selectedSet: MutableSet<String> = mutableSetOf()

    init {
        // Load on IO, then mutate shared state back on Main to avoid race conditions.
        // If toggleApp/selectAll is called before loading finishes it won't overwrite prefs.
        viewModelScope.launch {
            val pm = application.packageManager
            val (apps, system) = withContext(Dispatchers.IO) {
                val installed = pm.getInstalledApplications(PackageManager.GET_META_DATA)
                val a = installed.map { info ->
                    info.packageName to (info.loadLabel(pm).toString())
                }.sortedBy { it.second.lowercase() }
                val s = installed
                    .filter { it.flags and ApplicationInfo.FLAG_SYSTEM != 0 }
                    .map { it.packageName }
                    .toSet()
                a to s
            }
            // Back on Main — safe to mutate allApps / systemPackages / selectedSet
            allApps = apps
            systemPackages = system
            selectedSet = prefs.selectedPackages.toMutableSet()
            rebuildList()
        }
    }

    fun setMode(mode: AppSelectionPrefs.Mode) {
        prefs.mode = mode
        _state.value = _state.value.copy(mode = mode)
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
        prefs.selectedPackages = selectedSet.toSet()
        rebuildList()
    }

    fun selectAll() {
        val visible = getVisiblePackages()
        selectedSet.addAll(visible)
        prefs.selectedPackages = selectedSet.toSet()
        rebuildList()
    }

    fun deselectAll() {
        val visible = getVisiblePackages()
        selectedSet.removeAll(visible)
        prefs.selectedPackages = selectedSet.toSet()
        rebuildList()
    }

    private fun getVisiblePackages(): Set<String> {
        val s = _state.value
        return allApps
            .filter { (pkg, label) ->
                val isSystem = pkg in systemPackages
                (s.showSystemApps || !isSystem) &&
                    (s.searchQuery.isBlank() || label.contains(s.searchQuery, ignoreCase = true) || pkg.contains(s.searchQuery, ignoreCase = true))
            }
            .map { it.first }
            .toSet()
    }

    private fun rebuildList() {
        val s = _state.value
        val filtered = allApps
            .filter { (pkg, label) ->
                val isSystem = pkg in systemPackages
                (s.showSystemApps || !isSystem) &&
                    (s.searchQuery.isBlank() || label.contains(s.searchQuery, ignoreCase = true) || pkg.contains(s.searchQuery, ignoreCase = true))
            }
            .map { (pkg, label) ->
                AppItem(
                    packageName = pkg,
                    label = label,
                    isSystem = pkg in systemPackages,
                    selected = pkg in selectedSet,
                )
            }
        _state.value = s.copy(
            mode = prefs.mode,
            apps = filtered,
            loading = false,
        )
    }
}
