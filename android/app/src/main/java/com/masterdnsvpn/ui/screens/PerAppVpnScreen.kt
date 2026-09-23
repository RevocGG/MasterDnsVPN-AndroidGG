package com.masterdnsvpn.ui.screens

import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Clear
import androidx.compose.material.icons.filled.Search
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import com.masterdnsvpn.R
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
                    title = { Text(stringResource(R.string.settings_per_app_vpn), fontWeight = FontWeight.Bold, color = TealLight) },
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
                    .padding(horizontal = 16.dp)
                    .fillMaxSize(),
            ) {
                // ── Mode selector ───────────────────────────────────────────────
                GlassCard {
                    Column(verticalArrangement = Arrangement.spacedBy(0.dp)) {
                        Text(stringResource(R.string.perapp_mode), color = TealLight, fontWeight = FontWeight.SemiBold, fontSize = 13.sp)
                        Spacer(Modifier.height(2.dp))
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
                                    modifier = Modifier.size(32.dp),
                                )
                                Text(
                                    when (mode) {
                                        AppSelectionPrefs.Mode.ALL -> stringResource(R.string.perapp_all_apps)
                                        AppSelectionPrefs.Mode.INCLUDE -> stringResource(R.string.perapp_include)
                                        AppSelectionPrefs.Mode.EXCLUDE -> stringResource(R.string.perapp_exclude)
                                    },
                                    color = TextPrimary,
                                    fontSize = 13.sp,
                                )
                            }
                        }
                    }
                }

                Spacer(Modifier.height(6.dp))

                // ── Search bar ──────────────────────────────────────────────────
                OutlinedTextField(
                    value = state.searchQuery,
                    onValueChange = { vm.setSearchQuery(it) },
                    placeholder = { Text(stringResource(R.string.perapp_search), color = TextHint, fontSize = 13.sp) },
                    leadingIcon = { Icon(Icons.Default.Search, null, tint = TextSecondary, modifier = Modifier.size(18.dp)) },
                    trailingIcon = {
                        if (state.searchQuery.isNotBlank()) {
                            IconButton(onClick = { vm.setSearchQuery("") }, modifier = Modifier.size(36.dp)) {
                                Icon(Icons.Default.Clear, null, tint = TextSecondary, modifier = Modifier.size(16.dp))
                            }
                        }
                    },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth().height(50.dp),
                    shape = RoundedCornerShape(10.dp),
                    colors = OutlinedTextFieldDefaults.colors(
                        focusedBorderColor = TealPrimary,
                        unfocusedBorderColor = GlassBorder,
                        cursorColor = TealLight,
                        focusedTextColor = TextPrimary,
                        unfocusedTextColor = TextPrimary,
                    ),
                    textStyle = androidx.compose.ui.text.TextStyle(fontSize = 13.sp),
                )

                Spacer(Modifier.height(2.dp))

                // ── Controls row: system toggle + count + select/clear ──────────
                val enabled = state.mode != AppSelectionPrefs.Mode.ALL
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.spacedBy(4.dp),
                ) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Checkbox(
                            checked = state.showSystemApps,
                            onCheckedChange = { vm.toggleSystemApps(it) },
                            colors = CheckboxDefaults.colors(
                                checkedColor = TealPrimary,
                                uncheckedColor = TextSecondary,
                                checkmarkColor = DarkBg,
                            ),
                            modifier = Modifier.size(32.dp),
                        )
                        Text(stringResource(R.string.perapp_system), color = TextSecondary, fontSize = 12.sp)
                    }
                    Text(
                        stringResource(R.string.perapp_sel_count, state.apps.count { it.selected }),
                        color = TealLight,
                        fontSize = 12.sp,
                    )
                    Spacer(Modifier.weight(1f))
                    OutlinedButton(
                        onClick = { vm.selectAll() },
                        enabled = enabled,
                        modifier = Modifier.height(30.dp),
                        shape = RoundedCornerShape(6.dp),
                        contentPadding = PaddingValues(horizontal = 10.dp, vertical = 0.dp),
                        colors = ButtonDefaults.outlinedButtonColors(contentColor = CyanAccent),
                        border = androidx.compose.foundation.BorderStroke(
                            1.dp, if (enabled) CyanAccent else TextSecondary.copy(alpha = 0.3f),
                        ),
                    ) { Text(stringResource(R.string.perapp_all), fontSize = 12.sp) }
                    OutlinedButton(
                        onClick = { vm.deselectAll() },
                        enabled = enabled,
                        modifier = Modifier.height(30.dp),
                        shape = RoundedCornerShape(6.dp),
                        contentPadding = PaddingValues(horizontal = 10.dp, vertical = 0.dp),
                        colors = ButtonDefaults.outlinedButtonColors(contentColor = TextSecondary),
                        border = androidx.compose.foundation.BorderStroke(
                            1.dp, if (enabled) TextSecondary else TextSecondary.copy(alpha = 0.3f),
                        ),
                    ) { Text(stringResource(R.string.perapp_none), fontSize = 12.sp) }
                    // Save turns BRIGHT when there are unsaved changes so the
                    // user can tell at a glance they must save. When saved it
                    // dims down but stays tappable (re-save is harmless).
                    val hasUnsavedChanges = !state.savedToPrefs
                    Button(
                        onClick = { vm.save() },
                        enabled = true,
                        modifier = Modifier.height(30.dp),
                        border = if (hasUnsavedChanges) {
                            androidx.compose.foundation.BorderStroke(1.dp, CyanAccent)
                        } else null,
                        colors = ButtonDefaults.buttonColors(
                            containerColor = if (hasUnsavedChanges) CyanAccent else TealPrimary.copy(alpha = 0.3f),
                            contentColor = if (hasUnsavedChanges) DarkBg else TextSecondary,
                        ),
                        shape = RoundedCornerShape(6.dp),
                        contentPadding = PaddingValues(horizontal = 12.dp, vertical = 0.dp),
                    ) {
                        Text(
                            if (state.savedToPrefs) stringResource(R.string.perapp_saved)
                            else stringResource(R.string.perapp_save),
                            fontSize = 12.sp,
                            fontWeight = FontWeight.SemiBold,
                        )
                    }
                }

                Spacer(Modifier.height(4.dp))

                // ── App list ────────────────────────────────────────────────────
                if (state.loading) {
                    Box(
                        modifier = Modifier.weight(1f).fillMaxWidth(),
                        contentAlignment = Alignment.Center,
                    ) {
                        CircularProgressIndicator(color = TealPrimary)
                    }
                } else {
                    // Selected apps float to the top of the list so the user
                    // immediately sees (and can unselect) what is routed.
                    val orderedApps = remember(state.apps, state.searchQuery) {
                        state.apps.sortedByDescending { it.selected }
                    }
                    LazyColumn(
                        modifier = Modifier.weight(1f),
                        verticalArrangement = Arrangement.spacedBy(2.dp),
                    ) {
                        if (orderedApps.any { it.selected }) {
                            item(key = "selected_header") {
                                Text(
                                    stringResource(R.string.perapp_selected_section),
                                    color = TealLight,
                                    fontWeight = FontWeight.SemiBold,
                                    fontSize = 12.sp,
                                    modifier = Modifier.padding(vertical = 4.dp),
                                )
                            }
                        }
                        items(orderedApps, key = { it.packageName }) { app ->
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
