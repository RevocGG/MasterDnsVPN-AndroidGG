package com.masterdnsvpn.ui.viewmodel

import android.content.Context
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.masterdnsvpn.bridge.GoMobileBridge
import com.masterdnsvpn.hotspot.HotspotManager
import com.masterdnsvpn.log.LogEntry
import com.masterdnsvpn.log.LogLevel
import com.masterdnsvpn.log.LogManager
import com.masterdnsvpn.profile.ProfileDao
import com.masterdnsvpn.service.TunnelStateManager
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.*
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import javax.inject.Inject

data class HotspotUiState(
    /** true while the hotspot proxy relay is active in Go */
    val isRunning: Boolean = false,
    /** Hotspot AP IP detected on the device, e.g. "192.168.43.1" */
    val hotspotIp: String? = null,
    /** Port the relay is listening on */
    val listenPort: Int = HotspotManager.DEFAULT_HOTSPOT_PORT,
    /** Full SOCKS5 address to show the user: "<hotspotIp>:<port>" */
    val shareAddress: String? = null,
    /** PAC file URL for Auto proxy config */
    val pacUrl: String? = null,
    /** Error message if starting failed */
    val error: String? = null,
    /** true when no VPN profile is running (hotspot sharing needs VPN) */
    val noActiveVpn: Boolean = true,
)

@HiltViewModel
class HotspotViewModel @Inject constructor(
    private val bridge: GoMobileBridge,
    private val stateManager: TunnelStateManager,
    private val profileDao: ProfileDao,
    private val logManager: LogManager,
) : ViewModel() {

    private val _state = MutableStateFlow(HotspotUiState())
    val state: StateFlow<HotspotUiState> = _state.asStateFlow()

    init {
        // Keep noActiveVpn in sync with running profiles
        viewModelScope.launch {
            stateManager.runningProfileIds
                .combine(stateManager.runningMetaIds) { a, b -> a + b }
                .collect { running ->
                    _state.update { it.copy(noActiveVpn = running.isEmpty()) }
                    // If VPN stopped while hotspot proxy was running, stop it too.
                    if (running.isEmpty() && _state.value.isRunning) {
                        stopProxy()
                    }
                }
        }
    }

    /** Toggle hotspot sharing on/off. */
    fun toggle(ctx: Context) {
        if (_state.value.isRunning) {
            stopProxy()
        } else {
            startProxy(ctx)
        }
    }

    private fun startProxy(ctx: Context) {
        viewModelScope.launch(Dispatchers.IO) {
            val runningIds = stateManager.runningProfileIds.value +
                    stateManager.runningMetaIds.value
            if (runningIds.isEmpty()) {
                _state.update { it.copy(error = "Start a VPN profile first") }
                return@launch
            }

            // Pick the first running profile and get its SOCKS listen addr.
            val profileId = runningIds.first()
            val profile = profileDao.getProfileById(profileId)
            val socksAddr = if (profile != null) {
                "${profile.listenIP}:${profile.listenPort}"
            } else {
                "127.0.0.1:18000"
            }

            val port = HotspotManager.DEFAULT_HOTSPOT_PORT
            val result = runCatching {
                bridge.startHotspotProxy(port, socksAddr)
            }

            val listenAddr = result.getOrNull()
            val err = result.exceptionOrNull()?.message
            val hotspotIp = withContext(Dispatchers.Main) {
                HotspotManager.getHotspotIp(ctx)
            }

            val shareAddr = if (hotspotIp != null && listenAddr != null) {
                "$hotspotIp:$port"
            } else if (listenAddr != null) {
                "YOUR_HOTSPOT_IP:$port"
            } else null

            // Configure PAC server with the hotspot IP and port
            val pacUrl = if (hotspotIp != null && listenAddr != null) {
                bridge.setHotspotPacSocksAddr("$hotspotIp:$port")
                val pacPort = bridge.getHotspotPacPort()
                if (pacPort > 0) "http://$hotspotIp:$pacPort/proxy.pac" else null
            } else null

            if (listenAddr != null) {
                logManager.append(
                    LogEntry(
                        level = LogLevel.INFO, timestamp = "system",
                        message = "Hotspot proxy started on $listenAddr → $socksAddr",
                    )
                )
            }

            _state.update {
                it.copy(
                    isRunning = listenAddr != null,
                    hotspotIp = hotspotIp,
                    listenPort = port,
                    shareAddress = shareAddr,
                    pacUrl = pacUrl,
                    error = err,
                )
            }
        }
    }

    private fun stopProxy() {
        viewModelScope.launch(Dispatchers.IO) {
            bridge.stopHotspotProxy()
            logManager.append(
                LogEntry(level = LogLevel.INFO, timestamp = "system", message = "Hotspot proxy stopped")
            )
            _state.update {
                it.copy(isRunning = false, shareAddress = null, pacUrl = null, error = null)
            }
        }
    }
}
