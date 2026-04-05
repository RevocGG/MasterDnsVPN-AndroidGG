package com.masterdnsvpn.ui

sealed class Screen(val route: String) {
    data object Home : Screen("home")
    data object LogViewer : Screen("log")

    data object ProfileEdit : Screen("profile_edit/{profileId}") {
        const val NEW = "new"
        fun withId(id: String): String = "profile_edit/$id"
    }

    data object ResolverEditor : Screen("resolver_edit/{profileId}") {
        fun withId(id: String): String = "resolver_edit/$id"
    }

    data object Dashboard : Screen("dashboard/{profileId}") {
        fun withId(id: String): String = "dashboard/$id"
    }

    data object MetaProfileEdit : Screen("meta_edit/{metaId}") {
        const val NEW = "new"
        fun withId(id: String): String = "meta_edit/$id"
    }

    data object Settings : Screen("settings")
    data object PerAppVpn : Screen("per_app_vpn")
    data object Update : Screen("update")
}