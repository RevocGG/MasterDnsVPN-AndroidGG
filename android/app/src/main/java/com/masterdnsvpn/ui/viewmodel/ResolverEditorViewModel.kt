package com.masterdnsvpn.ui.viewmodel

import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.masterdnsvpn.ui.Screen
import com.masterdnsvpn.profile.ProfileRepository
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import javax.inject.Inject

@HiltViewModel
class ResolverEditorViewModel @Inject constructor(
    savedStateHandle: SavedStateHandle,
    private val repo: ProfileRepository,
) : ViewModel() {

    private val profileId: String = savedStateHandle["profileId"] ?: ""

    // Text bound to the manual paste/edit box — never set from file imports
    private val _manualText = MutableStateFlow("")
    val manualText: StateFlow<String> = _manualText.asStateFlow()

    // Count of resolvers loaded from file (or restored from existing profile save)
    private val _fileResolverCount = MutableStateFlow(0)
    val fileResolverCount: StateFlow<Int> = _fileResolverCount.asStateFlow()

    // Backing store for file/restored resolvers — NOT exposed to the UI text box
    private var fileText = ""

    private val _saved = MutableStateFlow(false)
    val saved: StateFlow<Boolean> = _saved.asStateFlow()

    init {
        if (profileId.isNotBlank() && profileId != Screen.ProfileEdit.NEW) {
            viewModelScope.launch {
                repo.getProfile(profileId)?.let { profile ->
                    val text = profile.resolversText
                    if (text.isNotBlank()) {
                        fileText = text
                        _fileResolverCount.value = text.lines().count { it.isNotBlank() }
                    }
                }
            }
        }
    }

    /** Called when user types / pastes in the manual box. */
    fun updateManual(text: String) {
        _manualText.value = text
    }

    /** Called when user picks a file — stores backing text and shows only the count. */
    fun loadFromFile(text: String) {
        fileText = text
        _fileResolverCount.value = text.lines().count { it.isNotBlank() }
        _manualText.value = ""   // clear paste box so file content never appears there
    }

    /** Discards the file-imported resolvers. */
    fun clearFile() {
        fileText = ""
        _fileResolverCount.value = 0
    }

    /**
     * Moves file content into the manual edit box so the user can edit it directly.
     * Intentionally explicit — large lists may cause the UI to be slow.
     */
    fun loadFileIntoManual() {
        _manualText.value = fileText
        fileText = ""
        _fileResolverCount.value = 0
    }

    fun save() {
        viewModelScope.launch {
            val profile = repo.getProfileForTunnel(profileId) ?: return@launch
            // Manual text takes priority; fall back to file-imported text
            val textToSave = _manualText.value.ifBlank { fileText }
            repo.saveProfile(profile.copy(resolversText = textToSave, updatedAt = System.currentTimeMillis()))
            _saved.value = true
        }
    }
}