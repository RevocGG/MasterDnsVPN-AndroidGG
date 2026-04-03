package com.masterdnsvpn.di

import android.content.Context
import android.content.SharedPreferences
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.android.qualifiers.ApplicationContext
import dagger.hilt.components.SingletonComponent
import javax.inject.Qualifier
import javax.inject.Singleton

@Qualifier
@Retention(AnnotationRetention.BINARY)
annotation class EncryptedPrefs

@Module
@InstallIn(SingletonComponent::class)
object SecurityModule {

    /**
     * Provides an [EncryptedSharedPreferences] instance backed by the Android Keystore.
     *
     * Falls back to regular (unencrypted) SharedPreferences on devices where
     * the Keystore is unavailable or throws (e.g. some Android 10 OEM images
     * with broken StrongBox / TEE implementations). This prevents app crashes
     * at launch while still securing keys on capable hardware.
     */
    @Provides
    @Singleton
    @EncryptedPrefs
    fun provideEncryptedSharedPreferences(
        @ApplicationContext ctx: Context,
    ): SharedPreferences {
        return try {
            val masterKey = MasterKey.Builder(ctx)
                .setKeyScheme(MasterKey.KeyScheme.AES256_GCM)
                .build()
            EncryptedSharedPreferences.create(
                ctx,
                "masterdnsvpn_secure_prefs",
                masterKey,
                EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
                EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM,
            )
        } catch (e: Exception) {
            // Keystore unavailable on this device — fall back to plain prefs.
            // This is acceptable: the app works, though keys are stored
            // without hardware-backed encryption on affected devices.
            ctx.getSharedPreferences("masterdnsvpn_secure_prefs_fallback", Context.MODE_PRIVATE)
        }
    }
}