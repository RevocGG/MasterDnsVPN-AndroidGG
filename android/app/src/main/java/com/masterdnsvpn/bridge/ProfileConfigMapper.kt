package com.masterdnsvpn.bridge

import com.masterdnsvpn.profile.ProfileEntity
import com.masterdnsvpn.gomobile.mobile.MobileConfig

/**
 * Maps a [ProfileEntity] (stored in Room) to the gomobile [MobileConfig]
 * that the Go tunnel runtime accepts.
 *
 * Every field of [ProfileEntity] corresponds 1-to-1 with a field on
 * [MobileConfig].  When the Go client adds new config fields, add them here
 * after updating [ProfileEntity].
 */
object ProfileConfigMapper {

    fun toMobileConfig(p: ProfileEntity): MobileConfig {
        val c = MobileConfig()
        // Section 1: Identity
        c.domains = p.domains
        c.dataEncryptionMethod = p.dataEncryptionMethod.toLong()
        c.encryptionKey = p.encryptionKey

        // Section 2: Proxy Listener
        c.protocolType = p.protocolType
        c.listenIP = p.listenIP
        c.listenPort = p.listenPort.toLong()
        c.setSOCKS5Auth(p.socks5Auth)
        c.setSOCKS5User(p.socks5User)
        c.setSOCKS5Pass(p.socks5Pass)

        // Section 3: Local DNS
        c.localDNSEnabled = p.localDnsEnabled
        c.localDNSIP = p.localDnsIP
        c.localDNSPort = p.localDnsPort.toLong()
        c.localDNSCacheMaxRecords = p.localDnsCacheMaxRecords.toLong()
        c.localDNSCacheTTLSec = p.localDnsCacheTtlSeconds
        c.localDNSPendingTimeout = p.localDnsPendingTimeoutSec
        c.localDNSCachePersist = p.localDnsCachePersist
        c.localDNSCacheFlushSec = p.localDnsCacheFlushSec

        // Section 4: Balancing
        c.resolverBalancingStrategy = p.resolverBalancingStrategy.toLong()
        c.packetDuplicationCount = p.packetDuplicationCount.toLong()
        c.setupDuplicationCount = p.setupPacketDuplicationCount.toLong()
        c.streamFailoverResendThreshold = p.streamResolverFailoverResendThreshold.toLong()
        c.streamFailoverCooldownSec = p.streamResolverFailoverCooldownSec

        // Section 5: Resolver Health
        c.recheckInactiveEnabled = p.recheckInactiveServersEnabled
        c.recheckInactiveIntervalSec = p.recheckInactiveIntervalSeconds
        c.recheckServerIntervalSec = p.recheckServerIntervalSeconds
        c.recheckBatchSize = p.recheckBatchSize.toLong()
        c.autoDisableTimeoutServers = p.autoDisableTimeoutServers
        c.autoDisableTimeoutWindowSec = p.autoDisableTimeoutWindowSeconds
        c.autoDisableMinObservations = p.autoDisableMinObservations.toLong()
        c.autoDisableCheckIntervalSec = p.autoDisableCheckIntervalSeconds

        // Section 6: Encoding/Compression
        c.baseEncodeData = p.baseEncodeData
        c.uploadCompressionType = p.uploadCompressionType.toLong()
        c.downloadCompressionType = p.downloadCompressionType.toLong()
        c.compressionMinSize = p.compressionMinSize.toLong()

        // Section 7: MTU
        c.minUploadMTU = p.minUploadMTU.toLong()
        c.minDownloadMTU = p.minDownloadMTU.toLong()
        c.maxUploadMTU = p.maxUploadMTU.toLong()
        c.maxDownloadMTU = p.maxDownloadMTU.toLong()
        c.setMTUTestRetries(p.mtuTestRetries.toLong())
        c.setMTUTestTimeout(p.mtuTestTimeout)
        c.setMTUTestParallel(p.mtuTestParallelism.toLong())

        // Section 8: Workers & Timeouts
        c.rxTxWorkers = p.rxTxWorkers.toLong()
        c.tunnelProcessWorkers = p.tunnelProcessWorkers.toLong()
        c.tunnelPacketTimeoutSec = p.tunnelPacketTimeoutSec
        c.dispatcherIdlePollIntervalSec = p.dispatcherIdlePollIntervalSeconds

        // Section 9: Ping
        c.pingAggressiveIntervalSec = p.pingAggressiveIntervalSeconds
        c.pingLazyIntervalSec = p.pingLazyIntervalSeconds
        c.pingCooldownIntervalSec = p.pingCooldownIntervalSeconds
        c.pingColdIntervalSec = p.pingColdIntervalSeconds
        c.pingWarmThresholdSec = p.pingWarmThresholdSeconds
        c.pingCoolThresholdSec = p.pingCoolThresholdSeconds
        c.pingColdThresholdSec = p.pingColdThresholdSeconds

        // Section 10: Advanced
        c.setTXChannelSize(p.txChannelSize.toLong())
        c.setRXChannelSize(p.rxChannelSize.toLong())
        c.resolverUDPPoolSize = p.resolverUdpConnectionPoolSize.toLong()
        c.streamQueueInitCap = p.streamQueueInitialCapacity.toLong()
        c.orphanQueueInitCap = p.orphanQueueInitialCapacity.toLong()
        c.setDNSFragmentStoreCap(p.dnsResponseFragmentStoreCap.toLong())
        c.setDNSFragmentTimeoutSec(p.dnsResponseFragmentTimeoutSeconds)
        c.setSOCKSUDPAssocReadTimeoutSec(p.socksUdpAssociateReadTimeoutSeconds)
        c.clientTerminalStreamRetentionSec = p.clientTerminalStreamRetentionSeconds
        c.clientCancelledSetupRetentionSec = p.clientCancelledSetupRetentionSeconds
        c.sessionInitRetryBaseSec = p.sessionInitRetryBaseSeconds
        c.sessionInitRetryStepSec = p.sessionInitRetryStepSeconds
        c.sessionInitRetryLinearAfter = p.sessionInitRetryLinearAfter.toLong()
        c.sessionInitRetryMaxSec = p.sessionInitRetryMaxSeconds
        c.sessionInitBusyRetryIntervalSec = p.sessionInitBusyRetryIntervalSeconds

        // Section 11: MTU files
        c.saveMTUServersToFile = p.saveMtuServersToFile
        // If a custom output directory is set, prefix the filename with that directory so
        // the Go runtime writes the file to accessible storage (e.g. Downloads folder).
        val effectiveMtuFileName = if (p.mtuServersFileDir.isNotBlank()) {
            "${p.mtuServersFileDir.trimEnd('/')}/${p.mtuServersFileName}"
        } else {
            p.mtuServersFileName
        }
        c.setMTUServersFileName(effectiveMtuFileName)
        c.setMTUServersFileFormat(p.mtuServersFileFormat)
        c.setMTUUsingSeparatorText(p.mtuUsingSeparatorText)
        c.setMTURemovedServerLogFormat(p.mtuRemovedServerLogFormat)
        c.setMTUAddedServerLogFormat(p.mtuAddedServerLogFormat)

        // Section 12: Logging
        c.logLevel = p.logLevel

        // Section 13: ARQ
        c.maxPacketsPerBatch = p.maxPacketsPerBatch.toLong()
        c.setARQWindowSize(p.arqWindowSize.toLong())
        c.setARQInitialRTOSec(p.arqInitialRtoSeconds)
        c.setARQMaxRTOSec(p.arqMaxRtoSeconds)
        c.setARQControlInitialRTOSec(p.arqControlInitialRtoSeconds)
        c.setARQControlMaxRTOSec(p.arqControlMaxRtoSeconds)
        c.setARQMaxControlRetries(p.arqMaxControlRetries.toLong())
        c.setARQInactivityTimeoutSec(p.arqInactivityTimeoutSeconds)
        c.setARQDataPacketTTLSec(p.arqDataPacketTtlSeconds)
        c.setARQControlPacketTTLSec(p.arqControlPacketTtlSeconds)
        c.setARQMaxDataRetries(p.arqMaxDataRetries.toLong())
        c.setARQDataNackMaxGap(p.arqDataNackMaxGap.toLong())
        c.setARQDataNackInitialDelaySec(p.arqDataNackInitialDelaySeconds)
        c.setARQDataNackRepeatSec(p.arqDataNackRepeatSeconds)
        c.setARQTerminalDrainTimeoutSec(p.arqTerminalDrainTimeoutSec)
        c.setARQTerminalAckWaitTimeoutSec(p.arqTerminalAckWaitTimeoutSec)

        return c
    }
}