package com.masterdnsvpn.ui.screens

import android.app.Activity
import android.content.Context
import android.content.Intent
import android.graphics.BitmapFactory
import android.net.Uri
import android.os.Build
import android.os.VibrationEffect
import android.os.Vibrator
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.*
import androidx.compose.animation.expandVertically
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.shrinkVertically
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.*
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.runtime.saveable.rememberSaveable
import kotlinx.coroutines.launch
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.hilt.navigation.compose.hiltViewModel
import com.masterdnsvpn.BuildConfig
import com.masterdnsvpn.hardware.ProfileWarning
import com.masterdnsvpn.profile.MetaProfileEntity
import com.masterdnsvpn.profile.ProfileEntity
import com.masterdnsvpn.ui.theme.*
import com.masterdnsvpn.ui.viewmodel.HomeViewModel
import com.masterdnsvpn.ui.viewmodel.HotspotViewModel
import com.masterdnsvpn.ui.viewmodel.MonitorViewModel
import com.masterdnsvpn.gomobile.mobile.Stats


@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun HomeScreen(
    onEditProfile: (String) -> Unit,
    onNewProfile: () -> Unit,
    onOpenDashboard: (String) -> Unit,
    onNewMetaProfile: () -> Unit,
    onEditMetaProfile: (String) -> Unit,
    vm: HomeViewModel = hiltViewModel(),
    monitorVm: MonitorViewModel = hiltViewModel(),
    hotspotVm: HotspotViewModel = hiltViewModel(),
) {
    val ctx = LocalContext.current
    val uiState by vm.uiState.collectAsState()
    val monitorState by monitorVm.state.collectAsState()
    val pendingWarnings by vm.pendingWarnings.collectAsState()
    val warningForProfileId by vm.warningForProfileId.collectAsState()
    val hotspotState by hotspotVm.state.collectAsState()

    var showHotspotDialog by remember { mutableStateOf(false) }
    var showAboutDialog by remember { mutableStateOf(false) }
    val snackbarHostState = remember { SnackbarHostState() }
    val coroutineScope = rememberCoroutineScope()



    // Show welcome dialog on first launch
    val prefs = ctx.getSharedPreferences("masterdnsvpn_prefs", android.content.Context.MODE_PRIVATE)
    var showWelcome by remember { mutableStateOf(!prefs.getBoolean("welcome_shown", false)) }
    if (showWelcome) {
        showAboutDialog = true
        prefs.edit().putBoolean("welcome_shown", true).apply()
        showWelcome = false
    }

    var pendingVpnProfileId by remember { mutableStateOf<String?>(null) }
    val vpnPermLauncher = rememberLauncherForActivityResult(
        ActivityResultContracts.StartActivityForResult()
    ) { result ->
        vm.onVpnPermissionResult(ctx, pendingVpnProfileId ?: "", result.resultCode == Activity.RESULT_OK)
        pendingVpnProfileId = null
    }
    LaunchedEffect(uiState.vpnPermissionNeeded) {
        uiState.vpnPermissionNeeded?.let { intent ->
            vpnPermLauncher.launch(intent)
        }
    }

    // Load logo bitmap
    val logoBitmap = remember {
        try {
            val stream = ctx.assets?.open("masterdnsvpn.png")
                ?: ctx.resources.openRawResource(
                    ctx.resources.getIdentifier("ic_launcher", "mipmap", ctx.packageName)
                )
            BitmapFactory.decodeStream(stream)?.asImageBitmap()
        } catch (_: Exception) { null }
    }

    GlassBackground {
        // ── About / Welcome dialog ─────────────────────────────────────
        if (showAboutDialog) {
            AlertDialog(
                onDismissRequest = { showAboutDialog = false },
                containerColor = DarkSurface,
                icon = {
                    logoBitmap?.let {
                        Image(
                            bitmap = it,
                            contentDescription = "Logo",
                            modifier = Modifier.size(72.dp).clip(CircleShape),
                        )
                    }
                },
                title = {
                    Text(
                        "Welcome to MasterDnsVPN-GG",
                        color = TealLight,
                        fontWeight = FontWeight.Bold,
                        fontSize = 18.sp,
                    )
                },
                text = {
                    Column(
                        verticalArrangement = Arrangement.spacedBy(12.dp),
                        modifier = Modifier.fillMaxWidth(),
                    ) {
                        Text(
                            "An open-source Android client for MasterDnsVPN",
                            color = TextSecondary,
                            fontSize = 14.sp,
                        )
                        Spacer(Modifier.height(4.dp))

                        AboutLink(icon = Icons.Default.Code, label = "Main GitHub", url = "https://github.com/masterking32/MasterDnsVPN")
                        AboutLink(icon = Icons.Default.Send, label = "Main Telegram", url = "https://t.me/masterdnsvpn")
                        AboutLink(icon = Icons.Default.PhoneAndroid, label = "GG Android Client", url = "https://github.com/RevocGG/MasterDnsVPN-AndroidGG")

                        Spacer(Modifier.height(8.dp))
                        HorizontalDivider(color = TextSecondary.copy(alpha = 0.3f))
                        Spacer(Modifier.height(4.dp))
                        Text(
                            "Upstream Engine",
                            color = TextSecondary,
                            fontSize = 12.sp,
                        )
                        Text(
                            BuildConfig.UPSTREAM_VERSION,
                            color = CyanAccent,
                            fontWeight = FontWeight.Medium,
                            fontSize = 13.sp,
                        )
                        Text(
                            "App Version: ${BuildConfig.VERSION_NAME}",
                            color = TextSecondary,
                            fontSize = 12.sp,
                        )
                    }
                },
                confirmButton = {
                    TextButton(onClick = { showAboutDialog = false }) {
                        Text("OK", color = TealLight)
                    }
                },
            )
        }

        // ── Hotspot sharing dialog ─────────────────────────────────────
        if (showHotspotDialog) {
            AlertDialog(
                onDismissRequest = { showHotspotDialog = false },
                containerColor = DarkSurface,
                title = {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Icon(
                            Icons.Default.Wifi,
                            contentDescription = null,
                            tint = if (hotspotState.isRunning) GreenOnline else TextSecondary,
                            modifier = Modifier.size(22.dp),
                        )
                        Spacer(Modifier.width(8.dp))
                        Text("Hotspot Sharing", color = TextPrimary, fontWeight = FontWeight.Bold)
                    }
                },
                text = {
                    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                        if (hotspotState.noActiveVpn) {
                            Text(
                                "No VPN profile is running. Start a profile first, then enable hotspot sharing.",
                                color = Color(0xFFFFAB40),
                                fontSize = 14.sp,
                            )
                        } else if (hotspotState.error != null) {
                            Text("Error: ${hotspotState.error}", color = Color(0xFFFF5252), fontSize = 13.sp)
                        } else if (hotspotState.isRunning && hotspotState.shareAddress != null) {
                            Text(
                                "Hotspot proxy is running.\nConfigure this SOCKS5 proxy on other devices:",
                                color = TextSecondary,
                                fontSize = 13.sp,
                            )
                            Spacer(Modifier.height(4.dp))
                            Surface(
                                color = DarkBg,
                                shape = RoundedCornerShape(8.dp),
                            ) {
                                Text(
                                    hotspotState.shareAddress!!,
                                    color = CyanAccent,
                                    fontWeight = FontWeight.Bold,
                                    fontSize = 18.sp,
                                    modifier = Modifier.padding(horizontal = 16.dp, vertical = 10.dp),
                                )
                            }
                            Spacer(Modifier.height(4.dp))
                            Text(
                                "Set as SOCKS5 proxy on Wi-Fi connected devices.",
                                color = TextSecondary,
                                fontSize = 12.sp,
                            )
                            if (hotspotState.pacUrl != null) {
                                Spacer(Modifier.height(8.dp))
                                Text(
                                    "Or use Auto proxy (PAC URL):",
                                    color = TextSecondary,
                                    fontSize = 12.sp,
                                )
                                Spacer(Modifier.height(4.dp))
                                Surface(
                                    color = DarkBg,
                                    shape = RoundedCornerShape(8.dp),
                                ) {
                                    Text(
                                        hotspotState.pacUrl!!,
                                        color = androidx.compose.ui.graphics.Color(0xFFFFCC02),
                                        fontWeight = FontWeight.Medium,
                                        fontSize = 13.sp,
                                        modifier = Modifier.padding(horizontal = 12.dp, vertical = 8.dp),
                                    )
                                }
                            }
                        } else {
                            Text("Starting hotspot proxy...", color = TextSecondary, fontSize = 13.sp)
                        }
                    }
                },
                confirmButton = {
                    if (hotspotState.isRunning) {
                        TextButton(onClick = {
                            hotspotVm.toggle(ctx)
                            showHotspotDialog = false
                        }) {
                            Text("Stop", color = Color(0xFFFF5252))
                        }
                    } else {
                        TextButton(onClick = { showHotspotDialog = false }) {
                            Text("Close", color = TealLight)
                        }
                    }
                },
                dismissButton = {
                    TextButton(onClick = { showHotspotDialog = false }) {
                        Text("Dismiss", color = TextSecondary)
                    }
                },
            )
        }
        Scaffold(
            containerColor = androidx.compose.ui.graphics.Color.Transparent,
            snackbarHost = { SnackbarHost(snackbarHostState) },
            topBar = {
                TopAppBar(
                    title = {
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            logoBitmap?.let {
                                Image(
                                    bitmap = it,
                                    contentDescription = "Logo",
                                    modifier = Modifier.size(36.dp).clip(CircleShape),
                                )
                                Spacer(Modifier.width(10.dp))
                            }
                            Text(
                                "MasterDnsVPN-GG",
                                fontWeight = FontWeight.Bold,
                                color = TealLight,
                            )
                        }
                    },
                    colors = TopAppBarDefaults.topAppBarColors(
                        containerColor = androidx.compose.ui.graphics.Color.Transparent,
                    ),
                    actions = {
                        IconButton(onClick = { showAboutDialog = true }) {
                            Icon(Icons.Default.Info, "About", tint = CyanAccent)
                        }
                        IconButton(onClick = onNewMetaProfile) {
                            Icon(Icons.Default.AccountTree, "Meta Profile", tint = CyanAccent)
                        }
                    },
                )
            },
            floatingActionButton = {
                // Hotspot sharing FAB only
                SmallFloatingActionButton(
                    onClick = {
                        if (hotspotState.isRunning) {
                            showHotspotDialog = true
                        } else {
                            hotspotVm.toggle(ctx)
                            showHotspotDialog = true
                        }
                    },
                    containerColor = if (hotspotState.isRunning) GreenOnline else DarkSurface,
                    contentColor = if (hotspotState.isRunning) Color.White else TextSecondary,
                ) {
                    Icon(Icons.Default.Wifi, contentDescription = "Hotspot Sharing")
                }
            },
        ) { padding ->
            LazyColumn(
                contentPadding = PaddingValues(16.dp),
                verticalArrangement = Arrangement.spacedBy(12.dp),
                modifier = Modifier.padding(padding),
            ) {
                // ── Status Dashboard ──────────────────────────────
                item {
                    StatusDashboard(
                        monitorState = monitorState,
                        runningCount = uiState.runningProfileIds.size + uiState.runningMetaIds.size,
                    )
                }

                // Meta profiles section
                if (uiState.metaProfiles.isNotEmpty()) {
                    item {
                        Text(
                            "Meta Profiles",
                            style = MaterialTheme.typography.titleSmall,
                            color = TextSecondary,
                            modifier = Modifier.padding(bottom = 4.dp),
                        )
                    }
                    items(uiState.metaProfiles, key = { "meta_${it.id}" }) { meta ->
                        val subIds = remember(meta.profileIds) {
                            meta.profileIds.split(",").filter { it.isNotBlank() }
                        }
                        val metaIsReady = remember(monitorState.activeProfiles, subIds) {
                            subIds.any { sid ->
                                monitorState.activeProfiles.any { it.profile.id == sid && it.stats?.sessionReady == true }
                            }
                        }
                        MetaProfileCard(
                            meta = meta,
                            isRunning = uiState.runningMetaIds.contains(meta.id),
                            isBusy = uiState.busyIds.contains(meta.id),
                            isReady = metaIsReady,
                            onStart = { vm.connectMetaProfile(ctx, meta) },
                            onStop = { vm.disconnectMetaProfile(ctx, meta.id) },
                            onEdit = { onEditMetaProfile(meta.id) },
                            onDelete = { vm.deleteMetaProfile(meta.id) },
                        )
                    }
                    item { Spacer(Modifier.height(8.dp)) }
                    item {
                        Text(
                            "Profiles",
                            style = MaterialTheme.typography.titleSmall,
                            color = TextSecondary,
                            modifier = Modifier.padding(bottom = 4.dp),
                        )
                    }
                }

                items(uiState.profiles, key = { it.id }) { profile ->
                    val monitorInfo = monitorState.activeProfiles.find { it.profile.id == profile.id }
                    val totalBytes = monitorVm.bandwidthPrefs.getTotalBytes(profile.id)
                    val wireTotalBytes = monitorVm.bandwidthPrefs.getWireTotalBytes(profile.id)
                    ProfileCard(
                        profile = profile,
                        isRunning = uiState.runningProfileIds.contains(profile.id),
                        isBusy = uiState.busyIds.contains(profile.id),
                        monitorInfo = monitorInfo,
                        totalUsageBytes = totalBytes,
                        wireTotalBytes = wireTotalBytes,
                        onConnect = {
                            pendingVpnProfileId = profile.id
                            vm.connectProfile(ctx, profile)
                        },
                        onDisconnect = { vm.disconnectProfile(ctx, profile.id) },
                        onEdit = { onEditProfile(profile.id) },
                        onDashboard = { onOpenDashboard(profile.id) },
                        onDelete = { vm.deleteProfile(profile.id) },
                    )
                    // ── Hardware warning panel (slides down below the card) ─────────
                    AnimatedVisibility(
                        visible = warningForProfileId == profile.id && pendingWarnings != null,
                        enter = expandVertically(
                            animationSpec = tween(durationMillis = 1000, easing = FastOutSlowInEasing),
                        ) + fadeIn(animationSpec = tween(800, delayMillis = 150)),
                        exit = shrinkVertically(
                            animationSpec = tween(durationMillis = 400, easing = LinearOutSlowInEasing),
                        ) + fadeOut(animationSpec = tween(300)),
                    ) {
                        pendingWarnings?.let { warnings ->
                            HardwareWarningPanel(
                                warnings = warnings,
                                onSkip = { vm.skipWarnings(ctx) },
                                onApply = { vm.applyRecommendations() },
                                onDismiss = { vm.dismissWarnings() },
                            )
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun HardwareWarningPanel(
    warnings: List<ProfileWarning>,
    onSkip: () -> Unit,
    onApply: () -> Unit,
    onDismiss: () -> Unit,
) {
    val amber = Color(0xFFFFAB40)
    val amberBg = amber.copy(alpha = 0.08f)
    val amberBorder = amber.copy(alpha = 0.35f)

    Surface(
        color = amberBg,
        shape = RoundedCornerShape(16.dp),
        modifier = Modifier
            .fillMaxWidth()
            .padding(bottom = 4.dp)
            .drawBehind {
                val strokePx = 1.dp.toPx()
                val half = strokePx / 2f
                val r = 16.dp.toPx()
                val path = Path().apply {
                    // Start at top-left, after the top-left corner arc
                    moveTo(r, half)
                    // Top edge
                    lineTo(size.width - r, half)
                    // Top-right corner
                    quadraticBezierTo(size.width - half, half, size.width - half, r)
                    // Right edge
                    lineTo(size.width - half, size.height - r)
                    // Bottom-right corner
                    quadraticBezierTo(size.width - half, size.height - half, size.width - r, size.height - half)
                    // Bottom edge
                    lineTo(r, size.height - half)
                    // Bottom-left corner
                    quadraticBezierTo(half, size.height - half, half, size.height - r)
                    // Left edge
                    lineTo(half, r)
                    // Top-left corner
                    quadraticBezierTo(half, half, r, half)
                }
                drawPath(path, color = amberBorder, style = Stroke(width = strokePx, cap = StrokeCap.Square))
            },
    ) {
        Column(modifier = Modifier.padding(horizontal = 16.dp, vertical = 14.dp)) {

            // ── Title row ────────────────────────────────────────────────
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(
                    Icons.Default.Warning,
                    contentDescription = null,
                    tint = amber,
                    modifier = Modifier.size(20.dp),
                )
                Spacer(Modifier.width(8.dp))
                Text(
                    "Hardware Compatibility Warning",
                    color = amber,
                    fontWeight = FontWeight.Bold,
                    fontSize = 14.sp,
                    modifier = Modifier.weight(1f),
                )
                IconButton(onClick = onDismiss, modifier = Modifier.size(28.dp)) {
                    Icon(Icons.Default.Close, contentDescription = "Close", tint = TextSecondary, modifier = Modifier.size(18.dp))
                }
            }

            // ── Warning items ─────────────────────────────────────────────
            Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                warnings.forEach { w ->
                    Column {
                        Text(
                            "• ${w.fieldLabel}",
                            color = TextPrimary,
                            fontWeight = FontWeight.SemiBold,
                            fontSize = 13.sp,
                        )
                        Row(modifier = Modifier.padding(start = 10.dp, top = 2.dp)) {
                            Surface(
                                color = Color(0xFFFF5252).copy(alpha = 0.12f),
                                shape = RoundedCornerShape(4.dp),
                            ) {
                                Text(
                                    "Current: ${w.currentValue}",
                                    color = Color(0xFFFF5252),
                                    fontSize = 11.sp,
                                    modifier = Modifier.padding(horizontal = 6.dp, vertical = 2.dp),
                                )
                            }
                            Spacer(Modifier.width(6.dp))
                            Text("→", color = TextSecondary, fontSize = 11.sp)
                            Spacer(Modifier.width(6.dp))
                            Surface(
                                color = GreenOnline.copy(alpha = 0.12f),
                                shape = RoundedCornerShape(4.dp),
                            ) {
                                Text(
                                    "Recommended: ${w.recommendedValue}",
                                    color = GreenOnline,
                                    fontSize = 11.sp,
                                    modifier = Modifier.padding(horizontal = 6.dp, vertical = 2.dp),
                                )
                            }
                        }
                        Text(
                            w.reason,
                            color = TextSecondary,
                            fontSize = 11.sp,
                            modifier = Modifier.padding(start = 10.dp, top = 3.dp),
                        )
                    }
                }
            }

            HorizontalDivider(
                modifier = Modifier.padding(vertical = 12.dp),
                color = amberBorder,
            )

            // ── Action buttons ─────────────────────────────────────────────
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(10.dp),
            ) {
                OutlinedButton(
                    onClick = onSkip,
                    modifier = Modifier.weight(1f),
                    colors = ButtonDefaults.outlinedButtonColors(contentColor = TextSecondary),
                    border = androidx.compose.foundation.BorderStroke(1.dp, TextSecondary.copy(alpha = 0.4f)),
                ) {
                    Text("Skip", fontSize = 13.sp)
                }
                Button(
                    onClick = onApply,
                    modifier = Modifier.weight(1f),
                    colors = ButtonDefaults.buttonColors(
                        containerColor = amber,
                        contentColor = Color.Black,
                    ),
                ) {
                    Icon(Icons.Default.Tune, contentDescription = null, modifier = Modifier.size(15.dp))
                    Spacer(Modifier.width(4.dp))
                    Text("Apply", fontSize = 13.sp, fontWeight = FontWeight.Bold)
                }
            }
        }
    }
}

@Composable
private fun StatusDashboard(
    monitorState: com.masterdnsvpn.ui.viewmodel.MonitorUiState,
    runningCount: Int,
) {
    Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
        // Connection status banner
        GlassCard {
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                val connected = runningCount > 0
                Box(
                    modifier = Modifier
                        .size(10.dp)
                        .clip(CircleShape)
                        .background(if (connected) GreenOnline else TextSecondary.copy(alpha = 0.3f)),
                )
                Spacer(Modifier.width(10.dp))
                Text(
                    if (connected) "$runningCount profile(s) active" else "Disconnected",
                    color = if (connected) GreenOnline else TextSecondary,
                    fontWeight = FontWeight.SemiBold,
                    fontSize = 14.sp,
                )
            }
        }

        // Mini stat tiles — 2x2 grid
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            MiniStatTile(
                modifier = Modifier.weight(1f),
                icon = Icons.Default.Memory,
                label = "CPU",
                value = "%.0f%%".format(monitorState.cpuUsage),
                color = CyanAccent,
            )
            MiniStatTile(
                modifier = Modifier.weight(1f),
                icon = Icons.Default.Storage,
                label = "RAM",
                value = "${monitorState.memoryUsageMb}MB",
                color = TealLight,
            )
        }
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            MiniStatTile(
                modifier = Modifier.weight(1f),
                icon = Icons.Default.ArrowUpward,
                label = "UP",
                value = formatSpeedCompact(monitorState.uploadSpeed),
                color = GreenOnline,
            )
            MiniStatTile(
                modifier = Modifier.weight(1f),
                icon = Icons.Default.ArrowDownward,
                label = "DOWN",
                value = formatSpeedCompact(monitorState.downloadSpeed),
                color = Color(0xFFFFAB40),
            )
        }
    }
}

