package com.masterdnsvpn.ui.screens

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowForwardIos
import androidx.compose.material.icons.filled.KeyboardArrowDown
import androidx.compose.material.icons.filled.Language
import androidx.compose.material.icons.filled.PhoneAndroid
import androidx.compose.material.icons.filled.SystemUpdate
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.masterdnsvpn.R
import com.masterdnsvpn.settings.AppLanguagePrefs
import com.masterdnsvpn.ui.theme.*

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsScreen(
    onNavigateToUpdate: () -> Unit,
    onNavigateToPerAppVpn: () -> Unit = {},
    languagePrefs: AppLanguagePrefs? = null,
) {
    val activityContext = LocalContext.current
    GlassBackground {
        Scaffold(
            containerColor = androidx.compose.ui.graphics.Color.Transparent,
            topBar = {
                TopAppBar(
                    title = { Text(stringResource(R.string.settings_title), fontWeight = FontWeight.Bold, color = TealLight) },
                    colors = TopAppBarDefaults.topAppBarColors(
                        containerColor = androidx.compose.ui.graphics.Color.Transparent,
                    ),
                )
            },
        ) { padding ->
            Column(
                modifier = Modifier
                    .padding(padding)
                    .padding(horizontal = 16.dp),
                verticalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                Spacer(Modifier.height(4.dp))

                SettingsMenuItem(
                    icon = Icons.Default.PhoneAndroid,
                    title = stringResource(R.string.settings_per_app_vpn),
                    subtitle = stringResource(R.string.settings_per_app_vpn_sub),
                    onClick = onNavigateToPerAppVpn,
                )

                // ── Language selector (EN default / fa / zh / ru) ──────────────
                if (languagePrefs != null) {
                    LanguageMenuItem(
                        currentTag = languagePrefs.tag,
                        onSelected = { newTag ->
                            if (languagePrefs.setLanguage(newTag)) {
                                // Smooth handover: fade a solid cover in over the
                                // whole window, THEN recreate so the locale swap
                                // happens behind the cover — no visible jump.
                                (activityContext as? android.app.Activity)?.let { act ->
                                    val decor = act.window.decorView as? android.view.ViewGroup
                                    if (decor != null) {
                                        val cover = android.widget.FrameLayout(act).apply {
                                            setBackgroundColor(android.graphics.Color.argb(255, 0x00, 0x25, 0x35))
                                            alpha = 0f
                                            isClickable = true
                                        }
                                        decor.addView(
                                            cover,
                                            android.view.ViewGroup.LayoutParams(
                                                android.view.ViewGroup.LayoutParams.MATCH_PARENT,
                                                android.view.ViewGroup.LayoutParams.MATCH_PARENT,
                                            ),
                                        )
                                        cover.animate().alpha(1f).setDuration(180)
                                            .withEndAction { act.recreate() }
                                            .start()
                                    } else {
                                        act.recreate()
                                    }
                                }
                            }
                        },
                    )

                    // ── Global "Disable IPv6" toggle (applies to the TUN bridge) ──
                    Surface(
                        shape = RoundedCornerShape(16.dp),
                        color = GlassBg,
                        tonalElevation = 1.dp,
                        modifier = Modifier.fillMaxWidth(),
                    ) {
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .padding(horizontal = 16.dp, vertical = 10.dp),
                            verticalAlignment = Alignment.CenterVertically,
                        ) {
                            Column(modifier = Modifier.weight(1f)) {
                                Text(
                                    stringResource(R.string.settings_disable_ipv6),
                                    color = TextPrimary,
                                    fontWeight = FontWeight.SemiBold,
                                    fontSize = 15.sp,
                                )
                                Text(
                                    stringResource(R.string.settings_disable_ipv6_sub),
                                    color = TextSecondary,
                                    fontSize = 12.sp,
                                )
                            }
                            // Read through a Compose state holder so the Switch
                            // recomposes immediately on toggle (the prefs object is
                            // a plain singleton — writing it alone does NOT invalidate
                            // composition, so the knob looked stuck until the screen
                            // was rebuilt by navigation).
                            var ipv6Off by remember { mutableStateOf(languagePrefs.disableIPv6) }
                            Switch(
                                checked = ipv6Off,
                                onCheckedChange = { checked ->
                                    ipv6Off = checked
                                    languagePrefs.disableIPv6 = checked
                                    languagePrefs.applyDisableIPv6ToBridge()
                                },
                            )
                        }
                    }
                }

                SettingsMenuItem(
                    icon = Icons.Default.SystemUpdate,
                    title = stringResource(R.string.settings_updates),
                    subtitle = stringResource(R.string.settings_updates_sub),
                    onClick = onNavigateToUpdate,
                )
            }
        }
    }
}

