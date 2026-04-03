package com.masterdnsvpn.profile

import androidx.room.Dao
import androidx.room.Query
import androidx.room.Upsert
import kotlinx.coroutines.flow.Flow

@Dao
interface MetaProfileDao {
    @Query("SELECT * FROM meta_profiles ORDER BY updatedAt DESC")
    fun getAllMetaProfiles(): Flow<List<MetaProfileEntity>>

    @Query("SELECT * FROM meta_profiles WHERE id = :id LIMIT 1")
    suspend fun getMetaProfileById(id: String): MetaProfileEntity?

    @Upsert
    suspend fun upsert(metaProfile: MetaProfileEntity)

    @Query("DELETE FROM meta_profiles WHERE id = :id")
    suspend fun deleteById(id: String)
}