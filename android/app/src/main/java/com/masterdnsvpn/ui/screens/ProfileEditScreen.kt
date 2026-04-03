package com.masterdnsvpn.ui.screens

import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Edit
import androidx.compose.material.icons.filled.FileOpen
import androidx.compose.material.icons.filled.IosShare
import androidx.compose.material.icons.filled.KeyboardArrowDown
import androidx.compose.material.icons.filled.KeyboardArrowUp
import androidx.compose.material.icons.filled.Visibility
import androidx.compose.material.icons.filled.VisibilityOff
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import com.masterdnsvpn.profile.ProfileEntity
import com.masterdnsvpn.ui.Screen
import com.masterdnsvpn.ui.viewmodel.ProfileEditViewModel

/**
 * Full profile configuration screen.
 *
 * All 13 config sections are present.  Advanced sections are collapsed behind
 * [ExpandableSection] to avoid overwhelming new users.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ProfileEditScreen(
    profileId: String,
    onNavigateUp: () -> Unit,
    onEditResolvers: (String) -> Unit,
    vm: ProfileEditViewModel = hiltViewModel(),
) {
    val profile by vm.profile.collectAsState()
    val saved by vm.saved.collectAsState()
    val resolverNavId by vm.resolverNavId.collectAsState()
    val importError by vm.importError.collectAsState()
    var saveBusy by remember { mutableStateOf(false) }
    val focusManager = LocalFocusManager.current
    val context = LocalContext.current
    val snackbarHostState = remember { SnackbarHostState() }

    // Import: pick a text file → read contents → hand to VM
    val importLauncher = rememberLauncherForActivityResult(
        ActivityResultContracts.GetContent()
    ) { uri ->
        uri ?: return@rememberLauncherForActivityResult
        try {
            val text = context.contentResolver.openInputStream(uri)
                ?.bufferedReader()?.readText() ?: ""
            vm.importFromToml(text)
        } catch (e: Exception) {
            // surface as a snackbar — importError handles semantic errors
        }
    }

    // Export: open SAF file-save dialog → write toml content
    val exportLauncher = rememberLauncherForActivityResult(
        ActivityResultContracts.CreateDocument("text/plain")
    ) { uri ->
        uri ?: return@rememberLauncherForActivityResult
        try {
            val toml = vm.exportToToml()
            context.contentResolver.openOutputStream(uri)
                ?.bufferedWriter()?.use { it.write(toml) }
        } catch (_: Exception) { }
    }

    // Show import error as snackbar
    LaunchedEffect(importError) {
        importError?.let {
            snackbarHostState.showSnackbar(it)
            vm.clearImportError()
        }
    }

    LaunchedEffect(saved) {
        if (saved) {
            vm.clearSaved()   // reset flag before navigating — prevents double-trigger
            onNavigateUp()
        }
    }
    // When a new profile was pre-saved, navigate to resolver editor with the real ID
    LaunchedEffect(resolverNavId) {
        resolverNavId?.let {
            vm.clearResolverNav()
            onEditResolvers(it)
        }
    }

    Scaffold(
        snackbarHost = { SnackbarHost(snackbarHostState) },
        topBar = {
            TopAppBar(
                title = { Text(if (profileId == "new") "New Profile" else "Edit Profile") },
                navigationIcon = {
                    IconButton(onClick = onNavigateUp) {
                        Icon(Icons.AutoMirrored.Filled.ArrowBack, "Back")
                    }
                },
                actions = {
                    // Import .toml
                    IconButton(onClick = { importLauncher.launch("*/*") }) {
                        Icon(Icons.Default.FileOpen, contentDescription = "Import .toml")
                    }
                    // Export .toml
                    IconButton(onClick = {
                        val safeName = profile.name
                            .replace(Regex("[^a-zA-Z0-9_-]"), "_")
                            .ifBlank { "profile" }
                        exportLauncher.launch("$safeName.toml")
                    }) {
                        Icon(Icons.Default.IosShare, contentDescription = "Export .toml")
                    }
                    // Save
                    IconButton(
                        onClick = {
                            if (!saveBusy) {
                                saveBusy = true
                                focusManager.clearFocus(force = true)
                                vm.save()
                            }
                        },
                        enabled = !saveBusy,
                    ) {
                        if (saveBusy) {
                            CircularProgressIndicator(
                                modifier = androidx.compose.ui.Modifier.size(20.dp),
                                strokeWidth = 2.dp,
                            )
                        } else {
                            Icon(Icons.Default.Check, "Save")
                        }
                    }
                },
            )
        }
    ) { padding ->
        Column(
            modifier = Modifier
                .padding(padding)
                .verticalScroll(rememberScrollState())
                .padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            // Meta
            ProfileTextField("Profile Name", profile.name) { vm.update { copy(name = it) } }

            // Tunnel mode selector
            TunnelModeSelector(profile.tunnelMode) { vm.update { copy(tunnelMode = it) } }

            Divider()

            // Section 1: Identity
            ExpandableSection("Tunnel Identity", expanded = true) {
                ProfileTextField("DOMAINS (comma-separated)", profile.domains, hint = "DOMAINS") {
                    vm.update { copy(domains = it) }
                }
                EncryptionMethodSelector(profile.dataEncryptionMethod) {
                    vm.update { copy(dataEncryptionMethod = it) }
                }
                PasswordField("ENCRYPTION_KEY", profile.encryptionKey) {
                    vm.update { copy(encryptionKey = it) }
                }
            }

            // Section 2: Proxy Listener
            ExpandableSection("Proxy / Listener") {
                ProtocolTypeSelector(profile.protocolType) { vm.update { copy(protocolType = it) } }
                ProfileTextField("LISTEN_IP", profile.listenIP, hint = "LISTEN_IP") {
                    vm.update { copy(listenIP = it) }
                }
                ProfileIntField("LISTEN_PORT", profile.listenPort, hint = "LISTEN_PORT") {
                    vm.update { copy(listenPort = it) }
                }
                SwitchRow("SOCKS5_AUTH", profile.socks5Auth) { vm.update { copy(socks5Auth = it) } }
                if (profile.socks5Auth) {
                    ProfileTextField("SOCKS5_USER", profile.socks5User, hint = "SOCKS5_USER") {
                        vm.update { copy(socks5User = it) }
                    }
                    PasswordField("SOCKS5_PASS", profile.socks5Pass, hint = "SOCKS5_PASS") {
                        vm.update { copy(socks5Pass = it) }
                    }
                }
            }

            // Section 3: Local DNS
            ExpandableSection("Local DNS") {
                SwitchRow("LOCAL_DNS_ENABLED", profile.localDnsEnabled) { vm.update { copy(localDnsEnabled = it) } }
                if (profile.localDnsEnabled) {
                    ProfileTextField("LOCAL_DNS_IP", profile.localDnsIP, hint = "LOCAL_DNS_IP") {
                        vm.update { copy(localDnsIP = it) }
                    }
                    ProfileIntField("LOCAL_DNS_PORT", profile.localDnsPort, hint = "LOCAL_DNS_PORT") {
                        vm.update { copy(localDnsPort = it) }
                    }
                    ProfileIntField("LOCAL_DNS_CACHE_MAX_RECORDS", profile.localDnsCacheMaxRecords) {
                        vm.update { copy(localDnsCacheMaxRecords = it) }
                    }
                    ProfileDoubleField("LOCAL_DNS_CACHE_TTL_SECONDS", profile.localDnsCacheTtlSeconds) {
                        vm.update { copy(localDnsCacheTtlSeconds = it) }
                    }
                    ProfileDoubleField("LOCAL_DNS_PENDING_TIMEOUT_SECONDS", profile.localDnsPendingTimeoutSec) {
                        vm.update { copy(localDnsPendingTimeoutSec = it) }
                    }
                    ProfileDoubleField("DNS_RESPONSE_FRAGMENT_TIMEOUT_SECONDS", profile.dnsResponseFragmentTimeoutSeconds) {
                        vm.update { copy(dnsResponseFragmentTimeoutSeconds = it) }
                    }
                    SwitchRow("LOCAL_DNS_CACHE_PERSIST", profile.localDnsCachePersist) {
                        vm.update { copy(localDnsCachePersist = it) }
                    }
                    ProfileDoubleField("LOCAL_DNS_CACHE_FLUSH_INTERVAL_SECONDS", profile.localDnsCacheFlushSec) {
                        vm.update { copy(localDnsCacheFlushSec = it) }
                    }
                }
            }

            // Section 4: Balancing
            ExpandableSection("Balancing & Duplication") {
                BalancingStrategySelector(profile.resolverBalancingStrategy) {
                    vm.update { copy(resolverBalancingStrategy = it) }
                }
                ProfileIntField("PACKET_DUPLICATION_COUNT", profile.packetDuplicationCount) {
                    vm.update { copy(packetDuplicationCount = it) }
                }
                ProfileIntField("SETUP_PACKET_DUPLICATION_COUNT", profile.setupPacketDuplicationCount) {
                    vm.update { copy(setupPacketDuplicationCount = it) }
                }
                ProfileIntField("STREAM_RESOLVER_FAILOVER_RESEND_THRESHOLD", profile.streamResolverFailoverResendThreshold) {
                    vm.update { copy(streamResolverFailoverResendThreshold = it) }
                }
                ProfileDoubleField("STREAM_RESOLVER_FAILOVER_COOLDOWN", profile.streamResolverFailoverCooldownSec) {
                    vm.update { copy(streamResolverFailoverCooldownSec = it) }
                }
            }

            // Section 5: Resolver Health
            ExpandableSection("Resolver Health") {
                SwitchRow("RECHECK_INACTIVE_SERVERS_ENABLED", profile.recheckInactiveServersEnabled) {
                    vm.update { copy(recheckInactiveServersEnabled = it) }
                }
                ProfileDoubleField("RECHECK_INACTIVE_INTERVAL_SECONDS", profile.recheckInactiveIntervalSeconds) {
                    vm.update { copy(recheckInactiveIntervalSeconds = it) }
                }
                ProfileDoubleField("RECHECK_SERVER_INTERVAL_SECONDS", profile.recheckServerIntervalSeconds) {
                    vm.update { copy(recheckServerIntervalSeconds = it) }
                }
                ProfileIntField("RECHECK_BATCH_SIZE", profile.recheckBatchSize) {
                    vm.update { copy(recheckBatchSize = it) }
                }
                SwitchRow("AUTO_DISABLE_TIMEOUT_SERVERS", profile.autoDisableTimeoutServers) {
                    vm.update { copy(autoDisableTimeoutServers = it) }
                }
                ProfileDoubleField("AUTO_DISABLE_TIMEOUT_WINDOW_SECONDS", profile.autoDisableTimeoutWindowSeconds) {
                    vm.update { copy(autoDisableTimeoutWindowSeconds = it) }
                }
                ProfileIntField("AUTO_DISABLE_MIN_OBSERVATIONS", profile.autoDisableMinObservations) {
                    vm.update { copy(autoDisableMinObservations = it) }
                }
                ProfileDoubleField("AUTO_DISABLE_CHECK_INTERVAL_SECONDS", profile.autoDisableCheckIntervalSeconds) {
                    vm.update { copy(autoDisableCheckIntervalSeconds = it) }
                }
            }

            // Section 6: Encoding/Compression
            ExpandableSection("Encoding & Compression") {
                SwitchRow("BASE_ENCODE_DATA", profile.baseEncodeData) {
                    vm.update { copy(baseEncodeData = it) }
                }
                CompressionTypeSelector("UPLOAD_COMPRESSION_TYPE", profile.uploadCompressionType) {
                    vm.update { copy(uploadCompressionType = it) }
                }
                CompressionTypeSelector("DOWNLOAD_COMPRESSION_TYPE", profile.downloadCompressionType) {
                    vm.update { copy(downloadCompressionType = it) }
                }
                ProfileIntField("COMPRESSION_MIN_SIZE", profile.compressionMinSize) {
                    vm.update { copy(compressionMinSize = it) }
                }
            }

            // Section 7: MTU
            ExpandableSection("MTU") {
                ProfileIntField("MIN_UPLOAD_MTU", profile.minUploadMTU) { vm.update { copy(minUploadMTU = it) } }
                ProfileIntField("MAX_UPLOAD_MTU", profile.maxUploadMTU) { vm.update { copy(maxUploadMTU = it) } }
                ProfileIntField("MIN_DOWNLOAD_MTU", profile.minDownloadMTU) { vm.update { copy(minDownloadMTU = it) } }
                ProfileIntField("MAX_DOWNLOAD_MTU", profile.maxDownloadMTU) { vm.update { copy(maxDownloadMTU = it) } }
                ProfileIntField("MTU_TEST_RETRIES", profile.mtuTestRetries) { vm.update { copy(mtuTestRetries = it) } }
                ProfileDoubleField("MTU_TEST_TIMEOUT", profile.mtuTestTimeout) { vm.update { copy(mtuTestTimeout = it) } }
                ProfileIntField("MTU_TEST_PARALLELISM", profile.mtuTestParallelism) { vm.update { copy(mtuTestParallelism = it) } }
            }

            // Section 8: Workers & Timeouts
            ExpandableSection("Workers & Timeouts") {
                ProfileIntField("TUNNEL_READER_WORKERS", profile.tunnelReaderWorkers) { vm.update { copy(tunnelReaderWorkers = it) } }
                ProfileIntField("TUNNEL_WRITER_WORKERS", profile.tunnelWriterWorkers) { vm.update { copy(tunnelWriterWorkers = it) } }
                ProfileIntField("TUNNEL_PROCESS_WORKERS", profile.tunnelProcessWorkers) { vm.update { copy(tunnelProcessWorkers = it) } }
                ProfileDoubleField("TUNNEL_PACKET_TIMEOUT_SECONDS", profile.tunnelPacketTimeoutSec) { vm.update { copy(tunnelPacketTimeoutSec = it) } }
                ProfileDoubleField("DISPATCHER_IDLE_POLL_INTERVAL_SECONDS", profile.dispatcherIdlePollIntervalSeconds) { vm.update { copy(dispatcherIdlePollIntervalSeconds = it) } }
            }

            // Section 9: Ping Manager
            ExpandableSection("Ping Manager") {
                ProfileDoubleField("PING_AGGRESSIVE_INTERVAL_SECONDS", profile.pingAggressiveIntervalSeconds) { vm.update { copy(pingAggressiveIntervalSeconds = it) } }
                ProfileDoubleField("PING_LAZY_INTERVAL_SECONDS", profile.pingLazyIntervalSeconds) { vm.update { copy(pingLazyIntervalSeconds = it) } }
                ProfileDoubleField("PING_COOLDOWN_INTERVAL_SECONDS", profile.pingCooldownIntervalSeconds) { vm.update { copy(pingCooldownIntervalSeconds = it) } }
                ProfileDoubleField("PING_COLD_INTERVAL_SECONDS", profile.pingColdIntervalSeconds) { vm.update { copy(pingColdIntervalSeconds = it) } }
                ProfileDoubleField("PING_WARM_THRESHOLD_SECONDS", profile.pingWarmThresholdSeconds) { vm.update { copy(pingWarmThresholdSeconds = it) } }
                ProfileDoubleField("PING_COOL_THRESHOLD_SECONDS", profile.pingCoolThresholdSeconds) { vm.update { copy(pingCoolThresholdSeconds = it) } }
                ProfileDoubleField("PING_COLD_THRESHOLD_SECONDS", profile.pingColdThresholdSeconds) { vm.update { copy(pingColdThresholdSeconds = it) } }
            }

            // Section 10: ARQ
            ExpandableSection("ARQ") {
                ProfileIntField("MAX_PACKETS_PER_BATCH", profile.maxPacketsPerBatch) { vm.update { copy(maxPacketsPerBatch = it) } }
                ProfileIntField("ARQ_WINDOW_SIZE", profile.arqWindowSize) { vm.update { copy(arqWindowSize = it) } }
                ProfileDoubleField("ARQ_INITIAL_RTO_SECONDS", profile.arqInitialRtoSeconds) { vm.update { copy(arqInitialRtoSeconds = it) } }
                ProfileDoubleField("ARQ_MAX_RTO_SECONDS", profile.arqMaxRtoSeconds) { vm.update { copy(arqMaxRtoSeconds = it) } }
                ProfileDoubleField("ARQ_CONTROL_INITIAL_RTO_SECONDS", profile.arqControlInitialRtoSeconds) { vm.update { copy(arqControlInitialRtoSeconds = it) } }
                ProfileDoubleField("ARQ_CONTROL_MAX_RTO_SECONDS", profile.arqControlMaxRtoSeconds) { vm.update { copy(arqControlMaxRtoSeconds = it) } }
                ProfileIntField("ARQ_MAX_CONTROL_RETRIES", profile.arqMaxControlRetries) { vm.update { copy(arqMaxControlRetries = it) } }
                ProfileIntField("ARQ_MAX_DATA_RETRIES", profile.arqMaxDataRetries) { vm.update { copy(arqMaxDataRetries = it) } }
                ProfileDoubleField("ARQ_INACTIVITY_TIMEOUT_SECONDS", profile.arqInactivityTimeoutSeconds) { vm.update { copy(arqInactivityTimeoutSeconds = it) } }
                ProfileDoubleField("ARQ_DATA_PACKET_TTL_SECONDS", profile.arqDataPacketTtlSeconds) { vm.update { copy(arqDataPacketTtlSeconds = it) } }
                ProfileDoubleField("ARQ_CONTROL_PACKET_TTL_SECONDS", profile.arqControlPacketTtlSeconds) { vm.update { copy(arqControlPacketTtlSeconds = it) } }
                ProfileIntField("ARQ_DATA_NACK_MAX_GAP", profile.arqDataNackMaxGap) { vm.update { copy(arqDataNackMaxGap = it) } }
                ProfileDoubleField("ARQ_DATA_NACK_INITIAL_DELAY_SECONDS", profile.arqDataNackInitialDelaySeconds) { vm.update { copy(arqDataNackInitialDelaySeconds = it) } }
                ProfileDoubleField("ARQ_DATA_NACK_REPEAT_SECONDS", profile.arqDataNackRepeatSeconds) { vm.update { copy(arqDataNackRepeatSeconds = it) } }
                ProfileDoubleField("ARQ_TERMINAL_DRAIN_TIMEOUT_SECONDS", profile.arqTerminalDrainTimeoutSec) { vm.update { copy(arqTerminalDrainTimeoutSec = it) } }
                ProfileDoubleField("ARQ_TERMINAL_ACK_WAIT_TIMEOUT_SECONDS", profile.arqTerminalAckWaitTimeoutSec) { vm.update { copy(arqTerminalAckWaitTimeoutSec = it) } }
            }

            // Section 11: Advanced
            ExpandableSection("Advanced") {
                LogLevelSelector(profile.logLevel) { vm.update { copy(logLevel = it) } }
                ProfileIntField("TX_CHANNEL_SIZE", profile.txChannelSize) { vm.update { copy(txChannelSize = it) } }
                ProfileIntField("RX_CHANNEL_SIZE", profile.rxChannelSize) { vm.update { copy(rxChannelSize = it) } }
                ProfileIntField("RESOLVER_UDP_CONNECTION_POOL_SIZE", profile.resolverUdpConnectionPoolSize) {
                    vm.update { copy(resolverUdpConnectionPoolSize = it) }
                }
                ProfileIntField("STREAM_QUEUE_INITIAL_CAPACITY", profile.streamQueueInitialCapacity) {
                    vm.update { copy(streamQueueInitialCapacity = it) }
                }
                ProfileIntField("ORPHAN_QUEUE_INITIAL_CAPACITY", profile.orphanQueueInitialCapacity) {
                    vm.update { copy(orphanQueueInitialCapacity = it) }
                }
                ProfileIntField("DNS_RESPONSE_FRAGMENT_STORE_CAPACITY", profile.dnsResponseFragmentStoreCap) {
                    vm.update { copy(dnsResponseFragmentStoreCap = it) }
                }
                ProfileDoubleField("SOCKS_UDP_ASSOCIATE_READ_TIMEOUT_SECONDS", profile.socksUdpAssociateReadTimeoutSeconds) {
                    vm.update { copy(socksUdpAssociateReadTimeoutSeconds = it) }
                }
                ProfileDoubleField("CLIENT_TERMINAL_STREAM_RETENTION_SECONDS", profile.clientTerminalStreamRetentionSeconds) {
                    vm.update { copy(clientTerminalStreamRetentionSeconds = it) }
                }
                ProfileDoubleField("CLIENT_CANCELLED_SETUP_RETENTION_SECONDS", profile.clientCancelledSetupRetentionSeconds) {
                    vm.update { copy(clientCancelledSetupRetentionSeconds = it) }
                }
                ProfileDoubleField("SESSION_INIT_RETRY_BASE_SECONDS", profile.sessionInitRetryBaseSeconds) {
                    vm.update { copy(sessionInitRetryBaseSeconds = it) }
                }
                ProfileDoubleField("SESSION_INIT_RETRY_STEP_SECONDS", profile.sessionInitRetryStepSeconds) {
                    vm.update { copy(sessionInitRetryStepSeconds = it) }
                }
                ProfileIntField("SESSION_INIT_RETRY_LINEAR_AFTER", profile.sessionInitRetryLinearAfter) {
                    vm.update { copy(sessionInitRetryLinearAfter = it) }
                }
                ProfileDoubleField("SESSION_INIT_RETRY_MAX_SECONDS", profile.sessionInitRetryMaxSeconds) {
                    vm.update { copy(sessionInitRetryMaxSeconds = it) }
                }
                ProfileDoubleField("SESSION_INIT_BUSY_RETRY_INTERVAL_SECONDS", profile.sessionInitBusyRetryIntervalSeconds) {
                    vm.update { copy(sessionInitBusyRetryIntervalSeconds = it) }
                }
            }

            // Section 12: MTU Files
            ExpandableSection("MTU Result Files") {
                SwitchRow("SAVE_MTU_SERVERS_TO_FILE", profile.saveMtuServersToFile) {
                    vm.update { copy(saveMtuServersToFile = it) }
                }
                if (profile.saveMtuServersToFile) {
                    ProfileTextField("MTU_SERVERS_FILE_NAME", profile.mtuServersFileName) {
                        vm.update { copy(mtuServersFileName = it) }
                    }
                    ProfileTextField("MTU_SERVERS_FILE_FORMAT", profile.mtuServersFileFormat) {
                        vm.update { copy(mtuServersFileFormat = it) }
                    }
                    ProfileTextField("MTU_USING_SECTION_SEPARATOR_TEXT", profile.mtuUsingSeparatorText) {
                        vm.update { copy(mtuUsingSeparatorText = it) }
                    }
                    ProfileTextField("MTU_REMOVED_SERVER_LOG_FORMAT", profile.mtuRemovedServerLogFormat) {
                        vm.update { copy(mtuRemovedServerLogFormat = it) }
                    }
                    ProfileTextField("MTU_ADDED_SERVER_LOG_FORMAT", profile.mtuAddedServerLogFormat) {
                        vm.update { copy(mtuAddedServerLogFormat = it) }
                    }
                }
            }

            // Resolver list
            Divider()
            OutlinedButton(
                onClick = {
                    if (profileId == Screen.ProfileEdit.NEW) {
                        // Profile not yet in DB — save it first, then navigate
                        vm.saveForResolver()
                    } else {
                        onEditResolvers(profile.id)
                    }
                },
                modifier = Modifier.fillMaxWidth(),
            ) {
                Icon(Icons.Default.Edit, contentDescription = null)
                Spacer(Modifier.width(8.dp))
                Text("Edit Resolver List")
            }
        }
    }
}

