package com.masterdnsvpn.profile

import androidx.room.Entity
import androidx.room.PrimaryKey
import java.util.UUID

/**
 * Room entity that stores one VPN profile.
 *
 * Every field mirrors a [MobileClientConfig] field from internal/mobile/config_bridge.go.
 * Default values match NewDefaultMobileClientConfig() in the Go layer.
 *
 * SECURITY: [encryptionKey], [socks5User], [socks5Pass] are stored with
 * EncryptedSharedPreferences / EncryptedFile at the repository layer.
 * Room itself stores only the profile UUID reference for those fields;
 * the actual secret values are kept in Android Keystore-backed storage.
 */
@Entity(tableName = "profiles")
data class ProfileEntity(
    @PrimaryKey
    val id: String = UUID.randomUUID().toString(),

    // Meta
    val name: String = "New Profile",
    val isMetaProfile: Boolean = false,
    val createdAt: Long = System.currentTimeMillis(),
    val updatedAt: Long = System.currentTimeMillis(),

    // Connection mode
    val tunnelMode: String = "SOCKS5", // "SOCKS5" or "TUN"

    // "Disable IPv6" toggle (user request). The DNS tunnel has no IPv6 egress:
    // with AAAA answers present, dual-stack apps (YouTube/Cronet etc.) first try
    // IPv6 and get "SOCKS5 connect refused code 3" timeouts. ON (default) strips
    // AAAA records and RSTs IPv6 CONNECTs in the TUN bridge so apps fall back to
    // IPv4 instantly. Only disable it when the server actually has IPv6 egress.
    val disableIPv6: Boolean = true,

    // Section 1: Identity
    val domains: String = "",
    val dataEncryptionMethod: Int = 1,
    val encryptionKey: String = "", // encrypted at rest; this is the keystore alias

    // Section 2: Proxy Listener
    val protocolType: String = "SOCKS5",
    val listenIP: String = "127.0.0.1",
    val listenPort: Int = 18000,
    val socks5Auth: Boolean = false,
    val socks5User: String = "master_dns_vpn",
    val socks5Pass: String = "master_dns_vpn", // encrypted at rest

    // Section 3: Local DNS
    val localDnsEnabled: Boolean = false,
    val localDnsIP: String = "127.0.0.1",
    val localDnsPort: Int = 5353,
    val localDnsCacheMaxRecords: Int = 10000,
    val localDnsCacheTtlSeconds: Double = 14400.0,
    // A8 fix: 300 s locked a failed domain's cache entry for 5 minutes even
    // after the tunnel recovered. 20 s matches typical browser retry loops.
    val localDnsPendingTimeoutSec: Double = 20.0,
    val dnsResponseFragmentTimeoutSeconds: Double = 60.0,
    val localDnsCachePersist: Boolean = true,
    val localDnsCacheFlushSec: Double = 60.0,

    // Section 4: Balancing & Duplication
    val resolverBalancingStrategy: Int = 3,
    // A6 fix: default duplication 2 doubled upload volume and server load on
    // healthy tunnels. Recheck/auto-disable health logic handles real loss.
    val packetDuplicationCount: Int = 1,
    val setupPacketDuplicationCount: Int = 2,
    val streamResolverFailoverResendThreshold: Int = 2,
    val streamResolverFailoverCooldownSec: Double = 2.5,

    // Section 5: Resolver Health
    val recheckInactiveServersEnabled: Boolean = true,
    val recheckInactiveIntervalSeconds: Double = 60.0,
    val recheckServerIntervalSeconds: Double = 3.0,
    val recheckBatchSize: Int = 10,
    val autoDisableTimeoutServers: Boolean = true,
    val autoDisableTimeoutWindowSeconds: Double = 30.0,
    val autoDisableMinObservations: Int = 5,
    val autoDisableCheckIntervalSeconds: Double = 2.0,

    // Section 6: Encoding/Compression
    val baseEncodeData: Boolean = false,
    val uploadCompressionType: Int = 0,
    val downloadCompressionType: Int = 0,
    val compressionMinSize: Int = 120,

    // Section 7: MTU
    val minUploadMTU: Int = 38,
    val minDownloadMTU: Int = 100,
    val maxUploadMTU: Int = 150,
    val maxDownloadMTU: Int = 500,
    val autoRemoveLowMtuServers: Boolean = true,
    val mtuTestRetries: Int = 2,
    val mtuTestTimeout: Double = 2.0,
    val mtuTestParallelism: Int = 16,

    // Section 8: Workers & Timeouts
    val rxTxWorkers: Int = 4,
    val tunnelProcessWorkers: Int = 6,
    val tunnelPacketTimeoutSec: Double = 10.0,
    // A7 fix: 20 ms polling = 50 CPU wakeups/s per profile for nothing.
    val dispatcherIdlePollIntervalSeconds: Double = 0.050,

    // Section 9: Ping
    // A7 fix: 10 pings/s per resolver was hostile to battery and the server.
    val pingAggressiveIntervalSeconds: Double = 0.250,
    val pingLazyIntervalSeconds: Double = 0.750,
    val pingCooldownIntervalSeconds: Double = 2.0,
    val pingColdIntervalSeconds: Double = 15.0,
    val pingWarmThresholdSeconds: Double = 8.0,
    val pingCoolThresholdSeconds: Double = 20.0,
    val pingColdThresholdSeconds: Double = 30.0,

    // Section 10: Advanced
    val txChannelSize: Int = 8192,
    val rxChannelSize: Int = 4096,
    val resolverUdpConnectionPoolSize: Int = 128,
    val streamQueueInitialCapacity: Int = 256,
    val orphanQueueInitialCapacity: Int = 64,
    val dnsResponseFragmentStoreCap: Int = 512,
    val socksUdpAssociateReadTimeoutSeconds: Double = 30.0,
    val clientTerminalStreamRetentionSeconds: Double = 45.0,
    val clientCancelledSetupRetentionSeconds: Double = 120.0,
    val sessionInitRetryBaseSeconds: Double = 1.0,
    val sessionInitRetryStepSeconds: Double = 1.0,
    val sessionInitRetryLinearAfter: Int = 5,
    val sessionInitRetryMaxSeconds: Double = 60.0,
    val sessionInitBusyRetryIntervalSeconds: Double = 60.0,
    val sessionInitRacingCount: Int = 3,

    // Section 11: MTU files
    val saveMtuServersToFile: Boolean = false,
    val mtuServersFileDir: String = "",  // empty = use internal profile dir; set to absolute path for accessible output
    val mtuServersFileName: String = "masterdnsvpn_success_test_{time}.log",
    val mtuServersFileFormat: String = "{IP} ({DOMAIN}) - UP: {UP_MTU} DOWN: {DOWN-MTU}",
    val mtuUsingSeparatorText: String = "",
    val mtuRemovedServerLogFormat: String = "Resolver {IP} ({DOMAIN}) removed at {TIME} due to {CAUSE}",
    val mtuAddedServerLogFormat: String = "Resolver {IP} ({DOMAIN}) added back at {TIME} (UP {UP_MTU}, DOWN {DOWN_MTU})",
    val mtuReactiveAddedServerLogFormat: String = "Resolver {IP} ({DOMAIN}) added back at {TIME} after reactive recheck (UP {UP_MTU}, DOWN {DOWN_MTU})",

    // Section 12: Logging
    val logLevel: String = "INFO",

    // Section 13: ARQ
    val maxPacketsPerBatch: Int = 8,
    val arqWindowSize: Int = 600,
    val arqInitialRtoSeconds: Double = 1.0,
    val arqMaxRtoSeconds: Double = 5.0,
    val arqControlInitialRtoSeconds: Double = 0.5,
    val arqControlMaxRtoSeconds: Double = 3.0,
    val arqMaxControlRetries: Int = 400,
    // A2 fix: old defaults (1800 s inactivity, 2400 s TTL, 1200 retries) kept a
    // dead stream retransmitting to the server for 40–100 minutes. 300 s / 180 s
    // / 80 retries (core floors: 120 s, 120 s, 60) bound a stalled stream to
    // ~3 minutes while leaving healthy slow uploads untouched.
    val arqInactivityTimeoutSeconds: Double = 300.0,
    val arqDataPacketTtlSeconds: Double = 180.0,
    val arqControlPacketTtlSeconds: Double = 1200.0,
    val arqMaxDataRetries: Int = 80,
    val arqDataNackMaxGap: Int = 16,
    val arqDataNackInitialDelaySeconds: Double = 0.1,
    val arqDataNackRepeatSeconds: Double = 1.0,
    val arqTerminalDrainTimeoutSec: Double = 30.0,
    val arqTerminalAckWaitTimeoutSec: Double = 30.0,

    // Resolver list (stored inline for simplicity; moved to separate table for large lists)
    val resolversText: String = "",

    // When true, domains and encryptionKey are hidden in the UI ("Locked")
    // Set during import when the exporter chose to hide identity fields.
    val identityLocked: Boolean = false,

    // Per-App VPN Filter (TUN mode only)
    // mode: "ALL" = no filter, "INCLUDE" = only listed apps use VPN, "EXCLUDE" = all except listed
    val perAppVpnMode: String = "ALL",
    // Comma-separated package names for the INCLUDE/EXCLUDE list
    val perAppVpnPackages: String = "",
)