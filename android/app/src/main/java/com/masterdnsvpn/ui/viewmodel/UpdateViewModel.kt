package com.masterdnsvpn.ui.viewmodel

import android.app.Application
import android.content.Intent
import android.os.Build
import androidx.core.content.FileProvider
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import com.masterdnsvpn.BuildConfig
import com.masterdnsvpn.profile.ProfileRepository
import com.masterdnsvpn.service.TunnelStateManager
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import org.json.JSONObject
import java.io.File
import java.io.FileOutputStream
import java.net.HttpURLConnection
import java.net.InetSocketAddress
import java.net.Proxy
import java.net.URL
import javax.inject.Inject

// ── Data models ───────────────────────────────────────────────────────────────

data class AssetInfo(
    val name: String,
    val downloadUrl: String,
    val size: Long,
)

data class ReleaseInfo(
    val tagName: String,
    val body: String,
    val assets: List<AssetInfo>,
)

sealed class UpdateUiState {
    data object Idle : UpdateUiState()
    data object Checking : UpdateUiState()
    data class UpToDate(val version: String) : UpdateUiState()
    data class UpdateAvailable(
        val release: ReleaseInfo,
        val preferredAsset: AssetInfo?,
    ) : UpdateUiState()
    data class Downloading(
        val fileName: String,
        val downloadedBytes: Long,
        val totalBytes: Long,
        val progressPercent: Int,
        val usingProxy: Boolean,
    ) : UpdateUiState()
    data class ReadyToInstall(val apkFile: File) : UpdateUiState()
    data class Error(val message: String) : UpdateUiState()
}

// ── ViewModel ─────────────────────────────────────────────────────────────────

