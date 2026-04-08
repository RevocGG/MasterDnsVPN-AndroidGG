package com.masterdnsvpn.profile

import androidx.room.Database
import androidx.room.RoomDatabase
import androidx.room.migration.Migration
import androidx.sqlite.db.SupportSQLiteDatabase

val MIGRATION_1_2 = object : Migration(1, 2) {
    override fun migrate(db: SupportSQLiteDatabase) {
        db.execSQL("ALTER TABLE meta_profiles ADD COLUMN tunnelMode TEXT NOT NULL DEFAULT 'SOCKS5'")
    }
}

val MIGRATION_2_3 = object : Migration(2, 3) {
    override fun migrate(db: SupportSQLiteDatabase) {
        db.execSQL("ALTER TABLE meta_profiles ADD COLUMN socksPort INTEGER NOT NULL DEFAULT 0")
    }
}

val MIGRATION_3_4 = object : Migration(3, 4) {
    override fun migrate(db: SupportSQLiteDatabase) {
        db.execSQL("ALTER TABLE profiles ADD COLUMN arqDataNackInitialDelaySeconds REAL NOT NULL DEFAULT 0.4")
    }
}

val MIGRATION_4_5 = object : Migration(4, 5) {
    override fun migrate(db: SupportSQLiteDatabase) {
        db.execSQL("ALTER TABLE profiles ADD COLUMN identityLocked INTEGER NOT NULL DEFAULT 0")
    }
}

val MIGRATION_5_6 = object : Migration(5, 6) {
    override fun migrate(db: SupportSQLiteDatabase) {
        db.execSQL("ALTER TABLE profiles ADD COLUMN mtuServersFileDir TEXT NOT NULL DEFAULT ''")
    }
}

val MIGRATION_6_7 = object : Migration(6, 7) {
    override fun migrate(db: SupportSQLiteDatabase) {
        db.execSQL("ALTER TABLE profiles ADD COLUMN rxTxWorkers INTEGER NOT NULL DEFAULT 6")
    }
}

