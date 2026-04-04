package com.masterdnsvpn.ui.screens

import androidx.compose.foundation.layout.*
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.masterdnsvpn.ui.theme.*
import com.masterdnsvpn.ui.viewmodel.DashboardViewModel

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun DashboardScreen(
    profileId: String,
    onNavigateUp: () -> Unit,
    vm: DashboardViewModel = hiltViewModel(),
) {
    val stats by vm.stats.collectAsStateWithLifecycle()

    GlassBackground {
        Scaffold(
            containerColor = androidx.compose.ui.graphics.Color.Transparent,
            topBar = {
                TopAppBar(
                    title = { Text("Dashboard", color = TealLight) },
                    colors = TopAppBarDefaults.topAppBarColors(
                        containerColor = androidx.compose.ui.graphics.Color.Transparent,
                    ),
                    navigationIcon = {
                        IconButton(onClick = onNavigateUp) {
                            Icon(Icons.AutoMirrored.Filled.ArrowBack, "Back", tint = TextSecondary)
                        }
                    },
                )
            }
        ) { padding ->
            Column(
                modifier = Modifier
                    .padding(padding)
                    .padding(16.dp),
                verticalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                if (stats == null) {
                    GlassCard {
                        Text("Not running", color = RedError)
                    }
                } else {
                    val s = stats!!  // capture delegated property into local val for smart cast
                    StatGlassCard("Status", if (s.isRunning) "Running" else "Stopped", s.isRunning)
                    StatGlassCard("Session Ready", if (s.sessionReady) "Yes" else "No", s.sessionReady)
                    StatGlassCard("Resolvers", "${s.validResolverCount} valid / ${s.resolverCount} total", s.validResolverCount > 0)
                    StatGlassCard("Listen Address", s.listenAddr.takeIf { it.isNotEmpty() } ?: "-", true)
                }
            }
        }
    }
}

@Composable
private fun StatGlassCard(label: String, value: String, isGood: Boolean) {
    GlassCard {
        Column {
            Text(
                label,
                style = MaterialTheme.typography.labelMedium,
                color = TextSecondary,
            )
            Spacer(Modifier.height(4.dp))
            Text(
                value,
                style = MaterialTheme.typography.bodyLarge,
                color = if (isGood) GreenOnline else AmberWarn,
            )
        }
    }
}