// ---------------------------------------------------------------------------
// Reusable field composables
// ---------------------------------------------------------------------------

@Composable
fun ProfileTextField(
    label: String,
    value: String,
    hint: String = label,
    onValueChange: (String) -> Unit,
) {
    var local by remember { mutableStateOf(value) }
    var focused by remember { mutableStateOf(false) }
    LaunchedEffect(value) { if (!focused) local = value }
    OutlinedTextField(
        value = local,
        onValueChange = { local = it; onValueChange(it) },
        label = { Text(label) },
        placeholder = { Text(hint, style = MaterialTheme.typography.labelSmall) },
        modifier = Modifier
            .fillMaxWidth()
            .onFocusChanged { state ->
                focused = state.isFocused
                if (!state.isFocused && local != value) onValueChange(local)
            },
        singleLine = true,
    )
}

@Composable
fun PasswordField(label: String, value: String, hint: String = label, onValueChange: (String) -> Unit) {
    var visible by remember { mutableStateOf(false) }
    var local by remember { mutableStateOf(value) }
    var focused by remember { mutableStateOf(false) }
    LaunchedEffect(value) { if (!focused) local = value }
    OutlinedTextField(
        value = local,
        onValueChange = { local = it; onValueChange(it) },
        label = { Text(label) },
        placeholder = { Text(hint, style = MaterialTheme.typography.labelSmall) },
        modifier = Modifier
            .fillMaxWidth()
            .onFocusChanged { state ->
                focused = state.isFocused
                if (!state.isFocused && local != value) onValueChange(local)
            },
        singleLine = true,
        visualTransformation = if (visible)
            androidx.compose.ui.text.input.VisualTransformation.None
        else
            androidx.compose.ui.text.input.PasswordVisualTransformation(),
        trailingIcon = {
            IconButton(onClick = { visible = !visible }) {
                Icon(
                    if (visible) Icons.Filled.Visibility else Icons.Filled.VisibilityOff,
                    contentDescription = "Toggle visibility",
                )
            }
        },
    )
}

