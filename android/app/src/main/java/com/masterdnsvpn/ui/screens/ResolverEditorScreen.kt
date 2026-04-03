package com.masterdnsvpn.ui.screens

import android.net.Uri
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.*
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.FileOpen
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import com.masterdnsvpn.ui.theme.*
import com.masterdnsvpn.ui.viewmodel.ResolverEditorViewModel

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ResolverEditorScreen(
    profileId: String,
    onNavigateUp: () -> Unit,
    vm: ResolverEditorViewModel = hiltViewModel(),
) {
    val resolversText by vm.resolversText.collectAsState()
    val saved by vm.saved.collectAsState()
    val ctx = LocalContext.current

    LaunchedEffect(saved) { if (saved) onNavigateUp() }

    val filePickerLauncher = rememberLauncherForActivityResult(
        contract = ActivityResultContracts.OpenDocument()
    ) { uri: Uri? ->
        uri ?: return@rememberLauncherForActivityResult
        try {
            val text = ctx.contentResolver.openInputStream(uri)?.bufferedReader()?.readText() ?: ""
            vm.updateText(text)
        } catch (_: Exception) { }
    }

    GlassBackground {
        Scaffold(
            containerColor = androidx.compose.ui.graphics.Color.Transparent,
            topBar = {
                TopAppBar(
                    title = { Text("Resolver List", color = TealLight) },
                    colors = TopAppBarDefaults.topAppBarColors(
                        containerColor = androidx.compose.ui.graphics.Color.Transparent,
                    ),
                    navigationIcon = {
                        IconButton(onClick = onNavigateUp) {
                            Icon(Icons.AutoMirrored.Filled.ArrowBack, "Back", tint = TextSecondary)
                        }
                    },
                    actions = {
                        IconButton(onClick = { filePickerLauncher.launch(arrayOf("text/*", "*/*")) }) {
                            Icon(Icons.Default.FileOpen, "Import file", tint = CyanAccent)
                        }
                        IconButton(onClick = { vm.save() }) {
                            Icon(Icons.Default.Check, "Save", tint = GreenOnline)
                        }
                    },
                )
            }
        ) { padding ->
            Column(
                modifier = Modifier
                    .padding(padding)
                    .padding(16.dp)
                    .fillMaxSize(),
            ) {
                Text(
                    "One resolver per line. Import from client_resolvers.txt or enter manually.\nFormats: IP, IP:port, CIDR, CIDR:port, [IPv6], [IPv6]:port",
                    style = MaterialTheme.typography.bodySmall,
                    color = TextSecondary,
                )
                Spacer(Modifier.height(12.dp))
                OutlinedTextField(
                    value = resolversText,
                    onValueChange = { vm.updateText(it) },
                    modifier = Modifier
                        .fillMaxWidth()
                        .weight(1f),
                    textStyle = MaterialTheme.typography.bodySmall.copy(
                        fontFamily = FontFamily.Monospace,
                        color = TextPrimary,
                    ),
                    placeholder = { Text("8.8.8.8\n1.1.1.1:53\n192.168.1.0/24:5353", color = TextHint.copy(alpha = 0.4f)) },
                )
            }
        }
    }
}