// Migration 7→8: Remove deprecated tunnelReaderWorkers/tunnelWriterWorkers columns
// that were replaced by rxTxWorkers. SQLite does not support DROP COLUMN on older
// Android versions, so we recreate the table with the correct schema.
val MIGRATION_7_8 = object : Migration(7, 8) {
    override fun migrate(db: SupportSQLiteDatabase) {
        db.execSQL("""
            CREATE TABLE IF NOT EXISTS `profiles_new` (
                `id` TEXT NOT NULL,
                `name` TEXT NOT NULL,
                `isMetaProfile` INTEGER NOT NULL,
                `createdAt` INTEGER NOT NULL,
                `updatedAt` INTEGER NOT NULL,
                `tunnelMode` TEXT NOT NULL,
                `domains` TEXT NOT NULL,
                `dataEncryptionMethod` INTEGER NOT NULL,
                `encryptionKey` TEXT NOT NULL,
                `protocolType` TEXT NOT NULL,
                `listenIP` TEXT NOT NULL,
                `listenPort` INTEGER NOT NULL,
                `socks5Auth` INTEGER NOT NULL,
                `socks5User` TEXT NOT NULL,
                `socks5Pass` TEXT NOT NULL,
                `localDnsEnabled` INTEGER NOT NULL,
                `localDnsIP` TEXT NOT NULL,
                `localDnsPort` INTEGER NOT NULL,
                `localDnsCacheMaxRecords` INTEGER NOT NULL,
                `localDnsCacheTtlSeconds` REAL NOT NULL,
                `localDnsPendingTimeoutSec` REAL NOT NULL,
                `dnsResponseFragmentTimeoutSeconds` REAL NOT NULL,
                `localDnsCachePersist` INTEGER NOT NULL,
                `localDnsCacheFlushSec` REAL NOT NULL,
                `resolverBalancingStrategy` INTEGER NOT NULL,
                `packetDuplicationCount` INTEGER NOT NULL,
                `setupPacketDuplicationCount` INTEGER NOT NULL,
                `streamResolverFailoverResendThreshold` INTEGER NOT NULL,
                `streamResolverFailoverCooldownSec` REAL NOT NULL,
                `recheckInactiveServersEnabled` INTEGER NOT NULL,
                `recheckInactiveIntervalSeconds` REAL NOT NULL,
                `recheckServerIntervalSeconds` REAL NOT NULL,
                `recheckBatchSize` INTEGER NOT NULL,
                `autoDisableTimeoutServers` INTEGER NOT NULL,
                `autoDisableTimeoutWindowSeconds` REAL NOT NULL,
                `autoDisableMinObservations` INTEGER NOT NULL,
                `autoDisableCheckIntervalSeconds` REAL NOT NULL,
                `baseEncodeData` INTEGER NOT NULL,
                `uploadCompressionType` INTEGER NOT NULL,
                `downloadCompressionType` INTEGER NOT NULL,
                `compressionMinSize` INTEGER NOT NULL,
                `minUploadMTU` INTEGER NOT NULL,
                `minDownloadMTU` INTEGER NOT NULL,
                `maxUploadMTU` INTEGER NOT NULL,
                `maxDownloadMTU` INTEGER NOT NULL,
                `mtuTestRetries` INTEGER NOT NULL,
                `mtuTestTimeout` REAL NOT NULL,
                `mtuTestParallelism` INTEGER NOT NULL,
                `rxTxWorkers` INTEGER NOT NULL,
                `tunnelProcessWorkers` INTEGER NOT NULL,
                `tunnelPacketTimeoutSec` REAL NOT NULL,
                `dispatcherIdlePollIntervalSeconds` REAL NOT NULL,
                `pingAggressiveIntervalSeconds` REAL NOT NULL,
                `pingLazyIntervalSeconds` REAL NOT NULL,
                `pingCooldownIntervalSeconds` REAL NOT NULL,
                `pingColdIntervalSeconds` REAL NOT NULL,
                `pingWarmThresholdSeconds` REAL NOT NULL,
                `pingCoolThresholdSeconds` REAL NOT NULL,
                `pingColdThresholdSeconds` REAL NOT NULL,
                `txChannelSize` INTEGER NOT NULL,
                `rxChannelSize` INTEGER NOT NULL,
                `resolverUdpConnectionPoolSize` INTEGER NOT NULL,
                `streamQueueInitialCapacity` INTEGER NOT NULL,
                `orphanQueueInitialCapacity` INTEGER NOT NULL,
                `dnsResponseFragmentStoreCap` INTEGER NOT NULL,
                `socksUdpAssociateReadTimeoutSeconds` REAL NOT NULL,
                `clientTerminalStreamRetentionSeconds` REAL NOT NULL,
                `clientCancelledSetupRetentionSeconds` REAL NOT NULL,
                `sessionInitRetryBaseSeconds` REAL NOT NULL,
                `sessionInitRetryStepSeconds` REAL NOT NULL,
                `sessionInitRetryLinearAfter` INTEGER NOT NULL,
                `sessionInitRetryMaxSeconds` REAL NOT NULL,
                `sessionInitBusyRetryIntervalSeconds` REAL NOT NULL,
                `saveMtuServersToFile` INTEGER NOT NULL,
                `mtuServersFileDir` TEXT NOT NULL,
                `mtuServersFileName` TEXT NOT NULL,
                `mtuServersFileFormat` TEXT NOT NULL,
                `mtuUsingSeparatorText` TEXT NOT NULL,
                `mtuRemovedServerLogFormat` TEXT NOT NULL,
                `mtuAddedServerLogFormat` TEXT NOT NULL,
                `logLevel` TEXT NOT NULL,
                `maxPacketsPerBatch` INTEGER NOT NULL,
                `arqWindowSize` INTEGER NOT NULL,
                `arqInitialRtoSeconds` REAL NOT NULL,
                `arqMaxRtoSeconds` REAL NOT NULL,
                `arqControlInitialRtoSeconds` REAL NOT NULL,
                `arqControlMaxRtoSeconds` REAL NOT NULL,
                `arqMaxControlRetries` INTEGER NOT NULL,
                `arqInactivityTimeoutSeconds` REAL NOT NULL,
                `arqDataPacketTtlSeconds` REAL NOT NULL,
                `arqControlPacketTtlSeconds` REAL NOT NULL,
                `arqMaxDataRetries` INTEGER NOT NULL,
                `arqDataNackMaxGap` INTEGER NOT NULL,
                `arqDataNackInitialDelaySeconds` REAL NOT NULL,
                `arqDataNackRepeatSeconds` REAL NOT NULL,
                `arqTerminalDrainTimeoutSec` REAL NOT NULL,
                `arqTerminalAckWaitTimeoutSec` REAL NOT NULL,
                `resolversText` TEXT NOT NULL,
                `identityLocked` INTEGER NOT NULL,
                PRIMARY KEY(`id`)
            )
        """.trimIndent())
        db.execSQL("""
            INSERT INTO `profiles_new`
            SELECT
                `id`, `name`, `isMetaProfile`, `createdAt`, `updatedAt`, `tunnelMode`,
                `domains`, `dataEncryptionMethod`, `encryptionKey`, `protocolType`,
                `listenIP`, `listenPort`, `socks5Auth`, `socks5User`, `socks5Pass`,
                `localDnsEnabled`, `localDnsIP`, `localDnsPort`, `localDnsCacheMaxRecords`,
                `localDnsCacheTtlSeconds`, `localDnsPendingTimeoutSec`, `dnsResponseFragmentTimeoutSeconds`,
                `localDnsCachePersist`, `localDnsCacheFlushSec`, `resolverBalancingStrategy`,
                `packetDuplicationCount`, `setupPacketDuplicationCount`, `streamResolverFailoverResendThreshold`,
                `streamResolverFailoverCooldownSec`, `recheckInactiveServersEnabled`, `recheckInactiveIntervalSeconds`,
                `recheckServerIntervalSeconds`, `recheckBatchSize`, `autoDisableTimeoutServers`,
                `autoDisableTimeoutWindowSeconds`, `autoDisableMinObservations`, `autoDisableCheckIntervalSeconds`,
                `baseEncodeData`, `uploadCompressionType`, `downloadCompressionType`, `compressionMinSize`,
                `minUploadMTU`, `minDownloadMTU`, `maxUploadMTU`, `maxDownloadMTU`,
                `mtuTestRetries`, `mtuTestTimeout`, `mtuTestParallelism`, `rxTxWorkers`,
                `tunnelProcessWorkers`, `tunnelPacketTimeoutSec`, `dispatcherIdlePollIntervalSeconds`,
                `pingAggressiveIntervalSeconds`, `pingLazyIntervalSeconds`, `pingCooldownIntervalSeconds`,
                `pingColdIntervalSeconds`, `pingWarmThresholdSeconds`, `pingCoolThresholdSeconds`, `pingColdThresholdSeconds`,
                `txChannelSize`, `rxChannelSize`, `resolverUdpConnectionPoolSize`, `streamQueueInitialCapacity`,
                `orphanQueueInitialCapacity`, `dnsResponseFragmentStoreCap`, `socksUdpAssociateReadTimeoutSeconds`,
                `clientTerminalStreamRetentionSeconds`, `clientCancelledSetupRetentionSeconds`,
                `sessionInitRetryBaseSeconds`, `sessionInitRetryStepSeconds`, `sessionInitRetryLinearAfter`,
                `sessionInitRetryMaxSeconds`, `sessionInitBusyRetryIntervalSeconds`,
                `saveMtuServersToFile`, `mtuServersFileDir`, `mtuServersFileName`, `mtuServersFileFormat`,
                `mtuUsingSeparatorText`, `mtuRemovedServerLogFormat`, `mtuAddedServerLogFormat`, `logLevel`,
                `maxPacketsPerBatch`, `arqWindowSize`, `arqInitialRtoSeconds`, `arqMaxRtoSeconds`,
                `arqControlInitialRtoSeconds`, `arqControlMaxRtoSeconds`, `arqMaxControlRetries`,
                `arqInactivityTimeoutSeconds`, `arqDataPacketTtlSeconds`, `arqControlPacketTtlSeconds`,
                `arqMaxDataRetries`, `arqDataNackMaxGap`, `arqDataNackInitialDelaySeconds`,
                `arqDataNackRepeatSeconds`, `arqTerminalDrainTimeoutSec`, `arqTerminalAckWaitTimeoutSec`,
                `resolversText`, `identityLocked`
            FROM `profiles`
        """.trimIndent())
        db.execSQL("DROP TABLE `profiles`")
        db.execSQL("ALTER TABLE `profiles_new` RENAME TO `profiles`")
    }
}

val MIGRATION_8_9 = object : Migration(8, 9) {
    override fun migrate(db: SupportSQLiteDatabase) {
        db.execSQL("UPDATE `profiles` SET `localDnsPort` = 5353 WHERE `localDnsPort` = 53")
    }
}
val MIGRATION_10_11 = object : Migration(10, 11) {
    override fun migrate(db: SupportSQLiteDatabase) {
        db.execSQL("ALTER TABLE profiles ADD COLUMN sessionInitRacingCount INTEGER NOT NULL DEFAULT 3")
    }
}
val MIGRATION_9_10 = object : Migration(9, 10) {
    override fun migrate(db: SupportSQLiteDatabase) {
        db.execSQL("ALTER TABLE profiles ADD COLUMN perAppVpnMode TEXT NOT NULL DEFAULT 'ALL'")
        db.execSQL("ALTER TABLE profiles ADD COLUMN perAppVpnPackages TEXT NOT NULL DEFAULT ''")
    }
}

@Database(
    entities = [ProfileEntity::class, MetaProfileEntity::class],
    version = 11,
    exportSchema = false,
)
abstract class AppDatabase : RoomDatabase() {
    abstract fun profileDao(): ProfileDao
    abstract fun metaProfileDao(): MetaProfileDao
}