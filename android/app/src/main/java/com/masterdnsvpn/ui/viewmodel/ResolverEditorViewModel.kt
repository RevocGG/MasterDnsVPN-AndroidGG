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

    private val _resolversText = MutableStateFlow("")
    val resolversText: StateFlow<String> = _resolversText.asStateFlow()

    private val _saved = MutableStateFlow(false)
    val saved: StateFlow<Boolean> = _saved.asStateFlow()

    init {
        if (profileId.isNotBlank() && profileId != Screen.ProfileEdit.NEW) {
            viewModelScope.launch {
                repo.getProfile(profileId)?.let { _resolversText.value = it.resolversText }
            }
        }
    }

    fun updateText(text: String) {
        _resolversText.value = text
    }

    fun save() {
        viewModelScope.launch {
            val profile = repo.getProfileForTunnel(profileId) ?: return@launch
            repo.saveProfile(profile.copy(resolversText = _resolversText.value, updatedAt = System.currentTimeMillis()))
            _saved.value = true
        }
    }
}