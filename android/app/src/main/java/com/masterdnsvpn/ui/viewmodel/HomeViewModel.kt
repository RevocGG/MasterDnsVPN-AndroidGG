package com.masterdnsvpn.ui.viewmodel

import android.app.Activity
import android.content.Context
import android.content.Intent
import android.net.VpnService
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.masterdnsvpn.bridge.GoMobileBridge
import com.masterdnsvpn.profile.MetaProfileEntity
import com.masterdnsvpn.profile.ProfileEntity
import com.masterdnsvpn.profile.ProfileRepository
import com.masterdnsvpn.service.MetaProfileBalancer
import com.masterdnsvpn.service.TunnelController
import com.masterdnsvpn.service.TunnelStateManager
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.*
import kotlinx.coroutines.launch
import javax.inject.Inject

data class HomeUiState(
    val profiles: List<ProfileEntity> = emptyList(),
    val metaProfiles: List<MetaProfileEntity> = emptyList(),
    val runningProfileIds: Set<String> = emptySet(),
    val runningMetaIds: Set<String> = emptySet(),
    val busyIds: Set<String> = emptySet(),
    val vpnPermissionNeeded: Intent? = null,
)

@HiltViewModel
class HomeViewModel @Inject constructor(
    private val repo: ProfileRepository,
    private val bridge: GoMobileBridge,
    private val controller: TunnelController,
    private val tunnelStateManager: TunnelStateManager,
    private val metaBalancer: MetaProfileBalancer,
) : ViewModel() {

    private val _vpnPermissionIntent = MutableStateFlow<Intent?>(null)

    val uiState: StateFlow<HomeUiState> = combine(
        repo.allProfiles(),
        repo.allMetaProfiles(),
        _vpnPermissionIntent,
        tunnelStateManager.runningProfileIds,
        combine(tunnelStateManager.runningMetaIds, tunnelStateManager.busyProfileIds) { a, b -> a to b },
    ) { args ->
        @Suppress("UNCHECKED_CAST") val raw = args[4] as Pair<Set<String>, Set<String>>
        val metaIds = raw.first
        val busyIds = raw.second
        HomeUiState(
            profiles = args[0] as List<ProfileEntity>,
            metaProfiles = args[1] as List<MetaProfileEntity>,
            vpnPermissionNeeded = args[2] as? Intent,
            runningProfileIds = args[3] as Set<String>,
            runningMetaIds = metaIds,
            busyIds = busyIds,
        )
    }.stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), HomeUiState())

    fun connectProfile(ctx: Context, profile: ProfileEntity) {
        if (tunnelStateManager.busyProfileIds.value.contains(profile.id)) return
        if (tunnelStateManager.runningProfileIds.value.contains(profile.id)) return
        tunnelStateManager.markBusy(profile.id)
        viewModelScope.launch {
            when (profile.tunnelMode) {
                "TUN" -> {
                    val permIntent = controller.prepareVpnIntent(ctx)
                    if (permIntent != null) {
                        _vpnPermissionIntent.value = permIntent
                    } else {
                        controller.startVpn(ctx, profile.id, profile.name)
                    }
                }
                else -> controller.startProxy(ctx, profile.id, profile.name)
            }
        }
    }

    fun onVpnPermissionResult(ctx: Context, profileId: String, granted: Boolean) {
        _vpnPermissionIntent.value = null
        if (!granted) {
            pendingMetaVpn = null
            return
        }
        // Check if this was a meta profile VPN permission request
        val metaProfile = pendingMetaVpn
        if (metaProfile != null) {
            pendingMetaVpn = null
            viewModelScope.launch { startMetaInternal(ctx, metaProfile) }
            return
        }
        viewModelScope.launch {
            val p = repo.getProfile(profileId) ?: return@launch
            controller.startVpn(ctx, p.id, p.name)
        }
    }

    fun disconnectProfile(ctx: Context, profileId: String) {
        tunnelStateManager.markBusy(profileId)
        // Force-kill: stop Go instance immediately, then stop services
        viewModelScope.launch {
            try { bridge.forceStopInstance(profileId) } catch (_: Exception) {}
            controller.stopProxy(ctx)
            controller.stopVpn(ctx)
            tunnelStateManager.onTunnelStopped(profileId)
        }
    }

    fun deleteProfile(profileId: String) {
        viewModelScope.launch { repo.deleteProfile(profileId) }
    }

    /** Import a fully-built ProfileEntity (e.g. from QR code scan). */
    fun importProfileFromEntity(profile: ProfileEntity) {
        viewModelScope.launch { repo.saveProfile(profile) }
    }

    fun deleteMetaProfile(metaId: String) {
        viewModelScope.launch { repo.deleteMetaProfile(metaId) }
    }

    // ------------------------------------------------------------------
    // Meta Profile start / stop
    // ------------------------------------------------------------------

    private var pendingMetaVpn: MetaProfileEntity? = null

    fun connectMetaProfile(ctx: Context, meta: MetaProfileEntity) {
        if (tunnelStateManager.busyProfileIds.value.contains(meta.id)) return
        if (tunnelStateManager.runningMetaIds.value.contains(meta.id)) return
        val profileIds = meta.profileIds.split(",").filter { it.isNotBlank() }
        if (profileIds.isEmpty()) return
        tunnelStateManager.markBusy(meta.id)

        viewModelScope.launch {
            when (meta.tunnelMode) {
                "TUN" -> {
                    val permIntent = controller.prepareVpnIntent(ctx)
                    if (permIntent != null) {
                        pendingMetaVpn = meta
                        _vpnPermissionIntent.value = permIntent
                    } else {
                        startMetaInternal(ctx, meta)
                    }
                }
                else -> startMetaInternal(ctx, meta)
            }
        }
    }

    private suspend fun startMetaInternal(ctx: Context, meta: MetaProfileEntity) {
        val profileIds = meta.profileIds.split(",").filter { it.isNotBlank() }
        if (profileIds.isEmpty()) return

        // Use the first sub-profile as the "primary" for TUN/service naming
        val firstProfile = repo.getProfileForTunnel(profileIds.first()) ?: return

        when (meta.tunnelMode) {
            "TUN" -> {
                controller.startMetaVpn(ctx, meta.id, meta.name, profileIds, meta.balancingStrategy, meta.socksPort)
            }
            else -> {
                controller.startMetaProxy(ctx, meta.id, meta.name, profileIds, meta.balancingStrategy, meta.socksPort)
            }
        }

        tunnelStateManager.onMetaStarted(meta.id)

        // Start balancer monitoring
        metaBalancer.start(meta, viewModelScope) { newProfileId ->
            // Balancer decided to switch — in SOCKS mode this is informational,
            // in TUN mode it could switch the TUN bridge target
        }
    }

    fun disconnectMetaProfile(ctx: Context, metaId: String) {
        tunnelStateManager.markBusy(metaId)
        viewModelScope.launch {
            metaBalancer.stop()
            try { bridge.forceStopAllInstances() } catch (_: Exception) {}
            controller.stopMetaProfiles(ctx, metaId)
            tunnelStateManager.onMetaStopped(metaId)
        }
    }
}