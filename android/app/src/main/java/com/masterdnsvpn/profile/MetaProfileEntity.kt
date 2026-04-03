package com.masterdnsvpn.profile

import androidx.room.Entity
import androidx.room.PrimaryKey
import java.util.UUID

@Entity(tableName = "meta_profiles")
data class MetaProfileEntity(
    @PrimaryKey
    val id: String = UUID.randomUUID().toString(),
    val name: String = "New Meta Profile",
    /** Comma-separated profile UUIDs. */
    val profileIds: String = "",
    /** 0..4 mapped to Go balancer strategies. */
    val balancingStrategy: Int = 0,
    /** "SOCKS5" or "TUN" — how the meta profile runs its sub-profiles. */
    val tunnelMode: String = "SOCKS5",
    /** Port the SOCKS5 balancer/proxy listens on. 0 = auto-assign. */
    val socksPort: Int = 0,
    val enabled: Boolean = true,
    val createdAt: Long = System.currentTimeMillis(),
    val updatedAt: Long = System.currentTimeMillis(),
)