@Composable
private fun MiniStatTile(
    modifier: Modifier = Modifier,
    icon: androidx.compose.ui.graphics.vector.ImageVector,
    label: String,
    value: String,
    color: Color,
) {
    GlassCard(modifier = modifier) {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            modifier = Modifier.padding(vertical = 2.dp),
        ) {
            Icon(icon, null, tint = color, modifier = Modifier.size(16.dp))
            Spacer(Modifier.width(6.dp))
            Text(label, color = TextSecondary, fontSize = 10.sp)
            Spacer(Modifier.weight(1f))
            Text(value, color = TextPrimary, fontWeight = FontWeight.Bold, fontSize = 13.sp)
        }
    }
}

private fun formatSpeedCompact(bytesPerSec: Long): String = when {
    bytesPerSec < 1024 -> "${bytesPerSec}B/s"
    bytesPerSec < 1024 * 1024 -> "%.1fKB/s".format(bytesPerSec / 1024.0)
    else -> "%.1fMB/s".format(bytesPerSec / (1024.0 * 1024.0))
}

@Composable
private fun formatBytes(bytes: Long): String {
    if (bytes < 1024) return "${bytes} B"
    val kb = bytes / 1024.0
    if (kb < 1024) return "%.1f KB".format(kb)
    val mb = kb / 1024.0
    if (mb < 1024) return "%.1f MB".format(mb)
    val gb = mb / 1024.0
    return "%.2f GB".format(gb)
}

