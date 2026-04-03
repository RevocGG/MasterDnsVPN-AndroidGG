package com.masterdnsvpn.ui.screens

import android.graphics.Bitmap
import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.CheckBox
import androidx.compose.material.icons.filled.CheckBoxOutlineBlank
import androidx.compose.material.icons.filled.Clear
import androidx.compose.material.icons.filled.Search
import androidx.compose.material.icons.filled.SelectAll
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.core.graphics.drawable.toBitmap
import androidx.hilt.navigation.compose.hiltViewModel
import com.masterdnsvpn.settings.AppSelectionPrefs
import com.masterdnsvpn.ui.theme.*
import com.masterdnsvpn.ui.viewmodel.AppItem
import com.masterdnsvpn.ui.viewmodel.SettingsViewModel
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsScreen(
    vm: SettingsViewModel = hiltViewModel(),
) {
    val state by vm.state.collectAsState()
    val ctx = LocalContext.current

    GlassBackground {
        Scaffold(
            containerColor = androidx.compose.ui.graphics.Color.Transparent,
            topBar = {
                TopAppBar(
                    title = { Text("Settings", fontWeight = FontWeight.Bold, color = TealLight) },
                    colors = TopAppBarDefaults.topAppBarColors(
                        containerColor = androidx.compose.ui.graphics.Color.Transparent,
                    ),
                    actions = {
                        IconButton(onClick = { vm.selectAll() }) {
                            Icon(Icons.Default.SelectAll, "Select All", tint = CyanAccent)
                        }
                        IconButton(onClick = { vm.deselectAll() }) {
                            Icon(Icons.Default.Clear, "Deselect All", tint = TextSecondary)
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
                // Mode selector
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
                    }
                }

                Spacer(Modifier.height(8.dp))

                // Search bar
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

                // Show system apps toggle
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

                Spacer(Modifier.height(4.dp))

                if (state.loading) {
                    Box(
                        modifier = Modifier.fillMaxSize(),
                        contentAlignment = Alignment.Center,
                    ) {
                        CircularProgressIndicator(color = TealPrimary)
                    }
                } else {
                    // Disable list when mode is ALL
                    val enabled = state.mode != AppSelectionPrefs.Mode.ALL

                    LazyColumn(
                        verticalArrangement = Arrangement.spacedBy(2.dp),
                    ) {
                        items(state.apps, key = { it.packageName }) { app ->
                            AppRow(
                                app = app,
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
private fun AppRow(
    app: AppItem,
    enabled: Boolean,
    onToggle: () -> Unit,
) {
    val ctx = LocalContext.current
    // Load icon asynchronously on IO thread — prevents UI jank / ANR with large app lists
    val icon by produceState<Bitmap?>(initialValue = null, app.packageName) {
        value = withContext(Dispatchers.IO) {
            try {
                ctx.packageManager.getApplicationIcon(app.packageName).toBitmap(40, 40)
            } catch (_: Exception) { null }
        }
    }

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
            // App icon
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
