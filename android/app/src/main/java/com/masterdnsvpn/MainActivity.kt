package com.masterdnsvpn

import android.os.Build
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.core.view.WindowCompat
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Scaffold
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.navigation.NavType
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.rememberNavController
import androidx.navigation.navArgument
import com.masterdnsvpn.ui.BottomNavBar
import com.masterdnsvpn.ui.Screen
import com.masterdnsvpn.ui.screens.*
import com.masterdnsvpn.ui.theme.MasterDnsVpnTheme
import dagger.hilt.android.AndroidEntryPoint

@AndroidEntryPoint
class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        // Disable window-level inset handling so Compose can manage IME insets
        // independently from the bottom nav bar. Without this, the keyboard pushes
        // the nav bar up on adjustResize instead of only growing scroll content.
        WindowCompat.setDecorFitsSystemWindows(window, false)
        // Request notification permission (required at runtime on Android 13+)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            requestPermissions(arrayOf(android.Manifest.permission.POST_NOTIFICATIONS), 0)
        }
        setContent {
            MasterDnsVpnTheme {
                MainNavHost()
            }
        }
    }
}

@Composable
private fun MainNavHost() {
    val navController = rememberNavController()

    Scaffold(
        bottomBar = {
            BottomNavBar(
                navController = navController,
                onNewProfile = { navController.navigate(Screen.ProfileEdit.withId(Screen.ProfileEdit.NEW)) },
            )
        }
    ) { innerPadding ->
        NavHost(
            navController = navController,
            startDestination = Screen.Home.route,
            modifier = Modifier.padding(innerPadding),
        ) {
            // Home — profile list
            composable(Screen.Home.route) {
                HomeScreen(
                    onEditProfile = { id -> navController.navigate(Screen.ProfileEdit.withId(id)) },
                    onNewProfile = { navController.navigate(Screen.ProfileEdit.withId(Screen.ProfileEdit.NEW)) },
                    onOpenDashboard = { id -> navController.navigate(Screen.Dashboard.withId(id)) },
                    onNewMetaProfile = { navController.navigate(Screen.MetaProfileEdit.withId(Screen.MetaProfileEdit.NEW)) },
                    onEditMetaProfile = { id -> navController.navigate(Screen.MetaProfileEdit.withId(id)) },
                )
            }

            // Log viewer
            composable(Screen.LogViewer.route) {
                LogViewerScreen()
            }

            // Settings — menu
            composable(Screen.Settings.route) {
                SettingsScreen(
                    onNavigateToUpdate = { navController.navigate(Screen.Update.route) },
                    onNavigateToPerAppVpn = { navController.navigate(Screen.PerAppVpn.route) },
                )
            }

            // Per-app VPN selection (global)
            composable(Screen.PerAppVpn.route) {
                PerAppVpnScreen(onNavigateUp = { navController.popBackStack() })
            }

            // Update screen
            composable(Screen.Update.route) {
                UpdateScreen(onNavigateUp = { navController.popBackStack() })
            }

            // Profile create / edit
            composable(
                route = Screen.ProfileEdit.route,
                arguments = listOf(navArgument("profileId") { type = NavType.StringType }),
            ) { backStackEntry ->
                val profileId = backStackEntry.arguments?.getString("profileId") ?: Screen.ProfileEdit.NEW
                ProfileEditScreen(
                    profileId = profileId,
                    onNavigateUp = { navController.popBackStack() },
                    onEditResolvers = { id -> navController.navigate(Screen.ResolverEditor.withId(id)) },
                )
            }

            // Resolver editor
            composable(
                route = Screen.ResolverEditor.route,
                arguments = listOf(navArgument("profileId") { type = NavType.StringType }),
            ) { backStackEntry ->
                val profileId = backStackEntry.arguments?.getString("profileId") ?: ""
                ResolverEditorScreen(
                    profileId = profileId,
                    onNavigateUp = { navController.popBackStack() },
                )
            }

            // Dashboard
            composable(
                route = Screen.Dashboard.route,
                arguments = listOf(navArgument("profileId") { type = NavType.StringType }),
            ) { backStackEntry ->
                val profileId = backStackEntry.arguments?.getString("profileId") ?: ""
                DashboardScreen(
                    profileId = profileId,
                    onNavigateUp = { navController.popBackStack() },
                )
            }

            // Meta-profile editor
            composable(
                route = Screen.MetaProfileEdit.route,
                arguments = listOf(navArgument("metaId") { type = NavType.StringType }),
            ) { backStackEntry ->
                val metaId = backStackEntry.arguments?.getString("metaId") ?: Screen.MetaProfileEdit.NEW
                MetaProfileEditScreen(
                    metaId = metaId,
                    onNavigateUp = { navController.popBackStack() },
                )
            }
        }
    }
}
