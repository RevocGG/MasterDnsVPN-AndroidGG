package com.masterdnsvpn.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ArrowDownward
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Share
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.hilt.navigation.compose.hiltViewModel
import com.masterdnsvpn.log.LogEntry
import com.masterdnsvpn.log.LogLevel
import com.masterdnsvpn.ui.theme.*
import com.masterdnsvpn.ui.viewmodel.LogViewerViewModel
import java.io.File
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import kotlinx.coroutines.launch

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun LogViewerScreen(vm: LogViewerViewModel = hiltViewModel()) {
    var levelFilter by remember { mutableStateOf<LogLevel?>(null) }
    var sourceFilter by remember { mutableStateOf<LogEntry.Source?>(LogEntry.Source.APP) }
    var autoScroll by remember { mutableStateOf(true) }
    val ctx = LocalContext.current
    val scope = rememberCoroutineScope()

    // Ensure log callback is registered
    LaunchedEffect(Unit) { vm.ensureLogCallbackRegistered() }

    val entries by vm.filteredEntries(levelFilter, sourceFilter).collectAsState(initial = emptyList())
    val listState = rememberLazyListState()

    // Jump instantly to last item on first load (no animation)
    var initialScrollDone by remember { mutableStateOf(false) }
    LaunchedEffect(entries.size) {
        if (entries.isNotEmpty() && !initialScrollDone) {
            listState.scrollToItem(entries.size - 1)
            initialScrollDone = true
        } else if (autoScroll && entries.isNotEmpty() && initialScrollDone) {
            listState.animateScrollToItem(entries.size - 1)
        }
    }

    // Show "scroll to bottom" FAB when user has scrolled away from the last item
    val showScrollToBottom by remember {
        derivedStateOf {
            val lastVisible = listState.layoutInfo.visibleItemsInfo.lastOrNull()?.index ?: 0
            entries.isNotEmpty() && lastVisible < entries.size - 3
        }
    }

    GlassBackground {
        Scaffold(
            containerColor = androidx.compose.ui.graphics.Color.Transparent,
            floatingActionButton = {
                if (showScrollToBottom) {
                    SmallFloatingActionButton(
                        onClick = {
                            scope.launch {
                                if (entries.isNotEmpty())
                                    listState.animateScrollToItem(entries.size - 1)
                            }
                        },
                        containerColor = TealPrimary.copy(alpha = 0.9f),
                        contentColor = Color.White,
                        shape = CircleShape,
                    ) {
                        Icon(Icons.Default.ArrowDownward, contentDescription = "Scroll to bottom")
                    }
                }
            },
            topBar = {
                TopAppBar(
                    title = { Text("Log", color = TealLight) },
                    colors = TopAppBarDefaults.topAppBarColors(
                        containerColor = androidx.compose.ui.graphics.Color.Transparent,
                    ),
                    actions = {
                        FilterChip(
                            selected = autoScroll,
                            onClick = { autoScroll = !autoScroll },
                            label = { Text("Auto", fontSize = 11.sp) },
                            modifier = Modifier.padding(end = 4.dp),
                            colors = FilterChipDefaults.filterChipColors(
                                selectedContainerColor = TealPrimary.copy(alpha = 0.3f),
                                selectedLabelColor = TealLight,
                            ),
                        )
                        IconButton(onClick = {
                            val f = File(ctx.cacheDir, "masterdnsvpn_log_export.txt")
                            vm.export(f)
                            val uri = androidx.core.content.FileProvider.getUriForFile(
                                ctx, "${ctx.packageName}.provider", f,
                            )
                            val shareIntent = android.content.Intent(android.content.Intent.ACTION_SEND).apply {
                                type = "text/plain"
                                putExtra(android.content.Intent.EXTRA_STREAM, uri)
                                addFlags(android.content.Intent.FLAG_GRANT_READ_URI_PERMISSION)
                            }
                            ctx.startActivity(android.content.Intent.createChooser(shareIntent, "Export Log"))
                        }) {
                            Icon(Icons.Default.Share, "Export", tint = CyanAccent)
                        }
                        IconButton(onClick = { vm.clear() }) {
                            Icon(Icons.Default.Delete, "Clear", tint = TextSecondary)
                        }
                    },
                )
            }
        ) { padding ->
            Column(modifier = Modifier.padding(padding)) {

                // ── Source tabs: App / System (Logcat) ───────────────────────
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 8.dp, vertical = 2.dp),
                    horizontalArrangement = Arrangement.spacedBy(6.dp),
                ) {
                    listOf(
                        null to "ALL",
                        LogEntry.Source.APP to "App",
                        LogEntry.Source.LOGCAT to "Android",
                    ).forEach { (src, label) ->
                        FilterChip(
                            selected = sourceFilter == src,
                            onClick = { sourceFilter = src },
                            label = { Text(label, fontSize = 11.sp) },
                            colors = FilterChipDefaults.filterChipColors(
                                selectedContainerColor = TealPrimary.copy(alpha = 0.3f),
                                selectedLabelColor = TealLight,
                            ),
                        )
                    }
                }

                // ── Level filter chips ────────────────────────────────────────
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 8.dp, vertical = 2.dp),
                    horizontalArrangement = Arrangement.spacedBy(4.dp),
                ) {
                    FilterChip(
                        selected = levelFilter == null,
                        onClick = { levelFilter = null },
                        label = { Text("ALL", fontSize = 11.sp) },
                        colors = FilterChipDefaults.filterChipColors(
                            selectedContainerColor = TealPrimary.copy(alpha = 0.3f),
                            selectedLabelColor = TealLight,
                        ),
                    )
                    LogLevel.entries.forEach { level ->
                        val chipColor = when (level) {
                            LogLevel.DEBUG -> PurpleDebug
                            LogLevel.INFO -> GreenOnline
                            LogLevel.WARN -> AmberWarn
                            LogLevel.ERROR -> RedError
                        }
                        FilterChip(
                            selected = levelFilter == level,
                            onClick = { levelFilter = if (levelFilter == level) null else level },
                            label = { Text(level.label, fontSize = 11.sp) },
                            colors = FilterChipDefaults.filterChipColors(
                                selectedContainerColor = chipColor.copy(alpha = 0.2f),
                                selectedLabelColor = chipColor,
                            ),
                        )
                    }
                }

                if (entries.isEmpty()) {
                    Box(
                        modifier = Modifier.fillMaxSize().padding(32.dp),
                        contentAlignment = androidx.compose.ui.Alignment.Center,
                    ) {
                        Text(
                            "No log entries yet.\nStart a tunnel to see logs.",
                            style = MaterialTheme.typography.bodyMedium,
                            color = TextSecondary,
                        )
                    }
                } else {
                    LazyColumn(
                        state = listState,
                        modifier = Modifier.fillMaxSize(),
                        contentPadding = PaddingValues(horizontal = 10.dp, vertical = 4.dp),
                        verticalArrangement = Arrangement.spacedBy(3.dp),
                    ) {
                        items(entries, key = { System.identityHashCode(it) }) { entry ->
                            LogCard(entry)
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun LogCard(entry: LogEntry) {
    val levelColor = when (entry.level) {
        LogLevel.DEBUG -> PurpleDebug
        LogLevel.INFO  -> GreenOnline
        LogLevel.WARN  -> AmberWarn
        LogLevel.ERROR -> RedError
    }
    val levelLabel = entry.level.label
    val localTimeFmt = remember { SimpleDateFormat("HH:mm:ss", Locale.getDefault()) }
    val timeDisplay = localTimeFmt.format(Date(if (entry.epochMs > 0L) entry.epochMs else System.currentTimeMillis()))

    Surface(
        modifier = Modifier.fillMaxWidth(),
        shape = RoundedCornerShape(8.dp),
        color = levelColor.copy(alpha = 0.08f),
        tonalElevation = 0.dp,
    ) {
        Row(modifier = Modifier.fillMaxWidth()) {
            // Left accent bar
            Box(
                modifier = Modifier
                    .width(3.dp)
                    .fillMaxHeight()
                    .clip(RoundedCornerShape(topStart = 8.dp, bottomStart = 8.dp))
                    .background(levelColor),
            )
            Column(modifier = Modifier.padding(start = 8.dp, top = 5.dp, end = 8.dp, bottom = 5.dp)) {
                // Header: time + level badge
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(6.dp),
                ) {
                    Text(
                        text = timeDisplay,
                        fontSize = 9.sp,
                        color = TextSecondary,
                        fontFamily = FontFamily.Monospace,
                        fontWeight = FontWeight.Medium,
                    )
                    Surface(
                        shape = RoundedCornerShape(4.dp),
                        color = levelColor.copy(alpha = 0.20f),
                    ) {
                        Text(
                            text = levelLabel,
                            fontSize = 8.sp,
                            color = levelColor,
                            fontWeight = FontWeight.Bold,
                            modifier = Modifier.padding(horizontal = 5.dp, vertical = 1.dp),
                        )
                    }
                    if (entry.source == LogEntry.Source.LOGCAT) {
                        Surface(
                            shape = RoundedCornerShape(4.dp),
                            color = CyanAccent.copy(alpha = 0.12f),
                        ) {
                            Text(
                                text = "Android",
                                fontSize = 8.sp,
                                color = CyanAccent,
                                fontWeight = FontWeight.Bold,
                                modifier = Modifier.padding(horizontal = 5.dp, vertical = 1.dp),
                            )
                        }
                    }
                }
                Spacer(Modifier.height(2.dp))
                // Message
                Text(
                    text = entry.message,
                    fontSize = 10.sp,
                    color = TextPrimary.copy(alpha = 0.9f),
                    fontFamily = FontFamily.Monospace,
                    lineHeight = 14.sp,
                )
            }
        }
    }
}