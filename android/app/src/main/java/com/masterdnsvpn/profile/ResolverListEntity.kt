package com.masterdnsvpn.profile

import androidx.room.Dao
import androidx.room.Entity
import androidx.room.PrimaryKey
import androidx.room.Query
import androidx.room.Upsert
import kotlinx.coroutines.flow.Flow

/**
 * A user-managed named resolver list ("Resolver One", "My List", "Best Match"…).
 *
 * The [resolversText] holds one resolver per line (same formats as the core
 * client's client_resolvers.txt: IP, IP:port, CIDR, [IPv6]…).
 * [isBestMatch] marks the single auto-updated list generated from live
 * quality statistics while profiles are running.
 */
@Entity(tableName = "resolver_lists")
data class ResolverListEntity(
    @PrimaryKey val id: String,
    val name: String,
    val resolversText: String,
    val isBestMatch: Boolean = false,
    val createdAt: Long,
    val updatedAt: Long,
)

@Dao
interface ResolverListDao {
    @Query("SELECT * FROM resolver_lists ORDER BY isBestMatch DESC, createdAt ASC")
    fun getAll(): Flow<List<ResolverListEntity>>

    @Query("SELECT * FROM resolver_lists WHERE id = :id LIMIT 1")
    suspend fun getById(id: String): ResolverListEntity?

    @Query("SELECT * FROM resolver_lists WHERE isBestMatch = 1 LIMIT 1")
    suspend fun getBestMatch(): ResolverListEntity?

    @Query("SELECT COUNT(*) FROM resolver_lists WHERE isBestMatch = 1")
    suspend fun countBestMatch(): Int

    @Upsert
    suspend fun upsert(list: ResolverListEntity)

    @Query("DELETE FROM resolver_lists WHERE id = :id")
    suspend fun deleteById(id: String)
}