/**
 * Language row with a glass-styled dropdown matching the app's card look.
 * The menu opens in-place below the row (same RoundedCornerShape + GlassBg
 * surface as the settings cards) instead of the platform popup style.
 */
@Composable
private fun LanguageMenuItem(
    currentTag: String,
    onSelected: (String) -> Unit,
) {
    var expanded by remember { mutableStateOf(false) }
    val context = LocalContext.current
    val languages = remember { AppLanguagePrefs.Lang.entries }
    val current = languages.firstOrNull { it.tag == currentTag } ?: AppLanguagePrefs.Lang.EN

    Column {
        Surface(
            shape = RoundedCornerShape(16.dp),
            color = GlassBg,
            tonalElevation = 1.dp,
            modifier = Modifier
                .fillMaxWidth()
                .clickable { expanded = !expanded },
        ) {
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 16.dp, vertical = 14.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Icon(
                    imageVector = Icons.Default.Language,
                    contentDescription = null,
                    tint = TealLight,
                    modifier = Modifier.size(28.dp),
                )
                Spacer(Modifier.width(14.dp))
                Column(modifier = Modifier.weight(1f)) {
                    Text(
                        stringResource(R.string.settings_language),
                        color = TextPrimary,
                        fontWeight = FontWeight.SemiBold,
                        fontSize = 15.sp,
                    )
                    Text(
                        stringResource(R.string.settings_language_sub),
                        color = TextSecondary,
                        fontSize = 12.sp,
                    )
                }
                // Current language shown in its own script so it is always readable.
                Text(current.nativeName, color = TealLight, fontWeight = FontWeight.Bold, fontSize = 13.sp)
                Spacer(Modifier.width(6.dp))
                Icon(
                    imageVector = Icons.Default.KeyboardArrowDown,
                    contentDescription = null,
                    tint = TextSecondary.copy(alpha = 0.6f),
                    modifier = Modifier.size(16.dp),
                )
            }
        }

        // Glass panel with the language options, expanding under the card.
        androidx.compose.animation.AnimatedVisibility(visible = expanded) {
            Surface(
                shape = RoundedCornerShape(16.dp),
                color = GlassBg,
                tonalElevation = 2.dp,
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(top = 6.dp),
            ) {
                Column {
                    languages.forEach { lang ->
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .clickable {
                                    expanded = false
                                    onSelected(lang.tag)
                                }
                                .padding(horizontal = 20.dp, vertical = 12.dp),
                            verticalAlignment = Alignment.CenterVertically,
                        ) {
                            Text(
                                lang.nativeName,
                                color = if (lang.tag == currentTag) TealLight else TextPrimary,
                                fontWeight = if (lang.tag == currentTag) FontWeight.Bold else FontWeight.Normal,
                                fontSize = 14.sp,
                                modifier = Modifier.weight(1f),
                            )
                            if (lang.tag == currentTag) {
                                Text("✓", color = TealLight, fontWeight = FontWeight.Bold, fontSize = 14.sp)
                            }
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun SettingsMenuItem(
    icon: ImageVector,
    title: String,
    subtitle: String,
    onClick: () -> Unit,
) {
    Surface(
        shape = RoundedCornerShape(16.dp),
        color = GlassBg,
        tonalElevation = 1.dp,
        modifier = Modifier
            .fillMaxWidth()
            .clickable(onClick = onClick),
    ) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 16.dp, vertical = 14.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Icon(
                imageVector = icon,
                contentDescription = null,
                tint = TealLight,
                modifier = Modifier.size(28.dp),
            )
            Spacer(Modifier.width(14.dp))
            Column(modifier = Modifier.weight(1f)) {
                Text(title, color = TextPrimary, fontWeight = FontWeight.SemiBold, fontSize = 15.sp)
                Text(subtitle, color = TextSecondary, fontSize = 12.sp)
            }
            Icon(
                Icons.AutoMirrored.Filled.ArrowForwardIos,
                contentDescription = null,
                tint = TextSecondary.copy(alpha = 0.6f),
                modifier = Modifier.size(14.dp),
            )
        }
    }
}
