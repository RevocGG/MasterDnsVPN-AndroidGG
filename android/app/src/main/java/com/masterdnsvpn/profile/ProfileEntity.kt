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
    val localDnsPendingTimeoutSec: Double = 300.0,
    val dnsResponseFragmentTimeoutSeconds: Double = 60.0,
    val localDnsCachePersist: Boolean = true,
    val localDnsCacheFlushSec: Double = 60.0,

    // Section 4: Balancing & Duplication
    val resolverBalancingStrategy: Int = 0,
    val packetDuplicationCount: Int = 2,
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
    val minUploadMTU: Int = 40,
    val minDownloadMTU: Int = 100,
    val maxUploadMTU: Int = 150,
    val maxDownloadMTU: Int = 500,
    val mtuTestRetries: Int = 2,
    val mtuTestTimeout: Double = 2.0,
    val mtuTestParallelism: Int = 32,

    // Section 8: Workers & Timeouts
    val rxTxWorkers: Int = 4,
    val tunnelProcessWorkers: Int = 6,
    val tunnelPacketTimeoutSec: Double = 10.0,
    val dispatcherIdlePollIntervalSeconds: Double = 0.020,

    // Section 9: Ping
    val pingAggressiveIntervalSeconds: Double = 0.200,
    val pingLazyIntervalSeconds: Double = 0.750,
    val pingCooldownIntervalSeconds: Double = 2.0,
    val pingColdIntervalSeconds: Double = 15.0,
    val pingWarmThresholdSeconds: Double = 5.0,
    val pingCoolThresholdSeconds: Double = 15.0,
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
    val mtuServersFileFormat: String = "{IP} - UP: {UP_MTU} DOWN: {DOWN-MTU}",
    val mtuUsingSeparatorText: String = "",
    val mtuRemovedServerLogFormat: String = "Resolver {IP} removed at {TIME} due to {CAUSE}",
    val mtuAddedServerLogFormat: String = "Resolver {IP} added back at {TIME} (UP {UP_MTU}, DOWN {DOWN_MTU})",

    // Section 12: Logging
    val logLevel: String = "INFO",

    // Section 13: ARQ
    val maxPacketsPerBatch: Int = 5,
    val arqWindowSize: Int = 600,
    val arqInitialRtoSeconds: Double = 1.0,
    val arqMaxRtoSeconds: Double = 5.0,
    val arqControlInitialRtoSeconds: Double = 0.5,
    val arqControlMaxRtoSeconds: Double = 3.0,
    val arqMaxControlRetries: Int = 300,
    val arqInactivityTimeoutSeconds: Double = 1800.0,
    val arqDataPacketTtlSeconds: Double = 2400.0,
    val arqControlPacketTtlSeconds: Double = 1200.0,
    val arqMaxDataRetries: Int = 1200,
    val arqDataNackMaxGap: Int = 16,
    val arqDataNackInitialDelaySeconds: Double = 0.4,
    val arqDataNackRepeatSeconds: Double = 1.0,
    val arqTerminalDrainTimeoutSec: Double = 120.0,
    val arqTerminalAckWaitTimeoutSec: Double = 90.0,

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