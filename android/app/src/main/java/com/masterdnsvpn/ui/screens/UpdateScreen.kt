package com.masterdnsvpn.ui.screens

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Download
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.SystemUpdate
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.hilt.navigation.compose.hiltViewModel
import com.masterdnsvpn.BuildConfig
import com.masterdnsvpn.ui.theme.*
import com.masterdnsvpn.ui.viewmodel.AssetInfo
import com.masterdnsvpn.ui.viewmodel.UpdateUiState
import com.masterdnsvpn.ui.viewmodel.UpdateViewModel

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun UpdateScreen(
    onNavigateUp: () -> Unit,
    vm: UpdateViewModel = hiltViewModel(),
) {
    val state by vm.state.collectAsState()

    GlassBackground {
        Scaffold(
            containerColor = Color.Transparent,
            topBar = {
                TopAppBar(
                    title = { Text("Update", color = TealLight, fontWeight = FontWeight.Bold) },
                    colors = TopAppBarDefaults.topAppBarColors(containerColor = Color.Transparent),
                    navigationIcon = {
                        IconButton(onClick = onNavigateUp) {
                            Icon(Icons.AutoMirrored.Filled.ArrowBack, "Back", tint = TextSecondary)
                        }
                    },
                )
            },
        ) { padding ->
            Column(
                modifier = Modifier
                    .padding(padding)
                    .padding(horizontal = 16.dp)
                    .verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(16.dp),
            ) {
                Spacer(Modifier.height(4.dp))

                // Current version info card
                GlassCard {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Icon(
                            Icons.Default.SystemUpdate,
                            contentDescription = null,
                            tint = CyanAccent,
                            modifier = Modifier.size(28.dp),
                        )
                        Spacer(Modifier.width(12.dp))
                        Column {
                            Text("Installed Version", color = TextSecondary, fontSize = 11.sp)
                            Text(
                                BuildConfig.VERSION_NAME,
                                color = TextPrimary,
                                fontWeight = FontWeight.SemiBold,
                                fontSize = 16.sp,
                            )
                            Text(
                                "Device ABI: ${vm.deviceAbi}",
                                color = TextSecondary,
                                fontSize = 11.sp,
                            )
                        }
                    }
                }

                // State-driven content
                when (val s = state) {
                    is UpdateUiState.Idle -> {
                        Button(
                            onClick = { vm.checkForUpdate() },
                            modifier = Modifier.fillMaxWidth(),
                            colors = ButtonDefaults.buttonColors(
                                containerColor = TealPrimary.copy(alpha = 0.25f),
                                contentColor = TealLight,
                            ),
                            shape = RoundedCornerShape(14.dp),
                        ) {
                            Icon(Icons.Default.Refresh, null, modifier = Modifier.size(18.dp))
                            Spacer(Modifier.width(8.dp))
                            Text("Check for Updates")
                        }
                    }

                    is UpdateUiState.Checking -> {
                        GlassCard {
                            Row(
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(14.dp),
                            ) {
                                CircularProgressIndicator(
                                    modifier = Modifier.size(24.dp),
                                    color = TealPrimary,
                                    strokeWidth = 2.5.dp,
                                )
                                Text("Checking GitHub for latest release...", color = TextSecondary)
                            }
                        }
                    }

                    is UpdateUiState.UpToDate -> {
                        GlassCard {
                            Row(verticalAlignment = Alignment.CenterVertically) {
                                Icon(Icons.Default.Check, null, tint = GreenOnline, modifier = Modifier.size(24.dp))
                                Spacer(Modifier.width(10.dp))
                                Column {
                                    Text("You're up to date!", color = GreenOnline, fontWeight = FontWeight.SemiBold)
                                    Text("Version ${s.version} is the latest.", color = TextSecondary, fontSize = 12.sp)
                                }
                            }
                        }
                        Button(
                            onClick = { vm.checkForUpdate() },
                            modifier = Modifier.fillMaxWidth(),
                            colors = ButtonDefaults.buttonColors(
                                containerColor = GlassBg,
                                contentColor = TextSecondary,
                            ),
                            shape = RoundedCornerShape(14.dp),
                        ) {
                            Icon(Icons.Default.Refresh, null, modifier = Modifier.size(16.dp))
                            Spacer(Modifier.width(6.dp))
                            Text("Re-check", fontSize = 13.sp)
                        }
                    }

                    is UpdateUiState.UpdateAvailable -> {
                        GlassCard {
                            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                                Row(verticalAlignment = Alignment.CenterVertically) {
                                    Icon(
                                        Icons.Default.Download,
                                        null,
                                        tint = CyanAccent,
                                        modifier = Modifier.size(22.dp),
                                    )
                                    Spacer(Modifier.width(8.dp))
                                    Text(
                                        "Update Available: ${s.release.tagName}",
                                        color = CyanAccent,
                                        fontWeight = FontWeight.Bold,
                                        fontSize = 15.sp,
                                    )
                                }
                                if (s.release.body.isNotBlank()) {
                                    HorizontalDivider(color = GlassBorder)
                                    Text(
                                        "Release Notes:",
                                        color = TextSecondary,
                                        fontSize = 11.sp,
                                        fontWeight = FontWeight.SemiBold,
                                    )
                                    Text(
                                        s.release.body,
                                        color = TextPrimary,
                                        fontSize = 12.sp,
                                        maxLines = 8,
                                        overflow = TextOverflow.Ellipsis,
                                    )
                                }
                            }
                        }

                        s.preferredAsset?.let { asset ->
                            AssetDownloadCard(asset = asset, onDownload = { vm.startDownload(asset) })
                        }

                        if (s.preferredAsset == null) {
                            GlassCard {
                                Text(
                                    "No compatible APK found for your device (${vm.deviceAbi}).",
                                    color = Color(0xFFFFAB40),
                                    fontSize = 13.sp,
                                )
                            }
                        }
                    }

                    is UpdateUiState.Downloading -> {
                        GlassCard {
                            Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                                Row(verticalAlignment = Alignment.CenterVertically) {
                                    CircularProgressIndicator(
                                        progress = { s.progressPercent / 100f },
                                        modifier = Modifier.size(28.dp),
                                        color = TealPrimary,
                                        strokeWidth = 3.dp,
                                    )
                                    Spacer(Modifier.width(12.dp))
                                    Column {
                                        Text("Downloading...", color = TealLight, fontWeight = FontWeight.SemiBold)
                                        Text(
                                            s.fileName,
                                            color = TextSecondary,
                                            fontSize = 11.sp,
                                            maxLines = 1,
                                            overflow = TextOverflow.Ellipsis,
                                        )
                                    }
                                }
                                LinearProgressIndicator(
                                    progress = { s.progressPercent / 100f },
                                    modifier = Modifier.fillMaxWidth(),
                                    color = TealPrimary,
                                    trackColor = GlassBorder,
                                )
                                Row(
                                    modifier = Modifier.fillMaxWidth(),
                                    horizontalArrangement = Arrangement.SpaceBetween,
                                ) {
                                    Text(
                                        "${formatBytes(s.downloadedBytes)} / ${formatBytes(s.totalBytes)}",
                                        color = TextSecondary,
                                        fontSize = 11.sp,
                                    )
                                    Text(
                                        "${s.progressPercent}%",
                                        color = TealLight,
                                        fontSize = 11.sp,
                                        fontWeight = FontWeight.Bold,
                                    )
                                }
                                if (s.usingProxy) {
                                    Text(
                                        "Downloading via active SOCKS5 proxy",
                                        color = GreenOnline,
                                        fontSize = 10.sp,
                                    )
                                }
                            }
                        }
                    }

                    is UpdateUiState.ReadyToInstall -> {
                        GlassCard {
                            Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                                Row(verticalAlignment = Alignment.CenterVertically) {
                                    Icon(Icons.Default.Check, null, tint = GreenOnline, modifier = Modifier.size(22.dp))
                                    Spacer(Modifier.width(8.dp))
                                    Text("Download complete!", color = GreenOnline, fontWeight = FontWeight.SemiBold)
                                }
                                Button(
                                    onClick = { vm.installApk(s.apkFile) },
                                    modifier = Modifier.fillMaxWidth(),
                                    colors = ButtonDefaults.buttonColors(
                                        containerColor = GreenOnline.copy(alpha = 0.2f),
                                        contentColor = GreenOnline,
                                    ),
                                    shape = RoundedCornerShape(14.dp),
                                ) {
                                    Icon(Icons.Default.SystemUpdate, null, modifier = Modifier.size(18.dp))
                                    Spacer(Modifier.width(8.dp))
                                    Text("Install Update")
                                }
                            }
                        }
                    }

                    is UpdateUiState.Error -> {
                        GlassCard {
                            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                                Text("Error", color = RedError, fontWeight = FontWeight.Bold)
                                Text(s.message, color = TextPrimary, fontSize = 13.sp)
                            }
                        }
                        Button(
                            onClick = { vm.reset() },
                            modifier = Modifier.fillMaxWidth(),
                            colors = ButtonDefaults.buttonColors(
                                containerColor = GlassBg,
                                contentColor = TextSecondary,
                            ),
                            shape = RoundedCornerShape(14.dp),
                        ) {
                            Text("Try Again")
                        }
                    }
                }

                Spacer(Modifier.height(24.dp))
            }
        }
    }
}

