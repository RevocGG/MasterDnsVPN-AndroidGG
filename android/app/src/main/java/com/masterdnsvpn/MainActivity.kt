package com.masterdnsvpn

import android.content.Context
import android.os.Build
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.core.view.WindowCompat
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Scaffold
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.res.painterResource
import androidx.navigation.NavType
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.rememberNavController
import androidx.navigation.navArgument
import com.masterdnsvpn.ui.BottomNavBar
import com.masterdnsvpn.ui.Screen
import com.masterdnsvpn.ui.screens.*
import com.masterdnsvpn.ui.theme.DarkBg
import com.masterdnsvpn.ui.theme.MasterDnsVpnTheme
import dagger.hilt.android.AndroidEntryPoint
import javax.inject.Inject

@AndroidEntryPoint
class MainActivity : ComponentActivity() {
    @Inject
    lateinit var languagePrefs: com.masterdnsvpn.settings.AppLanguagePrefs

    override fun attachBaseContext(newBase: android.content.Context) {
        // Localize the very first frame: the base context is wrapped with the
        // saved language (default English) BEFORE any resource is resolved.
        // Hilt injection happens after attachBaseContext, so use a lightweight
        // direct read of the same SharedPreferences AppLanguagePrefs owns.
        val tag = newBase.getSharedPreferences("app_settings", Context.MODE_PRIVATE)
            .getString("app_language", "en") ?: "en"
        val locale = java.util.Locale.forLanguageTag(tag)
        java.util.Locale.setDefault(locale)
        val config = android.content.res.Configuration(newBase.resources.configuration)
        config.setLocale(locale)
        config.setLocales(android.os.LocaleList(locale))
        // setLocales does NOT derive layout direction — set it explicitly so
        // Persian (fa) lays out right-to-left across every Compose screen.
        config.setLayoutDirection(locale)
        super.attachBaseContext(newBase.createConfigurationContext(config))
    }

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
                // Full-screen background drawn here (behind system bars) so it
                // extends under the status bar and navigation bar on all screens.
                Box(modifier = Modifier.fillMaxSize()) {
                    Image(
                        painter = painterResource(id = R.drawable.app_background),
                        contentDescription = null,
                        modifier = Modifier.fillMaxSize(),
                        contentScale = ContentScale.Crop,
                    )
                    Box(
                        modifier = Modifier
                            .fillMaxSize()
                            .background(DarkBg.copy(alpha = 0.55f)),
                    )
                    MainNavHost(languagePrefs = languagePrefs)
                }
            }
        }
    }
}

@Composable
private fun MainNavHost(languagePrefs: com.masterdnsvpn.settings.AppLanguagePrefs) {
    val navController = rememberNavController()

    Scaffold(
        containerColor = Color.Transparent,
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
                    languagePrefs = languagePrefs,
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
                )
            }

            // NOTE: the per-profile ResolverEditor route was retired — resolver
            // lists live on the Home-screen Resolvers card now.

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
