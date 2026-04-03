package com.masterdnsvpn.ui

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Home
import androidx.compose.material.icons.filled.List
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material3.Icon
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.NavigationBarItemDefaults
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import androidx.navigation.NavHostController
import androidx.navigation.compose.currentBackStackEntryAsState
import com.masterdnsvpn.ui.theme.*

@Composable
fun BottomNavBar(navController: NavHostController, onNewProfile: () -> Unit) {
    val currentRoute = navController.currentBackStackEntryAsState().value?.destination?.route

    NavigationBar(
        containerColor = DarkSurface.copy(alpha = 0.9f),
        contentColor = TextPrimary,
    ) {
        NavigationBarItem(
            selected = currentRoute == Screen.Home.route,
            onClick = {
                if (currentRoute != Screen.Home.route) {
                    navController.navigate(Screen.Home.route) {
                        popUpTo(Screen.Home.route) { inclusive = false }
                        launchSingleTop = true
                    }
                }
            },
            icon = { Icon(Icons.Default.Home, contentDescription = "Home") },
            label = { Text("Home") },
            colors = NavigationBarItemDefaults.colors(
                selectedIconColor = TealLight,
                selectedTextColor = TealLight,
                unselectedIconColor = TextSecondary,
                unselectedTextColor = TextSecondary,
                indicatorColor = TealPrimary.copy(alpha = 0.15f),
            ),
        )

        NavigationBarItem(
            selected = currentRoute == Screen.LogViewer.route,
            onClick = {
                if (currentRoute != Screen.LogViewer.route) {
                    navController.navigate(Screen.LogViewer.route) { launchSingleTop = true }
                }
            },
            icon = { Icon(Icons.Default.List, contentDescription = "Logs") },
            label = { Text("Logs") },
            colors = NavigationBarItemDefaults.colors(
                selectedIconColor = TealLight,
                selectedTextColor = TealLight,
                unselectedIconColor = TextSecondary,
                unselectedTextColor = TextSecondary,
                indicatorColor = TealPrimary.copy(alpha = 0.15f),
            ),
        )

        // ── Add Profile button (center) ───────────────────────────────────────
        NavigationBarItem(
            selected = false,
            onClick = onNewProfile,
            icon = {
                Surface(
                    shape = CircleShape,
                    color = TealPrimary,
                    modifier = Modifier.size(40.dp),
                ) {
                    Box(contentAlignment = Alignment.Center) {
                        Icon(
                            Icons.Default.Add,
                            contentDescription = "New Profile",
                            tint = Color.White,
                            modifier = Modifier.size(22.dp),
                        )
                    }
                }
            },
            label = { Text("Add") },
            colors = NavigationBarItemDefaults.colors(
                selectedIconColor = TealLight,
                selectedTextColor = TealLight,
                unselectedIconColor = TextSecondary,
                unselectedTextColor = TextSecondary,
                indicatorColor = Color.Transparent,
            ),
        )

        NavigationBarItem(
            selected = currentRoute == Screen.Settings.route,
            onClick = {
                if (currentRoute != Screen.Settings.route) {
                    navController.navigate(Screen.Settings.route) { launchSingleTop = true }
                }
            },
            icon = { Icon(Icons.Default.Settings, contentDescription = "Settings") },
            label = { Text("Settings") },
            colors = NavigationBarItemDefaults.colors(
                selectedIconColor = TealLight,
                selectedTextColor = TealLight,
                unselectedIconColor = TextSecondary,
                unselectedTextColor = TextSecondary,
                indicatorColor = TealPrimary.copy(alpha = 0.15f),
            ),
        )
    }
}