@Composable
private fun AssetDownloadCard(asset: AssetInfo, onDownload: () -> Unit) {
    Surface(
        shape = RoundedCornerShape(14.dp),
        color = GlassBg,
        modifier = Modifier.fillMaxWidth(),
    ) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 14.dp, vertical = 12.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Column(modifier = Modifier.weight(1f)) {
                Text(asset.name, color = TextPrimary, fontSize = 13.sp, maxLines = 2, overflow = TextOverflow.Ellipsis)
                if (asset.size > 0) {
                    Text(formatBytes(asset.size), color = TextSecondary, fontSize = 11.sp)
                }
            }
            Spacer(Modifier.width(10.dp))
            Button(
                onClick = onDownload,
                colors = ButtonDefaults.buttonColors(
                    containerColor = TealPrimary.copy(alpha = 0.25f),
                    contentColor = TealLight,
                ),
                shape = RoundedCornerShape(12.dp),
                contentPadding = PaddingValues(horizontal = 14.dp, vertical = 8.dp),
            ) {
                Icon(Icons.Default.Download, null, modifier = Modifier.size(16.dp))
                Spacer(Modifier.width(4.dp))
                Text("Download", fontSize = 12.sp)
            }
        }
    }
}

private fun formatBytes(bytes: Long): String = when {
    bytes < 1024 -> "$bytes B"
    bytes < 1024 * 1024 -> "%.1f KB".format(bytes / 1024.0)
    bytes < 1024L * 1024 * 1024 -> "%.1f MB".format(bytes / (1024.0 * 1024))
    else -> "%.2f GB".format(bytes / (1024.0 * 1024 * 1024))
}