@Composable
private fun AboutLink(
    icon: androidx.compose.ui.graphics.vector.ImageVector,
    label: String,
    url: String,
) {
    val ctx = LocalContext.current
    Row(
        verticalAlignment = Alignment.CenterVertically,
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(8.dp))
            .clickable { ctx.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url))) }
            .background(DarkBg.copy(alpha = 0.5f))
            .padding(horizontal = 12.dp, vertical = 10.dp),
    ) {
        Icon(icon, contentDescription = null, tint = CyanAccent, modifier = Modifier.size(20.dp))
        Spacer(Modifier.width(10.dp))
        Column {
            Text(label, color = TextPrimary, fontWeight = FontWeight.Medium, fontSize = 14.sp)
            Text(url, color = CyanAccent, fontSize = 11.sp, textDecoration = TextDecoration.Underline)
        }
    }
}

@Composable
private fun ProfileCard(
    profile: ProfileEntity,
    isRunning: Boolean,
    isBusy: Boolean,
    monitorInfo: com.masterdnsvpn.ui.viewmodel.ProfileMonitorInfo?,
    totalUsageBytes: Long = 0L,
    wireTotalBytes: Long = 0L,
    onConnect: () -> Unit,
    onDisconnect: () -> Unit,
    onEdit: () -> Unit,
    onDashboard: () -> Unit,
    onDelete: () -> Unit,
) {
    var showDeleteDialog by remember { mutableStateOf(false) }
    // Instant feedback — set true on click, reset when external state actually changes
    var localBusy by remember { mutableStateOf(false) }
    LaunchedEffect(isRunning, isBusy) { localBusy = false }
    val effectiveBusy = localBusy || isBusy

    // Scanning = tunnel started but session not ready yet
    val isScanning = isRunning && monitorInfo?.stats?.sessionReady != true
    val isReady = isRunning && monitorInfo?.stats?.sessionReady == true

    // Vibrate once when session transitions to ready.
    // rememberSaveable survives navigation so it won't re-fire on back-nav.
    // LaunchedEffect also reacts to isRunning so prevReady is cleared when
    // the profile stops, allowing vibration to fire again on next reconnect.
    val ctx = LocalContext.current
    var prevReady by rememberSaveable { mutableStateOf(false) }
    LaunchedEffect(isRunning, isReady) {
        if (!isRunning) {
            prevReady = false
        } else if (isReady && !prevReady) {
            try {
                @Suppress("DEPRECATION")
                val vibrator = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
                    (ctx.getSystemService(Context.VIBRATOR_MANAGER_SERVICE) as android.os.VibratorManager).defaultVibrator
                } else {
                    ctx.getSystemService(Context.VIBRATOR_SERVICE) as Vibrator
                }
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                    vibrator.vibrate(VibrationEffect.createOneShot(120, VibrationEffect.DEFAULT_AMPLITUDE))
                } else {
                    @Suppress("DEPRECATION")
                    vibrator.vibrate(120)
                }
            } catch (_: Exception) { /* vibrate is optional — never crash */ }
            prevReady = true
        }
    }

    if (showDeleteDialog) {
        AlertDialog(
            onDismissRequest = { showDeleteDialog = false },
            title = { Text("Delete profile?") },
            text = { Text("\"${profile.name}\" will be permanently removed.") },
            confirmButton = {
                TextButton(onClick = { showDeleteDialog = false; onDelete() }) { Text("Delete", color = RedError) }
            },
            dismissButton = {
                TextButton(onClick = { showDeleteDialog = false }) { Text("Cancel") }
            },
        )
    }

    GlassCard {
        Column {
            Row(verticalAlignment = Alignment.CenterVertically) {
                // Status indicator — blinks orange while scanning, solid green when ready
                val scanTransition = rememberInfiniteTransition(label = "dot_blink")
                val scanAlpha by scanTransition.animateFloat(
                    initialValue = 1f,
                    targetValue = 0.15f,
                    animationSpec = infiniteRepeatable(
                        animation = tween(500, easing = LinearEasing),
                        repeatMode = RepeatMode.Reverse,
                    ),
                    label = "dot_alpha",
                )
                val dotColor = when {
                    isBusy -> Color(0xFFFFAA00)
                    isScanning -> Color(0xFFFFAA00)
                    isReady -> GreenOnline
                    else -> TextSecondary.copy(alpha = 0.3f)
                }
                Icon(
                    Icons.Default.Circle,
                    contentDescription = null,
                    tint = if (isScanning) dotColor.copy(alpha = scanAlpha) else dotColor,
                    modifier = Modifier.size(12.dp),
                )
                Spacer(Modifier.width(10.dp))
                Column(modifier = Modifier.weight(1f)) {
                    Text(
                        profile.name,
                        style = MaterialTheme.typography.titleMedium,
                        fontWeight = FontWeight.SemiBold,
                        color = TextPrimary,
                    )
                    // Animated dots: Scanning. → Scanning.. → Scanning... → Scanning.
                    // Use InfiniteTransition instead of a while-loop coroutine to avoid
                    // allocating a suspended coroutine per profile card.
                    val scanDotsTransition = rememberInfiniteTransition(label = "scan_dots")
                    val scanDotsRaw by scanDotsTransition.animateFloat(
                        initialValue = 0f,
                        targetValue = 3f,
                        animationSpec = infiniteRepeatable(
                            animation = tween(1200, easing = LinearEasing),
                            repeatMode = RepeatMode.Restart,
                        ),
                        label = "scan_dots_idx",
                    )
                    val scanDots = if (isScanning) (scanDotsRaw.toInt() % 3) + 1 else 1
                    Text(
                        buildString {
                            append(profile.tunnelMode)
                            when {
                                isBusy -> append(" - Processing...")
                                isScanning -> append(" - Scanning" + ".".repeat(scanDots))
                                isReady -> append(" - Connected")
                                profile.identityLocked -> append(" - \uD83D\uDD12 Locked")
                                else -> { /* no extra info */ }
                            }
                        },
                        style = MaterialTheme.typography.bodySmall,
                        color = when {
                            isBusy -> Color(0xFFFFAA00)
                            isScanning -> Color(0xFFFFAA00)
                            isReady -> GreenOnline
                            else -> TextSecondary
                        },
                        fontSize = 11.sp,
                    )
                }
                // Usage badges — actual traffic + MasterDNS overhead (side by side)
                if (totalUsageBytes > 0 || wireTotalBytes > 0) {
                    Row(
                        horizontalArrangement = Arrangement.spacedBy(4.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        // Actual traffic (TUN — what apps really consumed)
                        if (totalUsageBytes > 0) {
                            Surface(
                                color = CyanAccent.copy(alpha = 0.12f),
                                shape = RoundedCornerShape(8.dp),
                            ) {
                                Text(
                                    "↕ ${formatBytes(totalUsageBytes)}",
                                    modifier = Modifier.padding(horizontal = 8.dp, vertical = 3.dp),
                                    color = CyanAccent,
                                    fontSize = 10.sp,
                                    fontWeight = FontWeight.Medium,
                                )
                            }
                        }
                        // MasterDNS overhead (wire - actual = extra from ARQ/duplication)
                        val overhead = (wireTotalBytes - totalUsageBytes).coerceAtLeast(0L)
                        if (overhead > 0) {
                            Surface(
                                color = Color(0xFFFFAB40).copy(alpha = 0.12f),
                                shape = RoundedCornerShape(8.dp),
                            ) {
                                Text(
                                    "+${formatBytes(overhead)}",
                                    modifier = Modifier.padding(horizontal = 8.dp, vertical = 3.dp),
                                    color = Color(0xFFFFAB40),
                                    fontSize = 10.sp,
                                    fontWeight = FontWeight.Medium,
                                )
                            }
                        }
                    }
                }
            }

            Spacer(Modifier.height(12.dp))

            // Monitor details when running
            if (isRunning && monitorInfo != null) {
                Surface(
                    color = DarkSurface.copy(alpha = 0.4f),
                    shape = RoundedCornerShape(8.dp),
                ) {
                    Row(
                        modifier = Modifier.fillMaxWidth().padding(horizontal = 10.dp, vertical = 6.dp),
                        horizontalArrangement = Arrangement.SpaceEvenly,
                    ) {
                        val stats = monitorInfo.stats
                        Column(horizontalAlignment = Alignment.CenterHorizontally) {
                            Text("Status", color = TextSecondary, fontSize = 9.sp)
                            Text(
                                if (stats?.sessionReady == true) "Ready" else "Init...",
                                color = if (stats?.sessionReady == true) GreenOnline else Color(0xFFFFAB40),
                                fontWeight = FontWeight.Medium,
                                fontSize = 12.sp,
                            )
                        }
                        Column(horizontalAlignment = Alignment.CenterHorizontally) {
                            Text("Resolvers", color = TextSecondary, fontSize = 9.sp)
                            val valid = stats?.validResolverCount ?: 0
                            val total = remember(profile.resolversText, profile.domains) {
                                val resolvers = profile.resolversText.lines().count { it.isNotBlank() }
                                val domainCount = profile.domains.split(",").count { it.isNotBlank() }.coerceAtLeast(1)
                                resolvers * domainCount
                            }
                            Text(
                                "$valid/$total",
                                color = if (valid > 0) GreenOnline else Color(0xFFFFAB40),
                                fontWeight = FontWeight.Medium,
                                fontSize = 12.sp,
                            )
                        }
                        Column(horizontalAlignment = Alignment.CenterHorizontally) {
                            Text("Listen", color = TextSecondary, fontSize = 9.sp)
                            Text(
                                stats?.listenAddr?.ifEmpty { "-" } ?: "-",
                                color = TextPrimary,
                                fontWeight = FontWeight.Medium,
                                fontSize = 12.sp,
                            )
                        }
                    }
                }
                if (!monitorInfo.lastError.isNullOrBlank()) {
                    Spacer(Modifier.height(4.dp))
                    Text(
                        monitorInfo.lastError,
                        color = RedError.copy(alpha = 0.8f),
                        fontSize = 10.sp,
                        maxLines = 2,
                    )
                }
                Spacer(Modifier.height(8.dp))
            }

            // Action buttons row — Start and Stop are ALWAYS visible, enabled/disabled based on state
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    // Start button — disabled when running or busy
                    Button(
                        onClick = { localBusy = true; onConnect() },
                        enabled = !isRunning && !effectiveBusy,
                        colors = ButtonDefaults.buttonColors(
                            containerColor = TealPrimary.copy(alpha = 0.2f),
                            contentColor = TealLight,
                            disabledContainerColor = TealPrimary.copy(alpha = 0.05f),
                            disabledContentColor = TealLight.copy(alpha = 0.3f),
                        ),
                        shape = RoundedCornerShape(12.dp),
                    ) {
                        if (effectiveBusy && !isRunning) {
                            CircularProgressIndicator(
                                modifier = Modifier.size(16.dp),
                                color = TealLight,
                                strokeWidth = 2.dp,
                            )
                        } else {
                            Icon(Icons.Default.PlayArrow, null, modifier = Modifier.size(18.dp))
                        }
                        Spacer(Modifier.width(4.dp))
                        Text("Start", fontSize = 13.sp)
                    }

                    // Stop button — disabled when NOT running or busy
                    Button(
                        onClick = { localBusy = true; onDisconnect() },
                        enabled = isRunning && !effectiveBusy,
                        colors = ButtonDefaults.buttonColors(
                            containerColor = RedError.copy(alpha = 0.15f),
                            contentColor = RedError,
                            disabledContainerColor = RedError.copy(alpha = 0.05f),
                            disabledContentColor = RedError.copy(alpha = 0.3f),
                        ),
                        shape = RoundedCornerShape(12.dp),
                    ) {
                        if (effectiveBusy && isRunning) {
                            CircularProgressIndicator(
                                modifier = Modifier.size(16.dp),
                                color = RedError,
                                strokeWidth = 2.dp,
                            )
                        } else {
                            Icon(Icons.Default.Stop, null, modifier = Modifier.size(18.dp))
                        }
                        Spacer(Modifier.width(4.dp))
                        Text("Stop", fontSize = 13.sp)
                    }
                }

                Row {
                    if (isRunning) {
                        IconButton(onClick = onDashboard) {
                            Icon(Icons.Default.BarChart, "Dashboard", tint = CyanAccent)
                        }
                    }
                    IconButton(onClick = onEdit) {
                        Icon(Icons.Default.Edit, "Edit", tint = TextSecondary)
                    }
                    IconButton(onClick = { showDeleteDialog = true }) {
                        Icon(Icons.Default.Delete, "Delete", tint = TextSecondary.copy(alpha = 0.6f))
                    }
                }
            }
        }
    }
}