@Composable
fun ProfileIntField(label: String, value: Int, hint: String = label, onValueChange: (Int) -> Unit) {
    var text by remember { mutableStateOf(value.toString()) }
    var focused by remember { mutableStateOf(false) }
    // Only sync external value changes when the field is not being edited
    LaunchedEffect(value) { if (!focused) text = value.toString() }
    OutlinedTextField(
        value = text,
        onValueChange = {
            text = it
            it.toIntOrNull()?.let { v -> onValueChange(v) }
        },
        label = { Text(label) },
        placeholder = { Text(hint, style = MaterialTheme.typography.labelSmall) },
        modifier = Modifier
            .fillMaxWidth()
            .onFocusChanged { state ->
                focused = state.isFocused
                if (!state.isFocused) {
                    // When losing focus (e.g. user presses Save), commit the
                    // current text or reset to the last valid ViewModel value.
                    val parsed = text.toIntOrNull()
                    if (parsed != null) onValueChange(parsed)
                    else text = value.toString()
                }
            },
        singleLine = true,
        keyboardOptions = androidx.compose.foundation.text.KeyboardOptions(
            keyboardType = androidx.compose.ui.text.input.KeyboardType.Number,
        ),
    )
}

@Composable
fun ProfileDoubleField(label: String, value: Double, onValueChange: (Double) -> Unit) {
    var text by remember { mutableStateOf(value.toString()) }
    var focused by remember { mutableStateOf(false) }
    // Only sync external value changes when the field is not being edited
    LaunchedEffect(value) { if (!focused) text = value.toString() }
    OutlinedTextField(
        value = text,
        onValueChange = { s ->
            text = s
            s.toDoubleOrNull()?.let { onValueChange(it) }
        },
        label = { Text(label) },
        modifier = Modifier
            .fillMaxWidth()
            .onFocusChanged { state ->
                focused = state.isFocused
                if (!state.isFocused) {
                    val parsed = text.toDoubleOrNull()
                    if (parsed != null) onValueChange(parsed)
                    else text = value.toString()
                }
            },
        singleLine = true,
        keyboardOptions = androidx.compose.foundation.text.KeyboardOptions(
            keyboardType = androidx.compose.ui.text.input.KeyboardType.Decimal,
        ),
    )
}

