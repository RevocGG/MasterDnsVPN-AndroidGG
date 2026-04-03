package com.masterdnsvpn.service

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Singleton that tracks which profile IDs currently have an active tunnel service.
 * Updated directly by [DnsTunnelProxyService] and [DnsTunnelVpnService] via Android
 * service lifecycle callbacks (onCreate/onDestroy), which is more reliable than
 * polling the Go bridge's isRunning() state.
 */
@Singleton
class TunnelStateManager @Inject constructor() {

    private val _runningIds = MutableStateFlow<Set<String>>(emptySet())
    val runningProfileIds: StateFlow<Set<String>> = _runningIds.asStateFlow()

    private val _runningMetaIds = MutableStateFlow<Set<String>>(emptySet())
    val runningMetaIds: StateFlow<Set<String>> = _runningMetaIds.asStateFlow()

    private val _busyIds = MutableStateFlow<Set<String>>(emptySet())
    val busyProfileIds: StateFlow<Set<String>> = _busyIds.asStateFlow()

    fun markBusy(id: String) { _busyIds.update { it + id } }
    fun clearBusy(id: String) { _busyIds.update { it - id } }

    fun onTunnelStarted(profileId: String) {
        _runningIds.update { it + profileId }
        _busyIds.update { it - profileId }
    }

    fun onTunnelStopped(profileId: String) {
        _runningIds.update { it - profileId }
        _busyIds.update { it - profileId }
    }

    fun onMetaStarted(metaId: String) {
        _runningMetaIds.update { it + metaId }
        _busyIds.update { it - metaId }
    }

    fun onMetaStopped(metaId: String) {
        _runningMetaIds.update { it - metaId }
        _busyIds.update { it - metaId }
    }

    fun clearAll() {
        _runningIds.value = emptySet()
        _runningMetaIds.value = emptySet()
        _busyIds.value = emptySet()
    }
}
