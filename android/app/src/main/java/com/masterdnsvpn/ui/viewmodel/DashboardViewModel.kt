package com.masterdnsvpn.ui.viewmodel

import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.masterdnsvpn.bridge.GoMobileBridge
import com.masterdnsvpn.gomobile.mobile.Stats
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import javax.inject.Inject

@HiltViewModel
class DashboardViewModel @Inject constructor(
    savedStateHandle: SavedStateHandle,
    private val bridge: GoMobileBridge,
) : ViewModel() {

    private val profileId: String = savedStateHandle["profileId"] ?: ""

    private val _stats = MutableStateFlow<Stats?>(null)
    val stats: StateFlow<Stats?> = _stats.asStateFlow()

    init {
        viewModelScope.launch {
            while (isActive) {
                _stats.value = if (profileId.isNotBlank() && bridge.isRunning(profileId)) {
                    bridge.getStats(profileId)
                } else {
                    null
                }
                delay(1_000)
            }
        }
    }
}