@Composable
private fun MetaProfileCard(
    meta: MetaProfileEntity,
    isRunning: Boolean,
    isBusy: Boolean,
    isReady: Boolean,
    onStart: () -> Unit,
    onStop: () -> Unit,
    onEdit: () -> Unit,
    onDelete: () -> Unit,
) {
    var showDeleteDialog by remember { mutableStateOf(false) }
    var localBusy by remember { mutableStateOf(false) }
    LaunchedEffect(isRunning, isBusy) { localBusy = false }
    val effectiveBusy = localBusy || isBusy

    // Scanning = running but no sub-profile is sessionReady yet
    val isScanning = isRunning && !isReady

    if (showDeleteDialog) {
        AlertDialog(
            onDismissRequest = { showDeleteDialog = false },
            title = { Text("Delete meta profile?") },
            text = { Text("\"${meta.name}\" will be permanently removed.") },
            confirmButton = {
                TextButton(onClick = { showDeleteDialog = false; onDelete() }) { Text("Delete", color = RedError) }
            },
            dismissButton = {
                TextButton(onClick = { showDeleteDialog = false }) { Text("Cancel") }
            },
        )
    }

    GlassCard {
        Column {
            Row(verticalAlignment = Alignment.CenterVertically) {
                // Status dot — blinks orange while scanning, solid green when ready
                val scanTransition = rememberInfiniteTransition(label = "meta_dot_blink")
                val scanAlpha by scanTransition.animateFloat(
                    initialValue = 1f,
                    targetValue = 0.15f,
                    animationSpec = infiniteRepeatable(
                        animation = tween(600, easing = LinearEasing),
                        repeatMode = RepeatMode.Reverse,
                    ),
                    label = "meta_dot_alpha",
                )
                Icon(
                    Icons.Default.Circle,
                    contentDescription = null,
                    tint = when {
                        isScanning -> Color(0xFFFFAA00).copy(alpha = scanAlpha)
                        isBusy -> Color(0xFFFFAA00)
                        isReady -> GreenOnline
                        else -> TextSecondary.copy(alpha = 0.3f)
                    },
                    modifier = Modifier.size(12.dp),
                )
                Spacer(Modifier.width(8.dp))
                Icon(Icons.Default.AccountTree, "Meta", tint = CyanAccent)
                Spacer(Modifier.width(10.dp))
                Column(modifier = Modifier.weight(1f)) {
                    Text(
                        meta.name,
                        style = MaterialTheme.typography.titleMedium,
                        fontWeight = FontWeight.SemiBold,
                        color = TextPrimary,
                    )
                    val strategy = when (meta.balancingStrategy) {
                        1 -> "Random"
                        2 -> "Round-Robin"
                        3 -> "Least-Loss"
                        4 -> "Lowest-Latency"
                        else -> "Default"
                    }
                    val count = meta.profileIds.split(",").filter { it.isNotBlank() }.size

                    // Animated "Scanning…" text while waiting for sub-profiles
                    val scanDots by scanTransition.animateFloat(
                        initialValue = 0f,
                        targetValue = 3f,
                        animationSpec = infiniteRepeatable(
                            animation = tween(1200, easing = LinearEasing),
                            repeatMode = RepeatMode.Restart,
                        ),
                        label = "meta_scan_dots",
                    )
                    val dotsStr = ".".repeat(scanDots.toInt() + 1)

                    Text(
                        buildString {
                            append("${meta.tunnelMode} - $strategy - $count profiles")
                            if (isScanning) append(" - Scanning$dotsStr")
                            else if (isBusy) append(" - Processing...")
                            else if (isReady) append(" - Active")
                        },
                        style = MaterialTheme.typography.bodySmall,
                        color = when {
                            isScanning -> Color(0xFFFFAA00)
                            isBusy -> Color(0xFFFFAA00)
                            else -> TextSecondary
                        },
                        fontSize = 11.sp,
                    )
                }
            }

            Spacer(Modifier.height(12.dp))

            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    Button(
                        onClick = { localBusy = true; onStart() },
                        enabled = !isRunning && !effectiveBusy,
                        colors = ButtonDefaults.buttonColors(
                            containerColor = CyanAccent.copy(alpha = 0.2f),
                            contentColor = CyanAccent,
                            disabledContainerColor = CyanAccent.copy(alpha = 0.05f),
                            disabledContentColor = CyanAccent.copy(alpha = 0.3f),
                        ),
                        shape = RoundedCornerShape(12.dp),
                    ) {
                        if (effectiveBusy && !isRunning) {
                            CircularProgressIndicator(
                                modifier = Modifier.size(16.dp),
                                color = CyanAccent,
                                strokeWidth = 2.dp,
                            )
                        } else {
                            Icon(Icons.Default.PlayArrow, null, modifier = Modifier.size(18.dp))
                        }
                        Spacer(Modifier.width(4.dp))
                        Text("Start", fontSize = 13.sp)
                    }

                    Button(
                        onClick = { localBusy = true; onStop() },
                        enabled = isRunning && !effectiveBusy,
                        colors = ButtonDefaults.buttonColors(
                            containerColor = RedError.copy(alpha = 0.15f),
                            contentColor = RedError,
                            disabledContainerColor = RedError.copy(alpha = 0.05f),
                            disabledContentColor = RedError.copy(alpha = 0.3f),
                        ),
                        shape = RoundedCornerShape(12.dp),
                    ) {
                        if (effectiveBusy && isRunning) {
                            CircularProgressIndicator(
                                modifier = Modifier.size(16.dp),
                                color = RedError,
                                strokeWidth = 2.dp,
                            )
                        } else {
                            Icon(Icons.Default.Stop, null, modifier = Modifier.size(18.dp))
                        }
                        Spacer(Modifier.width(4.dp))
                        Text("Stop", fontSize = 13.sp)
                    }
                }

                Row {
                    IconButton(onClick = onEdit) {
                        Icon(Icons.Default.Edit, "Edit", tint = TextSecondary)
                    }
                    IconButton(onClick = { showDeleteDialog = true }) {
                        Icon(Icons.Default.Delete, "Delete", tint = TextSecondary.copy(alpha = 0.6f))
                    }
                }
            }
        }
    }
}