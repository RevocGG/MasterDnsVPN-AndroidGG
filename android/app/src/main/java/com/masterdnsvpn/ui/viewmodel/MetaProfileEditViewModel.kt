package com.masterdnsvpn.ui.viewmodel

import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.masterdnsvpn.profile.MetaProfileEntity
import com.masterdnsvpn.profile.ProfileEntity
import com.masterdnsvpn.profile.ProfileRepository
import com.masterdnsvpn.ui.Screen
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.*
import kotlinx.coroutines.launch
import javax.inject.Inject

@HiltViewModel
class MetaProfileEditViewModel @Inject constructor(
    savedStateHandle: SavedStateHandle,
    private val repo: ProfileRepository,
) : ViewModel() {

    private val metaId: String = savedStateHandle["metaId"] ?: Screen.MetaProfileEdit.NEW

    private val _meta = MutableStateFlow(MetaProfileEntity())
    val meta: StateFlow<MetaProfileEntity> = _meta.asStateFlow()

    val allProfiles: StateFlow<List<ProfileEntity>> = repo.allProfiles()
        .stateIn(viewModelScope, SharingStarted.WhileSubscribed(), emptyList())

    private val _saved = MutableStateFlow(false)
    val saved: StateFlow<Boolean> = _saved.asStateFlow()

    init {
        if (metaId != Screen.MetaProfileEdit.NEW) {
            viewModelScope.launch {
                repo.getMetaProfile(metaId)?.let { _meta.value = it }
            }
        }
    }

    fun update(updater: MetaProfileEntity.() -> MetaProfileEntity) {
        _meta.value = _meta.value.updater()
    }

    fun toggleProfile(profileId: String) {
        val current = _meta.value.profileIds.split(",").filter { it.isNotBlank() }.toMutableList()
        if (current.contains(profileId)) current.remove(profileId) else current.add(profileId)
        _meta.value = _meta.value.copy(profileIds = current.joinToString(","))
    }

    fun save() {
        viewModelScope.launch {
            repo.saveMetaProfile(_meta.value.copy(updatedAt = System.currentTimeMillis()))
            _saved.value = true
        }
    }
}