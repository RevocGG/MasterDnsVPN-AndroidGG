package com.masterdnsvpn.ui.screens

import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Clear
import androidx.compose.material.icons.filled.Info
import androidx.compose.material.icons.filled.Save
import androidx.compose.material.icons.filled.Search
import androidx.compose.material.icons.filled.SelectAll
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.hilt.navigation.compose.hiltViewModel
import com.masterdnsvpn.settings.AppSelectionPrefs
import com.masterdnsvpn.ui.theme.*
import com.masterdnsvpn.ui.viewmodel.AppItem
import com.masterdnsvpn.ui.viewmodel.SettingsViewModel

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PerAppVpnScreen(
    onNavigateUp: () -> Unit,
    vm: SettingsViewModel = hiltViewModel(),
) {
    val state by vm.state.collectAsState()

    GlassBackground {
        Scaffold(
            containerColor = androidx.compose.ui.graphics.Color.Transparent,
            topBar = {
                TopAppBar(
                    title = { Text("Per-App VPN", fontWeight = FontWeight.Bold, color = TealLight) },
                    colors = TopAppBarDefaults.topAppBarColors(
                        containerColor = androidx.compose.ui.graphics.Color.Transparent,
                    ),
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
                    .padding(horizontal = 16.dp),
            ) {
                // ── TUN-only info banner ────────────────────────────────────────
                Surface(
                    color = TealPrimary.copy(alpha = 0.15f),
                    shape = RoundedCornerShape(10.dp),
                    modifier = Modifier.fillMaxWidth(),
                ) {
                    Row(
                        modifier = Modifier.padding(horizontal = 12.dp, vertical = 8.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Icon(Icons.Default.Info, null, tint = TealLight, modifier = Modifier.size(16.dp))
                        Spacer(Modifier.width(8.dp))
                        Text(
                            "These settings only apply in TUN mode. They have no effect in SOCKS5 mode.",
                            color = TealLight,
                            fontSize = 12.sp,
                        )
                    }
                }

                Spacer(Modifier.height(8.dp))

                // ── Mode selector ───────────────────────────────────────────────
                GlassCard {
                    Column {
                        Text("Per-App VPN Mode", color = TealLight, fontWeight = FontWeight.SemiBold)
                        Spacer(Modifier.height(8.dp))

                        AppSelectionPrefs.Mode.entries.forEach { mode ->
                            Row(
                                verticalAlignment = Alignment.CenterVertically,
                                modifier = Modifier.fillMaxWidth(),
                            ) {
                                RadioButton(
                                    selected = state.mode == mode,
                                    onClick = { vm.setMode(mode) },
                                    colors = RadioButtonDefaults.colors(
                                        selectedColor = TealLight,
                                        unselectedColor = TextSecondary,
                                    ),
                                )
                                Text(
                                    when (mode) {
                                        AppSelectionPrefs.Mode.ALL -> "All Apps (no filter)"
                                        AppSelectionPrefs.Mode.INCLUDE -> "Only selected apps use VPN"
                                        AppSelectionPrefs.Mode.EXCLUDE -> "All apps EXCEPT selected"
                                    },
                                    color = TextPrimary,
                                    fontSize = 14.sp,
                                )
                            }
                        }

                        Spacer(Modifier.height(8.dp))

                        // ── Save button ─────────────────────────────────────────
                        Button(
                            onClick = { vm.save() },
                            modifier = Modifier.fillMaxWidth(),
                            colors = ButtonDefaults.buttonColors(
                                containerColor = if (state.savedToPrefs) TealPrimary.copy(alpha = 0.5f) else TealPrimary,
                            ),
                            shape = RoundedCornerShape(10.dp),
                        ) {
                            Icon(Icons.Default.Save, null, modifier = Modifier.size(16.dp))
                            Spacer(Modifier.width(6.dp))
                            Text(
                                if (state.savedToPrefs) "Saved \u2713" else "Save Settings",
                                fontSize = 14.sp,
                                fontWeight = FontWeight.SemiBold,
                            )
                        }
                    }
                }

                Spacer(Modifier.height(8.dp))

                // ── Search bar ──────────────────────────────────────────────────
                OutlinedTextField(
                    value = state.searchQuery,
                    onValueChange = { vm.setSearchQuery(it) },
                    placeholder = { Text("Search apps...", color = TextHint) },
                    leadingIcon = { Icon(Icons.Default.Search, null, tint = TextSecondary) },
                    trailingIcon = {
                        if (state.searchQuery.isNotBlank()) {
                            IconButton(onClick = { vm.setSearchQuery("") }) {
                                Icon(Icons.Default.Clear, null, tint = TextSecondary)
                            }
                        }
                    },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                    shape = RoundedCornerShape(12.dp),
                    colors = OutlinedTextFieldDefaults.colors(
                        focusedBorderColor = TealPrimary,
                        unfocusedBorderColor = GlassBorder,
                        cursorColor = TealLight,
                        focusedTextColor = TextPrimary,
                        unfocusedTextColor = TextPrimary,
                    ),
                )

                Spacer(Modifier.height(4.dp))

                // ── Show system apps + selected count ───────────────────────────
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    modifier = Modifier.fillMaxWidth(),
                ) {
                    Checkbox(
                        checked = state.showSystemApps,
                        onCheckedChange = { vm.toggleSystemApps(it) },
                        colors = CheckboxDefaults.colors(
                            checkedColor = TealPrimary,
                            uncheckedColor = TextSecondary,
                            checkmarkColor = DarkBg,
                        ),
                    )
                    Text("Show system apps", color = TextSecondary, fontSize = 13.sp)
                    Spacer(Modifier.weight(1f))
                    Text("${state.apps.count { it.selected }} selected", color = TealLight, fontSize = 13.sp)
                }

                // ── Select All / Clear row ──────────────────────────────────────
                val enabled = state.mode != AppSelectionPrefs.Mode.ALL
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    OutlinedButton(
                        onClick = { vm.selectAll() },
                        enabled = enabled,
                        modifier = Modifier.weight(1f),
                        shape = RoundedCornerShape(8.dp),
                        colors = ButtonDefaults.outlinedButtonColors(contentColor = CyanAccent),
                        border = androidx.compose.foundation.BorderStroke(
                            1.dp,
                            if (enabled) CyanAccent else TextSecondary.copy(alpha = 0.3f),
                        ),
                    ) {
                        Icon(Icons.Default.SelectAll, null, modifier = Modifier.size(16.dp))
                        Spacer(Modifier.width(4.dp))
                        Text("Select All", fontSize = 13.sp)
                    }
                    OutlinedButton(
                        onClick = { vm.deselectAll() },
                        enabled = enabled,
                        modifier = Modifier.weight(1f),
                        shape = RoundedCornerShape(8.dp),
                        colors = ButtonDefaults.outlinedButtonColors(contentColor = TextSecondary),
                        border = androidx.compose.foundation.BorderStroke(
                            1.dp,
                            if (enabled) TextSecondary else TextSecondary.copy(alpha = 0.3f),
                        ),
                    ) {
                        Icon(Icons.Default.Clear, null, modifier = Modifier.size(16.dp))
                        Spacer(Modifier.width(4.dp))
                        Text("Clear All", fontSize = 13.sp)
                    }
                }

                Spacer(Modifier.height(4.dp))

                // ── App list ────────────────────────────────────────────────────
                if (state.loading) {
                    Box(
                        modifier = Modifier.fillMaxSize(),
                        contentAlignment = Alignment.Center,
                    ) {
                        CircularProgressIndicator(color = TealPrimary)
                    }
                } else {
                    LazyColumn(
                        verticalArrangement = Arrangement.spacedBy(2.dp),
                    ) {
                        items(state.apps, key = { it.packageName }) { app ->
                            AppRowItem(
                                app = app,
                                iconCache = vm.iconCache,
                                enabled = enabled,
                                onToggle = { vm.toggleApp(app.packageName) },
                            )
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun AppRowItem(
    app: AppItem,
    iconCache: java.util.concurrent.ConcurrentHashMap<String, android.graphics.Bitmap?>,
    enabled: Boolean,
    onToggle: () -> Unit,
) {
    // Read from pre-warmed cache — no per-item coroutine or allocation.
    val icon: android.graphics.Bitmap? = iconCache[app.packageName]

    Surface(
        onClick = { if (enabled) onToggle() },
        color = if (app.selected && enabled) GlassBg else androidx.compose.ui.graphics.Color.Transparent,
        shape = RoundedCornerShape(8.dp),
    ) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 8.dp, vertical = 6.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            icon?.let {
                Image(
                    bitmap = it.asImageBitmap(),
                    contentDescription = null,
                    modifier = Modifier
                        .size(36.dp)
                        .clip(RoundedCornerShape(8.dp)),
                )
            } ?: Box(Modifier.size(36.dp))

            Spacer(Modifier.width(12.dp))

            Column(modifier = Modifier.weight(1f)) {
                Text(
                    app.label,
                    color = if (enabled) TextPrimary else TextSecondary,
                    fontSize = 14.sp,
                    maxLines = 1,
                )
                Text(
                    app.packageName,
                    color = TextSecondary.copy(alpha = 0.6f),
                    fontSize = 11.sp,
                    maxLines = 1,
                )
            }

            Checkbox(
                checked = app.selected,
                onCheckedChange = { if (enabled) onToggle() },
                enabled = enabled,
                colors = CheckboxDefaults.colors(
                    checkedColor = TealPrimary,
                    uncheckedColor = TextSecondary,
                    checkmarkColor = DarkBg,
                ),
            )
        }
    }
}
