package com.masterdnsvpn.ui.viewmodel

import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.masterdnsvpn.profile.ProfileEntity
import com.masterdnsvpn.profile.ProfileRepository
import com.masterdnsvpn.profile.TomlConfigMapper
import com.masterdnsvpn.ui.Screen
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import javax.inject.Inject

@HiltViewModel
class ProfileEditViewModel @Inject constructor(
    savedStateHandle: SavedStateHandle,
    private val repo: ProfileRepository,
) : ViewModel() {

    private val profileId: String = savedStateHandle["profileId"] ?: Screen.ProfileEdit.NEW

    private val _profile = MutableStateFlow(ProfileEntity())
    val profile: StateFlow<ProfileEntity> = _profile.asStateFlow()

    // Use a one-shot event channel instead of a boolean flag to prevent
    // double-navigate on recomposition.
    private val _saved = MutableStateFlow(false)
    val saved: StateFlow<Boolean> = _saved.asStateFlow()

    private val _resolverNavId = MutableStateFlow<String?>(null)
    val resolverNavId: StateFlow<String?> = _resolverNavId.asStateFlow()

    // Prevents concurrent/duplicate saves
    private val saveMutex = Mutex()

    /** Validation error to display in UI */
    private val _validationError = MutableStateFlow<String?>(null)
    val validationError: StateFlow<String?> = _validationError.asStateFlow()

    fun clearValidationError() { _validationError.value = null }

    /**
     * Returns a validation error message if the profile is missing required fields,
     * or null if valid.
     */
    private suspend fun validateProfile(): String? {
        val p = _profile.value
        val resolvers = repo.getProfile(p.id)?.resolversText ?: p.resolversText
        if (resolvers.isBlank() || resolvers.lines().none { it.isNotBlank() }) {
            return "Resolver list is empty. Add at least one resolver before saving."
        }
        if (p.domains.isBlank()) {
            return "DOMAINS is required. Enter at least one domain."
        }
        if (p.dataEncryptionMethod != 0 && p.encryptionKey.isBlank()) {
            return "ENCRYPTION_KEY is required when encryption is enabled."
        }
        return null
    }

    init {
        if (profileId != Screen.ProfileEdit.NEW) {
            viewModelScope.launch {
                repo.getProfileForTunnel(profileId)?.let { _profile.value = it }
            }
        }
    }

    fun update(updater: ProfileEntity.() -> ProfileEntity) {
        _profile.value = _profile.value.updater().copy(updatedAt = System.currentTimeMillis())
    }

    fun save() {
        if (saveMutex.isLocked) return  // Ignore duplicate taps
        viewModelScope.launch {
            saveMutex.withLock {
                val error = validateProfile()
                if (error != null) {
                    _validationError.value = error
                    return@withLock
                }
                val current = repo.getProfile(_profile.value.id)
                val merged = if (current != null) {
                    _profile.value.copy(resolversText = current.resolversText)
                } else {
                    _profile.value
                }
                repo.saveProfile(merged)
                _saved.value = true
            }
        }
    }

    /** Save profile to DB first (for new profiles), then signal navigation to resolver editor. */
    fun saveForResolver() {
        if (saveMutex.isLocked) return
        viewModelScope.launch {
            saveMutex.withLock {
                val current = repo.getProfile(_profile.value.id)
                val merged = if (current != null) {
                    _profile.value.copy(resolversText = current.resolversText)
                } else {
                    _profile.value
                }
                repo.saveProfile(merged)
                _resolverNavId.value = _profile.value.id
            }
        }
    }

    fun clearResolverNav() {
        _resolverNavId.value = null
    }

    /** Called after navigation so the flag doesn't re-trigger on recomposition. */
    fun clearSaved() {
        _saved.value = false
    }

    // ─── Import / Export ──────────────────────────────────────────────────────

    /** Error message from last import attempt, null when none. */
    private val _importError = MutableStateFlow<String?>(null)
    val importError: StateFlow<String?> = _importError.asStateFlow()

    fun clearImportError() { _importError.value = null }

    /**
     * Parse [toml] text and apply all recognised keys to the current profile.
     * Preserves: id, name, createdAt, updatedAt, resolversText.
     */
    fun importFromToml(toml: String) {
        try {
            val imported = TomlConfigMapper.fromToml(toml, _profile.value.name)
            _profile.value = imported.copy(
                id           = _profile.value.id,
                name         = _profile.value.name,
                createdAt    = _profile.value.createdAt,
                updatedAt    = System.currentTimeMillis(),
                resolversText = _profile.value.resolversText,
            )
        } catch (e: Exception) {
            _importError.value = e.message ?: "Failed to parse .toml file"
        }
    }

    /** Render the current profile as a .toml config string (ready to save/share).
     *  When [hideIdentity] is true, the exported config will mark domains &
     *  encryption key as locked — hidden in the importing app's UI. */
    fun exportToToml(hideIdentity: Boolean = false): String =
        TomlConfigMapper.toToml(_profile.value, hideIdentity)
}