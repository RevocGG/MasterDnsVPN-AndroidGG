package com.masterdnsvpn.ui.screens

import androidx.compose.animation.core.*
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.hilt.navigation.compose.hiltViewModel
import com.masterdnsvpn.ui.theme.*
import com.masterdnsvpn.ui.viewmodel.MonitorUiState
import com.masterdnsvpn.ui.viewmodel.MonitorViewModel
import com.masterdnsvpn.ui.viewmodel.ProfileMonitorInfo
import com.masterdnsvpn.gomobile.mobile.Stats

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun MonitorScreen(vm: MonitorViewModel = hiltViewModel()) {
    val state by vm.state.collectAsState()

    GlassBackground {
        Scaffold(
            containerColor = Color.Transparent,
            topBar = {
                TopAppBar(
                    title = {
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Icon(Icons.Default.Monitor, "Monitor", tint = CyanAccent)
                            Spacer(Modifier.width(10.dp))
                            Text("System Monitor", fontWeight = FontWeight.Bold, color = TealLight)
                        }
                    },
                    colors = TopAppBarDefaults.topAppBarColors(containerColor = Color.Transparent),
                )
            },
        ) { padding ->
            LazyColumn(
                contentPadding = PaddingValues(16.dp),
                verticalArrangement = Arrangement.spacedBy(12.dp),
                modifier = Modifier.padding(padding),
            ) {
                // System stats tiles
                item {
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.spacedBy(12.dp),
                    ) {
                        StatTile(
                            modifier = Modifier.weight(1f),
                            icon = Icons.Default.Memory,
                            label = "CPU",
                            value = "%.1f%%".format(state.cpuUsage),
                            color = CyanAccent,
                            progress = state.cpuUsage / 100f,
                        )
                        StatTile(
                            modifier = Modifier.weight(1f),
                            icon = Icons.Default.Storage,
                            label = "RAM",
                            value = "${state.memoryUsageMb} MB",
                            color = TealLight,
                            progress = if (state.memoryTotalMb > 0)
                                state.memoryUsageMb.toFloat() / state.memoryTotalMb.toFloat()
                            else 0f,
                        )
                    }
                }

                item {
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.spacedBy(12.dp),
                    ) {
                        StatTile(
                            modifier = Modifier.weight(1f),
                            icon = Icons.Default.ArrowUpward,
                            label = "Upload",
                            value = formatSpeed(state.uploadSpeed),
                            subtitle = formatBytes(state.uploadBytes),
                            color = GreenOnline,
                        )
                        StatTile(
                            modifier = Modifier.weight(1f),
                            icon = Icons.Default.ArrowDownward,
                            label = "Download",
                            value = formatSpeed(state.downloadSpeed),
                            subtitle = formatBytes(state.downloadBytes),
                            color = Color(0xFFFFAB40),
                        )
                    }
                }

                // Active profiles header
                item {
                    Spacer(Modifier.height(8.dp))
                    Text(
                        "Active Profiles",
                        style = MaterialTheme.typography.titleSmall,
                        fontWeight = FontWeight.SemiBold,
                        color = TextSecondary,
                    )
                }

                if (state.activeProfiles.isEmpty()) {
                    item {
                        GlassCard {
                            Row(
                                modifier = Modifier.fillMaxWidth().padding(16.dp),
                                horizontalArrangement = Arrangement.Center,
                                verticalAlignment = Alignment.CenterVertically,
                            ) {
                                Icon(Icons.Default.CloudOff, null, tint = TextSecondary.copy(alpha = 0.5f))
                                Spacer(Modifier.width(10.dp))
                                Text("No active profiles", color = TextSecondary.copy(alpha = 0.7f))
                            }
                        }
                    }
                } else {
                    items(state.activeProfiles, key = { it.profile.id }) { info ->
                        ActiveProfileCard(info)
                    }
                }
            }
        }
    }
}

