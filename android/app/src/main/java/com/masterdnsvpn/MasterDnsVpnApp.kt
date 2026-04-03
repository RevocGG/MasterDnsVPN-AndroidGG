package com.masterdnsvpn

import android.app.Application
import android.util.Log
import dagger.hilt.android.HiltAndroidApp

@HiltAndroidApp
class MasterDnsVpnApp : Application() {
    override fun onCreate() {
        // Install a global crash logger BEFORE super.onCreate() (which triggers Hilt).
        // This writes the full stack trace to logcat under tag "MasterDnsVPN_CRASH"
        // so it is visible even without a debugger attached.
        val defaultHandler = Thread.getDefaultUncaughtExceptionHandler()
        Thread.setDefaultUncaughtExceptionHandler { thread, throwable ->
            Log.e("MasterDnsVPN_CRASH", "Uncaught exception on thread ${thread.name}", throwable)
            defaultHandler?.uncaughtException(thread, throwable)
        }
        super.onCreate()
    }
}