package com.masterdnsvpn.ui.components

import android.net.Uri
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.expandVertically
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.shrinkVertically
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.*
import androidx.compose.material.icons.outlined.CheckCircle
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.masterdnsvpn.R
import com.masterdnsvpn.profile.BEST_MATCH_LIST_ID
import com.masterdnsvpn.profile.ResolverListEntity
import com.masterdnsvpn.ui.theme.*
import com.masterdnsvpn.ui.viewmodel.ResolverCardViewModel
import com.masterdnsvpn.ui.viewmodel.ResolverCardState

/**
 * Home-screen resolver card, always collapsed by default.
 *
 * Tapping the header smoothly expands a drawer with:
 *  - the named resolver lists (select one or several for the next start)
 *  - the auto-managed "Best Match" list with a live score/RTT view
 *  - per-list actions: edit (rename + resolvers), export via SAF file
 *    browser, delete
 *  - import: create a new list from .txt files or paste manually
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ResolverCard(
    vm: ResolverCardViewModel = hiltViewModel(),
) {
    val state by vm.state.collectAsStateWithLifecycle()
    var expanded by remember { mutableStateOf(false) }

    GlassCard(modifier = Modifier.fillMaxWidth()) {
        Column {
            // ── Header (always visible, collapsed look) ─────────────────────
            // Height matches the status banner card (single 24dp row) so the
            // collapsed card lines up with the card above it.
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .height(24.dp)
                    .clickable { expanded = !expanded },
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Icon(
                    Icons.Default.Dns,
                    contentDescription = null,
                    tint = CyanAccent,
                    modifier = Modifier.size(18.dp),
                )
                Spacer(Modifier.width(8.dp))
                Text(
                    stringResource(R.string.resolver_card_title),
                    color = TextPrimary,
                    fontWeight = FontWeight.SemiBold,
                    fontSize = 14.sp,
                )
                Spacer(Modifier.weight(1f))
                val selCount = state.selectedIds.size
                if (selCount > 0) {
                    Text(
                        "$selCount",
                        color = GreenOnline,
                        fontWeight = FontWeight.Bold,
                        fontSize = 12.sp,
                        modifier = Modifier
                            .clip(RoundedCornerShape(10.dp))
                            .background(GreenOnline.copy(alpha = 0.15f))
                            .padding(horizontal = 8.dp, vertical = 2.dp),
                    )
                    Spacer(Modifier.width(8.dp))
                }
                Icon(
                    if (expanded) Icons.Default.ExpandLess else Icons.Default.ExpandMore,
                    contentDescription = null,
                    tint = TextSecondary,
                    modifier = Modifier.size(20.dp),
                )
            }

            // ── Drawer (smooth expand) ──────────────────────────────────────
            AnimatedVisibility(
                visible = expanded,
                enter = expandVertically(animationSpec = androidx.compose.animation.core.tween(250)) + fadeIn(),
                exit = shrinkVertically(animationSpec = androidx.compose.animation.core.tween(200)) + fadeOut(),
            ) {
                ResolverCardDrawer(state, vm)
            }
        }
    }
}

@Composable
private fun ResolverCardDrawer(state: ResolverCardState, vm: ResolverCardViewModel) {
    var showAddDialog by remember { mutableStateOf(false) }
    var editTarget by remember { mutableStateOf<ResolverListEntity?>(null) }
    var deleteTarget by remember { mutableStateOf<ResolverListEntity?>(null) }

    Column(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 12.dp)
            .padding(bottom = 12.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        HorizontalDivider(color = TextSecondary.copy(alpha = 0.15f), thickness = 0.5.dp)

        // ── List rows ───────────────────────────────────────────────────────
        state.lists.forEach { list ->
            ResolverListRow(
                list = list,
                selected = list.id in state.selectedIds,
                bestMatchScoreRows = if (list.isBestMatch) state.bestMatchRows else emptyList(),
                onSelect = { vm.toggleSelect(list.id) },
                onEdit = { editTarget = list },
                onDelete = { deleteTarget = list },
            )
        }

        // ── Footer actions ──────────────────────────────────────────────────
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            AssistChip(
                onClick = { showAddDialog = true },
                label = { Text(stringResource(R.string.resolver_add), fontSize = 12.sp) },
                leadingIcon = { Icon(Icons.Default.Add, null, Modifier.size(16.dp), tint = CyanAccent) },
            )
            AssistChip(
                onClick = { vm.forceRefreshBestMatch() },
                label = { Text(stringResource(R.string.resolver_refresh_best), fontSize = 12.sp) },
                leadingIcon = { Icon(Icons.Default.Refresh, null, Modifier.size(16.dp), tint = TealLight) },
            )
        }

        if (!state.bestMatchStale && state.bestMatchRows.isEmpty()) {
            Text(
                stringResource(R.string.resolver_best_empty),
                color = TextSecondary,
                fontSize = 11.sp,
            )
        }
    }

    if (showAddDialog) {
        var importedTexts by remember { mutableStateOf<List<String>>(emptyList()) }
        ResolverEditDialog(
            title = stringResource(R.string.resolver_add_title),
            initialName = nextDefaultName(state.lists),
            initialText = "",
            importedFilesCount = importedTexts.size,
            onImportFiles = { texts -> importedTexts = texts },
            onDismiss = { showAddDialog = false },
            onSave = { name, text ->
                val merged = com.masterdnsvpn.settings.ResolverSelectionPrefs.mergeResolverTexts(
                    listOf(text) + importedTexts
                )
                vm.createList(name, merged.ifBlank { text })
                showAddDialog = false
            },
        )
    }

    editTarget?.let { target ->
        var importedTexts by remember { mutableStateOf<List<String>>(emptyList()) }
        ResolverEditDialog(
            title = stringResource(R.string.resolver_edit_title),
            initialName = target.name,
            initialText = target.resolversText,
            importedFilesCount = importedTexts.size,
            onImportFiles = { texts -> importedTexts = texts },
            onDismiss = { editTarget = null },
            onSave = { name, text ->
                val merged = com.masterdnsvpn.settings.ResolverSelectionPrefs.mergeResolverTexts(
                    listOf(target.resolversText, text) + importedTexts
                )
                vm.updateList(target.id, name, merged)
                editTarget = null
            },
        )
    }

    deleteTarget?.let { target ->
        AlertDialog(
            onDismissRequest = { deleteTarget = null },
            title = { Text(stringResource(R.string.resolver_delete_title), color = TextPrimary) },
            text = {
                Text(
                    stringResource(R.string.resolver_delete_msg, target.name),
                    color = TextSecondary,
                )
            },
            confirmButton = {
                TextButton(onClick = {
                    vm.deleteList(target.id)
                    deleteTarget = null
                }) { Text(stringResource(R.string.resolver_delete), color = RedError) }
            },
            dismissButton = {
                TextButton(onClick = { deleteTarget = null }) {
                    Text(stringResource(R.string.resolver_cancel))
                }
            },
            containerColor = DarkSurface,
        )
    }
}

private fun nextDefaultName(lists: List<ResolverListEntity>): String {
    var i = lists.size + 1
    while (lists.any { it.name == "Resolver $i" }) i++
    return "Resolver $i"
}

@Composable
private fun ResolverListRow(
    list: ResolverListEntity,
    selected: Boolean,
    bestMatchScoreRows: List<com.masterdnsvpn.ui.viewmodel.BestMatchRow>,
    onSelect: () -> Unit,
    onEdit: () -> Unit,
    onDelete: () -> Unit,
) {
    val ctx = LocalContext.current
    var showPreview by remember { mutableStateOf(false) }

    val exportLauncher = rememberLauncherForActivityResult(
        ActivityResultContracts.CreateDocument("text/plain")
    ) { uri: Uri? ->
        if (uri != null) {
            try {
                ctx.contentResolver.openOutputStream(uri)?.use { out ->
                    out.write(list.resolversText.toByteArray())
                }
            } catch (_: Exception) { }
        }
    }

    Surface(
        shape = RoundedCornerShape(12.dp),
        color = if (selected) GreenOnline.copy(alpha = 0.10f) else GlassBg,
        tonalElevation = 0.dp,
        modifier = Modifier
            .fillMaxWidth()
            .clickable(onClick = onSelect),
    ) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 12.dp, vertical = 10.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Icon(
                if (selected) Icons.Outlined.CheckCircle else Icons.Default.RadioButtonUnchecked,
                contentDescription = null,
                tint = if (selected) GreenOnline else TextSecondary.copy(alpha = 0.5f),
                modifier = Modifier.size(20.dp),
            )
            Spacer(Modifier.width(10.dp))
            Column(modifier = Modifier.weight(1f)) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text(
                        list.name,
                        color = TextPrimary,
                        fontWeight = FontWeight.SemiBold,
                        fontSize = 13.sp,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.weight(1f, fill = false),
                    )
                    if (list.isBestMatch) {
                        Spacer(Modifier.width(6.dp))
                        Text(
                            "★",
                            color = TealLight,
                            fontSize = 12.sp,
                        )
                    }
                }
                val count = list.resolversText.lines().count { it.isNotBlank() && !it.startsWith("#") }
                Text(
                    if (count > 0) stringResource(R.string.resolver_count, count)
                    else stringResource(R.string.resolver_count_zero),
                    color = TextSecondary,
                    fontSize = 11.sp,
                )
                if (list.isBestMatch && bestMatchScoreRows.isNotEmpty()) {
                    val top = bestMatchScoreRows.take(3)
                    Text(
                        top.joinToString("  ·  ") { "${it.resolver} (${it.score}%)" },
                        color = TealLight.copy(alpha = 0.8f),
                        fontSize = 10.sp,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                }
            }
            // Small export button
            IconButton(onClick = { exportLauncher.launch("${list.name}.txt") }, modifier = Modifier.size(30.dp)) {
                Icon(Icons.Default.FileDownload, stringResource(R.string.resolver_export), tint = CyanAccent, modifier = Modifier.size(16.dp))
            }
            // Edit
            IconButton(onClick = onEdit, modifier = Modifier.size(30.dp)) {
                Icon(Icons.Default.Edit, stringResource(R.string.resolver_edit), tint = TextSecondary, modifier = Modifier.size(15.dp))
            }
            // Delete (never for Best Match)
            if (!list.isBestMatch) {
                IconButton(onClick = onDelete, modifier = Modifier.size(30.dp)) {
                    Icon(Icons.Default.DeleteOutline, stringResource(R.string.resolver_delete), tint = RedError.copy(alpha = 0.7f), modifier = Modifier.size(15.dp))
                }
            }
        }
    }
}

@Composable
private fun ResolverEditDialog(
    title: String,
    initialName: String,
    initialText: String,
    importedFilesCount: Int = 0,
    onImportFiles: ((List<String>) -> Unit)?,
    onDismiss: () -> Unit,
    onSave: (String, String) -> Unit,
) {
    val ctx = LocalContext.current
    // remember { mutableStateOf } would freeze the field at the first
    // composition and silently merge late keystrokes into the NEXT opened
    // dialog (Stateless state outliving the dialog instance). key() forces a
    // fresh state per dialog instance.
    var name by remember(initialName + "\u0001" + initialText) {
        mutableStateOf(initialName)
    }
    var text by remember(initialName + "\u0001" + initialText) {
        mutableStateOf(initialText)
    }

    val importLauncher = rememberLauncherForActivityResult(
        ActivityResultContracts.OpenMultipleDocuments()
    ) { uris: List<Uri> ->
        if (uris.isNotEmpty()) {
            val texts = uris.mapNotNull { uri ->
                try {
                    ctx.contentResolver.openInputStream(uri)?.bufferedReader()?.readText()
                } catch (_: Exception) { null }
            }
            onImportFiles?.invoke(texts)
        }
    }

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(title, color = TextPrimary, fontSize = 16.sp) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                OutlinedTextField(
                    value = name,
                    onValueChange = { name = it },
                    label = { Text(stringResource(R.string.resolver_name_label), color = TextSecondary) },
                    singleLine = true,
                    colors = OutlinedTextFieldDefaults.colors(
                        focusedTextColor = TextPrimary,
                        unfocusedTextColor = TextPrimary,
                    ),
                )
                OutlinedTextField(
                    value = text,
                    onValueChange = { text = it },
                    label = { Text(stringResource(R.string.resolver_text_label), color = TextSecondary) },
                    placeholder = {
                        Text("185.226.117.8\n185.235.196.6:53\n…", color = TextHint.copy(alpha = 0.5f), fontFamily = FontFamily.Monospace, fontSize = 11.sp)
                    },
                    textStyle = MaterialTheme.typography.bodySmall.copy(fontFamily = FontFamily.Monospace, color = TextPrimary),
                    minLines = 4,
                    maxLines = 8,
                    colors = OutlinedTextFieldDefaults.colors(
                        focusedTextColor = TextPrimary,
                        unfocusedTextColor = TextPrimary,
                    ),
                )
                if (onImportFiles != null) {
                    TextButton(onClick = {
                        importLauncher.launch(arrayOf("text/*", "*/*", "application/octet-stream"))
                    }) {
                        Icon(Icons.Default.FileOpen, null, Modifier.size(16.dp), tint = CyanAccent)
                        Spacer(Modifier.width(6.dp))
                        Text(
                            if (importedFilesCount > 0)
                                stringResource(R.string.resolver_imported_count, importedFilesCount)
                            else
                                stringResource(R.string.resolver_import_files),
                            fontSize = 12.sp,
                            color = if (importedFilesCount > 0) GreenOnline else CyanAccent,
                        )
                    }
                }
            }
        },
        confirmButton = {
            TextButton(onClick = { onSave(name, text) }) {
                Text(stringResource(R.string.resolver_save), color = GreenOnline)
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.resolver_cancel)) }
        },
        containerColor = DarkSurface,
    )
}
