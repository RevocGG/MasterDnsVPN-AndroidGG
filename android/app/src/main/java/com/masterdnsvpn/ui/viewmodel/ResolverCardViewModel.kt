package com.masterdnsvpn.ui.viewmodel

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.masterdnsvpn.gomobile.mobile.Mobile
import com.masterdnsvpn.profile.BEST_MATCH_LIST_ID
import com.masterdnsvpn.profile.ResolverListEntity
import com.masterdnsvpn.profile.ProfileRepository
import com.masterdnsvpn.settings.ResolverSelectionPrefs
import com.masterdnsvpn.service.TunnelStateManager
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch
import java.io.File
import java.util.UUID
import javax.inject.Inject

/** One row of the Best Match snapshot exposed to the UI. */
data class BestMatchRow(val resolver: String, val score: Int, val rttMs: Double = 0.0)

data class ResolverCardState(
    val lists: List<ResolverListEntity> = emptyList(),
    val selectedIds: Set<String> = emptySet(),
    val bestMatchRows: List<BestMatchRow> = emptyList(),
    val bestMatchStale: Boolean = true,
)

/**
 * Drives the Home-screen resolver card: named lists CRUD, selection
 * (single or multi), export and the auto-updating "Best Match" list.
 */
@HiltViewModel
class ResolverCardViewModel @Inject constructor(
    private val repo: ProfileRepository,
    private val selectionPrefs: ResolverSelectionPrefs,
    private val tunnelStateManager: TunnelStateManager,
    @dagger.hilt.android.qualifiers.ApplicationContext private val appCtx: android.content.Context,
) : ViewModel() {

    private val _selection = MutableStateFlow(selectionPrefs.selectedIds.toSet())
    val selection: StateFlow<Set<String>> = _selection.asStateFlow()

    private val _bestMatchRows = MutableStateFlow<List<BestMatchRow>>(emptyList())
    private val _bestMatchStale = MutableStateFlow(true)

    val state: StateFlow<ResolverCardState> = combine(
        repo.allResolverLists(),
        _selection,
        _bestMatchRows,
        _bestMatchStale,
    ) { lists, sel, rows, stale ->
        ResolverCardState(lists, sel, rows, stale)
    }.stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), ResolverCardState())

    init {
        // Keep a Best Match row in Room so the card has a stable entry even
        // before the first run produced the file.
        viewModelScope.launch {
            if (repo.getResolverList(BEST_MATCH_LIST_ID) == null) {
                repo.saveResolverList(
                    ResolverListEntity(
                        id = BEST_MATCH_LIST_ID,
                        name = "Best Match",
                        resolversText = "",
                        isBestMatch = true,
                        createdAt = System.currentTimeMillis(),
                        updatedAt = System.currentTimeMillis(),
                    )
                )
            }
        }
        // Mirror the persisted Best Match file into Room at startup so the
        // card shows the last-known-best list even before the first run.
        viewModelScope.launch {
            mirrorBestMatchFileIntoRoom()
        }
        // Refresh Best Match from the engine every 15s while any tunnel runs,
        // persist the file and mirror it into Room (the auto-update the card
        // promises). When nothing runs, the loop idles cheaply.
        viewModelScope.launch {
            while (true) {
                val running = tunnelStateManager.runningProfileIds.value +
                    tunnelStateManager.runningMetaIds.value
                if (running.isNotEmpty()) {
                    updateAndMirrorBestMatch()
                }
                delay(15_000)
            }
        }
    }

    fun toggleSelect(id: String) {
        val cur = _selection.value.toMutableSet()
        if (!cur.add(id)) cur.remove(id)
        _selection.value = cur
        selectionPrefs.setSelectedIds(cur.toList())
    }

    fun createList(name: String, resolversText: String, onDone: (String) -> Unit = {}) {
        viewModelScope.launch {
            val id = UUID.randomUUID().toString()
            repo.saveResolverList(
                ResolverListEntity(
                    id = id,
                    name = name.ifBlank { "Resolver" },
                    resolversText = resolversText,
                    createdAt = System.currentTimeMillis(),
                    updatedAt = System.currentTimeMillis(),
                )
            )
            onDone(id)
        }
    }

    fun updateList(id: String, name: String, resolversText: String) {
        viewModelScope.launch {
            val existing = repo.getResolverList(id) ?: return@launch
            repo.saveResolverList(
                existing.copy(
                    name = name.ifBlank { existing.name },
                    resolversText = resolversText,
                    updatedAt = System.currentTimeMillis(),
                )
            )
        }
    }

    fun deleteList(id: String) {
        viewModelScope.launch {
            repo.deleteResolverList(id)
            if (_selection.value.contains(id)) {
                val cur = _selection.value - id
                _selection.value = cur
                selectionPrefs.setSelectedIds(cur.toList())
            }
        }
    }

    /**
     * One Best Match refresh cycle: pull the ranked scored snapshot from Go
     * (which also persists the shared file), update the visible score rows
     * and mirror the resolver text into the Room list.
     */
    private suspend fun updateAndMirrorBestMatch() {
        try {
            val csv = Mobile.updateBestMatchGlobal(appCtx.filesDir.absolutePath)
            val rows = mutableListOf<BestMatchRow>()
            for (line in csv.lines()) {
                val p = line.split(',')
                if (p.size < 2) continue
                val resolver = p[0].trim()
                if (resolver.isEmpty()) continue
                val score = p[1].toIntOrNull() ?: continue
                if (score <= 0) continue
                rows.add(BestMatchRow(resolver, score))
            }
            if (rows.isNotEmpty()) {
                _bestMatchRows.value = rows
                _bestMatchStale.value = false
            }
        } catch (_: Exception) { /* engine not loaded */ }
        mirrorBestMatchFileIntoRoom()
    }

    /** Force-refresh now: re-run the aggregation + persist + mirror cycle. */
    fun forceRefreshBestMatch() {
        viewModelScope.launch {
            updateAndMirrorBestMatch()
        }
    }

    /** Copy the on-disk shared Best Match file into the Room row shown in the card. */
    fun mirrorBestMatchFileIntoRoom() {
        viewModelScope.launch {
            val text = try {
                Mobile.readBestMatchGlobalText(appCtx.filesDir.absolutePath)
            } catch (_: Exception) { "" }
            val existing = repo.getResolverList(BEST_MATCH_LIST_ID) ?: return@launch
            if (text.isNotBlank() && text != existing.resolversText) {
                repo.saveResolverList(
                    existing.copy(
                        resolversText = text,
                        updatedAt = System.currentTimeMillis(),
                    )
                )
            }
        }
    }
}
