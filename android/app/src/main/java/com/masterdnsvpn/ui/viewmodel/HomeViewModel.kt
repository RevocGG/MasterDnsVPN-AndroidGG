package com.masterdnsvpn.ui.viewmodel

import android.app.Activity
import android.content.Context
import android.content.Intent
import android.net.VpnService
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.masterdnsvpn.bridge.GoMobileBridge
import com.masterdnsvpn.hardware.HardwareAdvisor
import com.masterdnsvpn.hardware.ProfileWarning
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

    // ── Hardware warning state ────────────────────────────────────────────────────
    private val _pendingWarnings = MutableStateFlow<List<ProfileWarning>?>(null)
    val pendingWarnings: StateFlow<List<ProfileWarning>?> = _pendingWarnings.asStateFlow()

    private val _warningForProfileId = MutableStateFlow<String?>(null)
    val warningForProfileId: StateFlow<String?> = _warningForProfileId.asStateFlow()

    /** The profile whose connect was intercepted by the warning check. Not a Context — safe to hold. */
    private var pendingConnectProfile: ProfileEntity? = null

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

        // Hardware compatibility check — show warning panel if issues found.
        // The profile still starts immediately; the user can apply recommendations later.
        val warnings = HardwareAdvisor.check(ctx, profile)
        if (warnings.isNotEmpty()) {
            pendingConnectProfile = profile
            _pendingWarnings.value = warnings
            _warningForProfileId.value = profile.id
        }

        doConnect(ctx, profile)
    }

    /** User pressed Skip — dismiss warnings; profile is already running, nothing else to do. */
    fun skipWarnings(ctx: Context) {
        clearPendingWarnings()
    }

    /** User pressed dismiss (X or back) — close panel without connecting. */
    fun dismissWarnings() {
        clearPendingWarnings()
    }

    /**
     * User pressed "Apply" — fold all recommended values into the profile
     * and persist to the repository. Does NOT stop or restart the tunnel;
     * the user must manually reconnect to pick up the new settings.
     */
    fun applyRecommendations() {
        val profile = pendingConnectProfile ?: return
        val warnings = _pendingWarnings.value ?: return
        clearPendingWarnings()
        viewModelScope.launch {
            val fullProfile = repo.getProfileForTunnel(profile.id) ?: profile
            val updated = warnings.fold(fullProfile) { p, w -> w.applyTo(p) }
            repo.saveProfile(updated)
        }
    }

    private fun clearPendingWarnings() {
        pendingConnectProfile = null
        _pendingWarnings.value = null
        _warningForProfileId.value = null
    }

    private fun doConnect(ctx: Context, profile: ProfileEntity) {
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
        // Guard against rapid double-tap: if already busy or not running, ignore.
        if (tunnelStateManager.busyProfileIds.value.contains(profileId)) return
        if (!tunnelStateManager.runningProfileIds.value.contains(profileId)) return
        tunnelStateManager.markBusy(profileId)
        // Force-kill: stop Go instance on IO thread, then stop services
        viewModelScope.launch {
            kotlinx.coroutines.withContext(kotlinx.coroutines.Dispatchers.IO) {
                try { bridge.forceStopInstance(profileId) } catch (_: Exception) {}
            }
            try { controller.stopProxy(ctx) } catch (_: Exception) {}
            try { controller.stopVpn(ctx) } catch (_: Exception) {}
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