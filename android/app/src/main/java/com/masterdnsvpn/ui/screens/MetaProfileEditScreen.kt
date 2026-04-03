package com.masterdnsvpn.ui.screens

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Check
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import com.masterdnsvpn.ui.theme.*
import com.masterdnsvpn.ui.viewmodel.MetaProfileEditViewModel

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun MetaProfileEditScreen(
    metaId: String,
    onNavigateUp: () -> Unit,
    vm: MetaProfileEditViewModel = hiltViewModel(),
) {
    val meta by vm.meta.collectAsState()
    val allProfiles by vm.allProfiles.collectAsState()
    val saved by vm.saved.collectAsState()
    var snackMsg by remember { mutableStateOf<String?>(null) }

    LaunchedEffect(saved) {
        if (saved) {
            onNavigateUp()
        }
    }

    val selectedIds = meta.profileIds.split(",").filter { it.isNotBlank() }.toSet()
    var socksPortText by remember(meta.id) { mutableStateOf(if (meta.socksPort == 0) "" else meta.socksPort.toString()) }

    val strategies = listOf(
        0 to "RR Default",
        1 to "Random",
        2 to "Round-Robin",
        3 to "Least-Loss",
        4 to "Lowest-Latency",
    )

    GlassBackground {
        Scaffold(
            containerColor = androidx.compose.ui.graphics.Color.Transparent,
            topBar = {
                TopAppBar(
                    title = { Text(if (metaId == "new") "New Meta Profile" else "Edit Meta Profile", color = TealLight) },
                    colors = TopAppBarDefaults.topAppBarColors(
                        containerColor = androidx.compose.ui.graphics.Color.Transparent,
                    ),
                    navigationIcon = {
                        IconButton(onClick = onNavigateUp) {
                            Icon(Icons.AutoMirrored.Filled.ArrowBack, "Back", tint = TextSecondary)
                        }
                    },
                    actions = {
                        IconButton(onClick = {
                            if (meta.name.isBlank()) {
                                snackMsg = "Please enter a name"
                            } else {
                                vm.save()
                            }
                        }) {
                            Icon(Icons.Default.Check, "Save", tint = GreenOnline)
                        }
                    },
                )
            },
            snackbarHost = {
                snackMsg?.let { msg ->
                    LaunchedEffect(msg) {
                        kotlinx.coroutines.delay(2000)
                        snackMsg = null
                    }
                    Snackbar(modifier = Modifier.padding(16.dp)) {
                        Text(msg)
                    }
                }
            },
        ) { padding ->
            LazyColumn(
                modifier = Modifier.padding(padding).padding(16.dp),
                verticalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                item {
                    OutlinedTextField(
                        value = meta.name,
                        onValueChange = { vm.update { copy(name = it) } },
                        label = { Text("Meta Profile Name") },
                        modifier = Modifier.fillMaxWidth(),
                        singleLine = true,
                        textStyle = MaterialTheme.typography.bodyLarge.copy(color = TextPrimary),
                    )
                }

                item {
                    Spacer(Modifier.height(4.dp))
                    Text("Tunnel Mode", style = MaterialTheme.typography.titleSmall, color = TextSecondary)
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        listOf("SOCKS5", "TUN").forEach { mode ->
                            Row(verticalAlignment = Alignment.CenterVertically) {
                                RadioButton(
                                    selected = meta.tunnelMode == mode,
                                    onClick = { vm.update { copy(tunnelMode = mode) } },
                                    colors = RadioButtonDefaults.colors(
                                        selectedColor = TealPrimary,
                                        unselectedColor = TextSecondary,
                                    ),
                                )
                                Text(mode, style = MaterialTheme.typography.bodyMedium, color = TextPrimary)
                                Spacer(Modifier.width(16.dp))
                            }
                        }
                    }
                }

                item {
                    Spacer(Modifier.height(4.dp))
                    OutlinedTextField(
                        value = socksPortText,
                        onValueChange = { v ->
                            socksPortText = v.filter { it.isDigit() }.take(5)
                        },
                        label = { Text("SOCKS Output Port (0 = auto-assign)") },
                        placeholder = { Text("e.g. 1080") },
                        modifier = Modifier.fillMaxWidth().onFocusChanged { focus ->
                            if (!focus.isFocused) {
                                val port = socksPortText.toIntOrNull() ?: 0
                                vm.update { copy(socksPort = port.coerceIn(0, 65535)) }
                            }
                        },
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                        singleLine = true,
                        textStyle = MaterialTheme.typography.bodyLarge.copy(color = TextPrimary),
                        supportingText = { Text("Leave 0 to let the OS assign a port automatically", color = TextSecondary) },
                    )
                }

                item {
                    Spacer(Modifier.height(4.dp))
                    Text("Balancing Strategy", style = MaterialTheme.typography.titleSmall, color = TextSecondary)
                    Column {
                        strategies.forEach { (value, label) ->
                            Row(verticalAlignment = Alignment.CenterVertically) {
                                RadioButton(
                                    selected = meta.balancingStrategy == value,
                                    onClick = { vm.update { copy(balancingStrategy = value) } },
                                    colors = RadioButtonDefaults.colors(
                                        selectedColor = TealPrimary,
                                        unselectedColor = TextSecondary,
                                    ),
                                )
                                Text(label, style = MaterialTheme.typography.bodyMedium, color = TextPrimary)
                            }
                        }
                    }
                }

                item {
                    Spacer(Modifier.height(4.dp))
                    Text("Sub-Profiles", style = MaterialTheme.typography.titleSmall, color = TextSecondary)
                }

                if (allProfiles.isEmpty()) {
                    item {
                        Text(
                            "No profiles created yet. Create profiles first.",
                            style = MaterialTheme.typography.bodySmall,
                            color = TextSecondary,
                        )
                    }
                }

                items(allProfiles, key = { it.id }) { profile ->
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        modifier = Modifier.fillMaxWidth(),
                    ) {
                        Checkbox(
                            checked = selectedIds.contains(profile.id),
                            onCheckedChange = { vm.toggleProfile(profile.id) },
                            colors = CheckboxDefaults.colors(
                                checkedColor = TealPrimary,
                                uncheckedColor = TextSecondary,
                            ),
                        )
                        Column {
                            Text(profile.name, style = MaterialTheme.typography.bodyMedium, color = TextPrimary)
                            Text(
                                profile.tunnelMode,
                                style = MaterialTheme.typography.bodySmall,
                                color = TextSecondary,
                            )
                        }
                    }
                }
            }
        }
    }
}