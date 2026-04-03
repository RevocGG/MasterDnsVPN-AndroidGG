package com.masterdnsvpn.service

import android.content.Context
import androidx.datastore.preferences.preferencesDataStore

val Context.tunnelPrefsDataStore by preferencesDataStore(name = "tunnel_prefs")