@HiltViewModel
class UpdateViewModel @Inject constructor(
    private val app: Application,
    private val tunnelStateManager: TunnelStateManager,
    private val repo: ProfileRepository,
) : AndroidViewModel(app) {

    companion object {
        private const val GITHUB_API =
            "https://api.github.com/repos/RevocGG/MasterDnsVPN-AndroidGG/releases/latest"
        private const val BUFFER_SIZE = 64 * 1024
        private const val CONNECT_TIMEOUT_MS = 20_000
        private const val READ_TIMEOUT_MS = 60_000
    }

    private val _state = MutableStateFlow<UpdateUiState>(UpdateUiState.Idle)
    val state: StateFlow<UpdateUiState> = _state.asStateFlow()

    /** Primary ABI of the running device. */
    val deviceAbi: String = Build.SUPPORTED_ABIS.firstOrNull() ?: "arm64-v8a"

    /** Preferred APK suffix based on device ABI. */
    private val preferredAbiSuffix: String = when {
        deviceAbi.startsWith("arm64") -> "arm64-v8a"
        deviceAbi.startsWith("armeabi") -> "armeabi-v7a"
        deviceAbi.startsWith("x86_64") -> "x86_64"
        deviceAbi.startsWith("x86") -> "x86"
        else -> "universal"
    }

    // ── Public API ────────────────────────────────────────────────────────────

    fun checkForUpdate() {
        viewModelScope.launch {
            _state.value = UpdateUiState.Checking
            try {
                // Always try direct first — the app is excluded from its own VPN via
                // addDisallowedApplication, so direct network access always works.
                // Only fall back to a SOCKS5 proxy if the direct attempt fails.
                val release = withContext(Dispatchers.IO) {
                    try {
                        fetchLatestRelease(null)
                    } catch (_: Exception) {
                        fetchLatestRelease(activeSocksProxy())
                    }
                }
                val current = BuildConfig.VERSION_NAME.trimStart('v').trim()
                val latest = release.tagName.trimStart('v').trim()
                if (!isNewerVersion(latest, current)) {
                    _state.value = UpdateUiState.UpToDate(current)
                } else {
                    val asset = bestAsset(release.assets)
                    _state.value = UpdateUiState.UpdateAvailable(release, asset)
                }
            } catch (e: Exception) {
                _state.value = UpdateUiState.Error(e.message ?: "Failed to check for updates")
            }
        }
    }

    fun startDownload(asset: AssetInfo) {
        viewModelScope.launch {
            try {
                val outFile = File(app.cacheDir, asset.name)
                withContext(Dispatchers.IO) {
                    try {
                        downloadWithProgress(asset, outFile, null)
                    } catch (_: Exception) {
                        downloadWithProgress(asset, outFile, activeSocksProxy())
                    }
                }
                _state.value = UpdateUiState.ReadyToInstall(outFile)
            } catch (e: Exception) {
                _state.value = UpdateUiState.Error(e.message ?: "Download failed")
            }
        }
    }

    fun installApk(file: File) {
        val uri = FileProvider.getUriForFile(app, "${app.packageName}.provider", file)
        val intent = Intent(Intent.ACTION_VIEW).apply {
            setDataAndType(uri, "application/vnd.android.package-archive")
            flags = Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_GRANT_READ_URI_PERMISSION
        }
        app.startActivity(intent)
    }

    fun reset() {
        _state.value = UpdateUiState.Idle
    }

    // ── Private helpers ───────────────────────────────────────────────────────

    /** Returns an active SOCKS5 proxy if any SOCKS5 profile is running, else null. */
    private suspend fun activeSocksProxy(): Proxy? {
        val runningIds = tunnelStateManager.runningProfileIds.value
        if (runningIds.isEmpty()) return null
        for (id in runningIds) {
            val profile = repo.getProfile(id) ?: continue
            if (profile.tunnelMode == "SOCKS5") {
                val addr = InetSocketAddress(profile.listenIP, profile.listenPort)
                return Proxy(Proxy.Type.SOCKS, addr)
            }
        }
        return null
    }

    private fun fetchLatestRelease(proxy: Proxy?): ReleaseInfo {
        val conn = openConnection(GITHUB_API, proxy)
        conn.setRequestProperty("Accept", "application/vnd.github+json")
        conn.setRequestProperty("User-Agent", "MasterDnsVPN-AndroidGG")
        conn.connect()
        check(conn.responseCode == HttpURLConnection.HTTP_OK) {
            "GitHub API returned HTTP ${conn.responseCode}"
        }
        val json = JSONObject(conn.inputStream.bufferedReader().readText())
        conn.disconnect()

        val tagName = json.getString("tag_name")
        val body = json.optString("body", "")
        val assetsArr = json.optJSONArray("assets") ?: return ReleaseInfo(tagName, body, emptyList())
        val assets = (0 until assetsArr.length()).map { i ->
            val a = assetsArr.getJSONObject(i)
            AssetInfo(
                name = a.getString("name"),
                downloadUrl = a.getString("browser_download_url"),
                size = a.optLong("size", 0L),
            )
        }
        return ReleaseInfo(tagName, body, assets)
    }

    private fun downloadWithProgress(asset: AssetInfo, outFile: File, proxy: Proxy?) {
        // Resolve GitHub redirect URL first (GH redirects to CDN)
        val resolvedUrl = resolveRedirect(asset.downloadUrl, proxy)
        val conn = openConnection(resolvedUrl, proxy)
        conn.connect()
        check(conn.responseCode == HttpURLConnection.HTTP_OK) {
            "Download returned HTTP ${conn.responseCode}"
        }
        val totalBytes = if (asset.size > 0) asset.size else conn.contentLengthLong.coerceAtLeast(1L)
        var downloadedBytes = 0L

        conn.inputStream.use { input ->
            FileOutputStream(outFile).use { output ->
                val buffer = ByteArray(BUFFER_SIZE)
                while (true) {
                    val n = input.read(buffer)
                    if (n == -1) break
                    output.write(buffer, 0, n)
                    downloadedBytes += n
                    val pct = ((downloadedBytes * 100) / totalBytes).toInt().coerceIn(0, 100)
                    _state.value = UpdateUiState.Downloading(
                        fileName = asset.name,
                        downloadedBytes = downloadedBytes,
                        totalBytes = totalBytes,
                        progressPercent = pct,
                        usingProxy = proxy != null,
                    )
                }
            }
        }
        conn.disconnect()
    }

    /** Manually follow a single redirect so the final URL is used with the proxy. */
    private fun resolveRedirect(url: String, proxy: Proxy?): String {
        val conn = openConnection(url, proxy)
        conn.instanceFollowRedirects = false
        conn.connect()
        return if (conn.responseCode in 300..399) {
            conn.getHeaderField("Location") ?: url
        } else {
            conn.disconnect()
            url
        }.also { conn.disconnect() }
    }

    private fun openConnection(url: String, proxy: Proxy?): HttpURLConnection {
        val u = URL(url)
        val conn = (if (proxy != null) u.openConnection(proxy) else u.openConnection()) as HttpURLConnection
        conn.connectTimeout = CONNECT_TIMEOUT_MS
        conn.readTimeout = READ_TIMEOUT_MS
        conn.instanceFollowRedirects = true
        return conn
    }

    private fun bestAsset(assets: List<AssetInfo>): AssetInfo? =
        assets.firstOrNull { it.name.contains(preferredAbiSuffix) && it.name.endsWith(".apk") }
            ?: assets.firstOrNull { it.name.contains("universal") && it.name.endsWith(".apk") }
            ?: assets.firstOrNull { it.name.endsWith(".apk") }

    private fun isNewerVersion(latest: String, current: String): Boolean {
        // Compare only numeric version segments (ignore build metadata like -f62330c)
        fun segments(v: String) = v.split("-").first().split(".").map { it.toIntOrNull() ?: 0 }
        val l = segments(latest)
        val c = segments(current)
        for (i in 0 until maxOf(l.size, c.size)) {
            val lv = l.getOrElse(i) { 0 }
            val cv = c.getOrElse(i) { 0 }
            if (lv > cv) return true
            if (lv < cv) return false
        }
        return false
    }
}
