package com.masterdnsvpn.profile

import android.content.SharedPreferences
import com.google.gson.Gson
import com.masterdnsvpn.di.EncryptedPrefs
import kotlinx.coroutines.flow.Flow
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Repository that wraps the Room [ProfileDao] and [MetaProfileDao].
 *
 * Sensitive fields ([ProfileEntity.encryptionKey], [ProfileEntity.socks5Pass])
 * are stored in [EncryptedSharedPreferences] keyed by profile UUID.
 * Room entities store only non-sensitive data.
 */
@Singleton
class ProfileRepository @Inject constructor(
    private val profileDao: ProfileDao,
    private val metaDao: MetaProfileDao,
    @EncryptedPrefs private val securePrefs: SharedPreferences,
    private val gson: Gson = Gson(),
) {
    private fun keyForField(profileId: String, field: String) = "profile_${profileId}_$field"

    // ------------------------------------------------------------------
    // Profile CRUD
    // ------------------------------------------------------------------

    fun allProfiles(): Flow<List<ProfileEntity>> = profileDao.getAllProfiles()

    suspend fun getProfile(id: String): ProfileEntity? = profileDao.getProfileById(id)

    suspend fun saveProfile(profile: ProfileEntity): ProfileEntity {
        // Store sensitive fields in Keystore-backed prefs, clear from Room row
        securePrefs.edit()
            .putString(keyForField(profile.id, "encryptionKey"), profile.encryptionKey)
            .putString(keyForField(profile.id, "socks5Pass"), profile.socks5Pass)
            .apply()
        val sanitized = profile.copy(
            encryptionKey = "", // not stored in Room
            socks5Pass = "",    // not stored in Room
        )
        profileDao.upsert(sanitized)
        return profile // return original (with secrets) for immediate use
    }

    suspend fun deleteProfile(id: String) {
        securePrefs.edit()
            .remove(keyForField(id, "encryptionKey"))
            .remove(keyForField(id, "socks5Pass"))
            .apply()
        profileDao.deleteById(id)
    }

    /**
     * Returns the profile with sensitive fields re-joined from secure prefs.
     * Call this when you need to pass the full config to [GoMobileBridge].
     */
    suspend fun getProfileForTunnel(id: String): ProfileEntity? {
        val p = profileDao.getProfileById(id) ?: return null
        val key = securePrefs.getString(keyForField(id, "encryptionKey"), "") ?: ""
        val pass = securePrefs.getString(keyForField(id, "socks5Pass"), "") ?: ""
        return p.copy(encryptionKey = key, socks5Pass = pass)
    }

    // ------------------------------------------------------------------
    // Profile export / import
    // ------------------------------------------------------------------

    /** Serialise a full profile (including secrets) to JSON. */
    suspend fun exportProfileJson(id: String): String? {
        val p = getProfileForTunnel(id) ?: return null
        return gson.toJson(p)
    }

    /** Import a profile from JSON. Generates a new UUID to avoid collisions. */
    suspend fun importProfileJson(json: String): ProfileEntity {
        val p = gson.fromJson(json, ProfileEntity::class.java)
        val newProfile = p.copy(
            id = java.util.UUID.randomUUID().toString(),
            createdAt = System.currentTimeMillis(),
            updatedAt = System.currentTimeMillis(),
        )
        return saveProfile(newProfile)
    }

    // ------------------------------------------------------------------
    // Meta-profile CRUD
    // ------------------------------------------------------------------

    fun allMetaProfiles(): Flow<List<MetaProfileEntity>> = metaDao.getAllMetaProfiles()

    suspend fun getMetaProfile(id: String): MetaProfileEntity? = metaDao.getMetaProfileById(id)

    suspend fun saveMetaProfile(meta: MetaProfileEntity) = metaDao.upsert(meta)

    suspend fun deleteMetaProfile(id: String) = metaDao.deleteById(id)
}