@Composable
private fun StatTile(
    modifier: Modifier = Modifier,
    icon: ImageVector,
    label: String,
    value: String,
    color: Color,
    subtitle: String? = null,
    progress: Float? = null,
) {
    GlassCard(modifier = modifier) {
        Column(modifier = Modifier.padding(4.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Box(
                    modifier = Modifier
                        .size(32.dp)
                        .clip(CircleShape)
                        .background(color.copy(alpha = 0.15f)),
                    contentAlignment = Alignment.Center,
                ) {
                    Icon(icon, null, tint = color, modifier = Modifier.size(18.dp))
                }
                Spacer(Modifier.width(8.dp))
                Text(label, color = TextSecondary, fontSize = 12.sp)
            }
            Spacer(Modifier.height(8.dp))
            Text(
                value,
                fontWeight = FontWeight.Bold,
                fontSize = 18.sp,
                color = TextPrimary,
            )
            if (subtitle != null) {
                Text(subtitle, fontSize = 11.sp, color = TextSecondary)
            }
            if (progress != null) {
                Spacer(Modifier.height(8.dp))
                LinearProgressIndicator(
                    progress = { progress.coerceIn(0f, 1f) },
                    modifier = Modifier
                        .fillMaxWidth()
                        .height(4.dp)
                        .clip(RoundedCornerShape(2.dp)),
                    color = color,
                    trackColor = color.copy(alpha = 0.12f),
                )
            }
        }
    }
}

@Composable
private fun ActiveProfileCard(info: ProfileMonitorInfo) {
    val profile = info.profile
    val stats = info.stats
    val isReady = stats?.sessionReady == true

    GlassCard {
        Column {
            Row(verticalAlignment = Alignment.CenterVertically) {
                // Pulsing dot
                val animAlpha by rememberInfiniteTransition(label = "pulse").animateFloat(
                    initialValue = 0.4f,
                    targetValue = 1f,
                    animationSpec = infiniteRepeatable(
                        tween(800, easing = FastOutSlowInEasing),
                        RepeatMode.Reverse,
                    ),
                    label = "pulse",
                )
                Box(
                    modifier = Modifier
                        .size(10.dp)
                        .clip(CircleShape)
                        .background(
                            if (isReady) GreenOnline.copy(alpha = animAlpha)
                            else Color(0xFFFFAB40).copy(alpha = animAlpha)
                        ),
                )
                Spacer(Modifier.width(10.dp))
                Column(modifier = Modifier.weight(1f)) {
                    Text(
                        profile.name,
                        fontWeight = FontWeight.SemiBold,
                        color = TextPrimary,
                        fontSize = 15.sp,
                    )
                    Text(
                        if (isReady) "Connected" else "Connecting...",
                        color = if (isReady) GreenOnline else Color(0xFFFFAB40),
                        fontSize = 12.sp,
                    )
                }
                // Mode badge
                Surface(
                    color = TealPrimary.copy(alpha = 0.15f),
                    shape = RoundedCornerShape(8.dp),
                ) {
                    Text(
                        profile.tunnelMode,
                        modifier = Modifier.padding(horizontal = 8.dp, vertical = 4.dp),
                        color = TealLight,
                        fontSize = 11.sp,
                        fontWeight = FontWeight.Medium,
                    )
                }
            }

            if (stats != null) {
                Spacer(Modifier.height(10.dp))
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.SpaceEvenly,
                ) {
                    MiniStat("Resolvers", "${stats.validResolverCount}/${stats.resolverCount}")
                    MiniStat("Listen", stats.listenAddr.ifEmpty { "-" })
                }
            }

            if (!info.lastError.isNullOrBlank()) {
                Spacer(Modifier.height(8.dp))
                Text(
                    info.lastError,
                    color = RedError.copy(alpha = 0.8f),
                    fontSize = 11.sp,
                    maxLines = 2,
                )
            }
        }
    }
}

@Composable
private fun MiniStat(label: String, value: String) {
    Column(horizontalAlignment = Alignment.CenterHorizontally) {
        Text(label, color = TextSecondary, fontSize = 10.sp)
        Text(value, color = TextPrimary, fontWeight = FontWeight.Medium, fontSize = 13.sp)
    }
}

private fun formatBytes(bytes: Long): String = when {
    bytes < 1024 -> "$bytes B"
    bytes < 1024 * 1024 -> "%.1f KB".format(bytes / 1024.0)
    bytes < 1024 * 1024 * 1024 -> "%.1f MB".format(bytes / (1024.0 * 1024.0))
    else -> "%.2f GB".format(bytes / (1024.0 * 1024.0 * 1024.0))
}

private fun formatSpeed(bytesPerSec: Long): String = when {
    bytesPerSec < 1024 -> "$bytesPerSec B/s"
    bytesPerSec < 1024 * 1024 -> "%.1f KB/s".format(bytesPerSec / 1024.0)
    else -> "%.1f MB/s".format(bytesPerSec / (1024.0 * 1024.0))
}