@Composable
fun SwitchRow(label: String, checked: Boolean, onCheckedChange: (Boolean) -> Unit) {
    Row(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = androidx.compose.ui.Alignment.CenterVertically,
    ) {
        Text(label, style = MaterialTheme.typography.bodyMedium)
        Switch(checked = checked, onCheckedChange = onCheckedChange)
    }
}

@Composable
fun TunnelModeSelector(mode: String, onSelect: (String) -> Unit) {
    val options = listOf("SOCKS5", "TUN")
    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        options.forEach { option ->
            FilterChip(
                selected = mode == option,
                onClick = { onSelect(option) },
                label = { Text(option) },
            )
        }
    }
}

@Composable
fun ProtocolTypeSelector(type: String, onSelect: (String) -> Unit) {
    val options = listOf("SOCKS5", "TCP")
    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        options.forEach { option ->
            FilterChip(
                selected = type == option,
                onClick = { onSelect(option) },
                label = { Text(option) },
            )
        }
    }
}

@Composable
fun BalancingStrategySelector(strategy: Int, onSelect: (Int) -> Unit) {
    val options = listOf(
        0 to "RR Default",
        1 to "Random",
        2 to "Round-Robin",
        3 to "Least-Loss",
        4 to "Lowest-Latency",
    )
    Column {
        Text("RESOLVER_BALANCING_STRATEGY", style = MaterialTheme.typography.labelMedium)
        options.forEach { (value, label) ->
            Row(verticalAlignment = androidx.compose.ui.Alignment.CenterVertically) {
                RadioButton(selected = strategy == value, onClick = { onSelect(value) })
                Text(label, style = MaterialTheme.typography.bodyMedium)
            }
        }
    }
}

@Composable
fun CompressionTypeSelector(label: String, type: Int, onSelect: (Int) -> Unit) {
    val options = listOf(0 to "None", 1 to "Zstd", 2 to "LZ4", 3 to "Zlib")
    Column {
        Text(label, style = MaterialTheme.typography.labelMedium)
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            options.forEach { (value, name) ->
                FilterChip(
                    selected = type == value,
                    onClick = { onSelect(value) },
                    label = { Text(name) },
                )
            }
        }
    }
}

@Composable
fun EncryptionMethodSelector(method: Int, onSelect: (Int) -> Unit) {
    val options = listOf(
        0 to "None",
        1 to "XOR",
        2 to "ChaCha20",
        3 to "AES-128-GCM",
        4 to "AES-192-GCM",
        5 to "AES-256-GCM",
    )
    Column {
        Text("DATA_ENCRYPTION_METHOD", style = MaterialTheme.typography.labelMedium)
        options.forEach { (value, label) ->
            Row(verticalAlignment = androidx.compose.ui.Alignment.CenterVertically) {
                RadioButton(selected = method == value, onClick = { onSelect(value) })
                Text(label, style = MaterialTheme.typography.bodyMedium)
            }
        }
    }
}

@Composable
fun LogLevelSelector(level: String, onSelect: (String) -> Unit) {
    val options = listOf("DEBUG", "INFO", "WARN", "ERROR")
    Column {
        Text("LOG_LEVEL", style = MaterialTheme.typography.labelMedium)
        Row(horizontalArrangement = Arrangement.spacedBy(4.dp)) {
            options.forEach { option ->
                FilterChip(
                    selected = level == option,
                    onClick = { onSelect(option) },
                    label = { Text(option) },
                )
            }
        }
    }
}

@Composable
fun ExpandableSection(
    title: String,
    expanded: Boolean = false,
    content: @Composable ColumnScope.() -> Unit,
) {
    var isExpanded by remember { mutableStateOf(expanded) }
    ElevatedCard(modifier = Modifier.fillMaxWidth()) {
        Column(modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp)) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = androidx.compose.ui.Alignment.CenterVertically,
            ) {
                Text(title, style = MaterialTheme.typography.titleSmall)
                IconButton(onClick = { isExpanded = !isExpanded }) {
                    Icon(
                        if (isExpanded) Icons.Default.KeyboardArrowUp else Icons.Default.KeyboardArrowDown,
                        contentDescription = if (isExpanded) "Collapse" else "Expand",
                    )
                }
            }
            if (isExpanded) {
                Spacer(Modifier.height(8.dp))
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    content()
                }
            }
        }
    }
}