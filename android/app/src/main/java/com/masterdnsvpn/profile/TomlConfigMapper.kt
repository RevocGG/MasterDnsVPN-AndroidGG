package com.masterdnsvpn.profile

/**
 * Maps between [ProfileEntity] and a .toml config string.
 *
 * The output format mirrors client_config.toml.simple exactly so a config
 * exported from the app can be used directly as a Go client config file.
 *
 * Import parses a raw .toml string line-by-line (no external library needed):
 * - KEY = VALUE  (string, int, float, bool, array)
 *
 * Fields that are irrelevant to the Android profile (like file-path fields)
 * are written as comments in the exported file but are parsed on import so
 * a file exported from the desktop client can be imported without errors.
 */
object TomlConfigMapper {

    // ─────────────────────────────────────────────────────────────────────────
    // IMPORT
    // ─────────────────────────────────────────────────────────────────────────

    /**
     * Parse [toml] text and return a [ProfileEntity] that reflects all
     * recognised keys. Any key not present in the file keeps the default from
     * [ProfileEntity].
     */
    fun fromToml(toml: String, profileName: String = "Imported Profile"): ProfileEntity {
        val kv = parseToml(toml)
        fun str(key: String) = kv[key]?.trim('"', ' ')
        fun int(key: String) = kv[key]?.trim()?.toIntOrNull()
        fun dbl(key: String) = kv[key]?.trim()?.toDoubleOrNull()
        fun bool(key: String) = kv[key]?.trim()?.lowercase()?.toBooleanStrictOrNull()

        // DOMAINS array: ["a.com", "b.com"]  → "a.com,b.com"
        // Also handles encrypted domains from locked profiles: ENC:...
        val domainsRaw = kv["DOMAINS"]
        val domains = if (domainsRaw != null) {
            val trimmed = domainsRaw.trim().trim('"')
            if (IdentityCipher.isEncrypted(trimmed)) {
                // Locked profile — decrypt the domains string
                IdentityCipher.decrypt(trimmed) ?: ""
            } else {
                domainsRaw
                    .trim('[', ']')
                    .split(",")
                    .joinToString(",") { it.trim().trim('"') }
            }
        } else ""

        // Handle encrypted ENCRYPTION_KEY from locked profiles
        val rawKey = str("ENCRYPTION_KEY") ?: ""
        val encryptionKey = if (IdentityCipher.isEncrypted(rawKey)) {
            IdentityCipher.decrypt(rawKey) ?: ""
        } else {
            rawKey
        }

        val base = ProfileEntity(name = profileName)
        return base.copy(
            // Section 1
            domains = domains,
            dataEncryptionMethod = int("DATA_ENCRYPTION_METHOD") ?: base.dataEncryptionMethod,
            encryptionKey = encryptionKey,
            // Section 2
            protocolType = str("PROTOCOL_TYPE") ?: base.protocolType,
            listenIP = str("LISTEN_IP") ?: base.listenIP,
            listenPort = int("LISTEN_PORT") ?: base.listenPort,
            socks5Auth = bool("SOCKS5_AUTH") ?: base.socks5Auth,
            socks5User = str("SOCKS5_USER") ?: base.socks5User,
            socks5Pass = str("SOCKS5_PASS") ?: base.socks5Pass,
            // Section 3
            localDnsEnabled = bool("LOCAL_DNS_ENABLED") ?: base.localDnsEnabled,
            localDnsIP = str("LOCAL_DNS_IP") ?: base.localDnsIP,
            localDnsPort = int("LOCAL_DNS_PORT") ?: base.localDnsPort,
            localDnsCacheMaxRecords = int("LOCAL_DNS_CACHE_MAX_RECORDS") ?: base.localDnsCacheMaxRecords,
            localDnsCacheTtlSeconds = dbl("LOCAL_DNS_CACHE_TTL_SECONDS") ?: base.localDnsCacheTtlSeconds,
            localDnsPendingTimeoutSec = dbl("LOCAL_DNS_PENDING_TIMEOUT_SECONDS") ?: base.localDnsPendingTimeoutSec,
            dnsResponseFragmentTimeoutSeconds = dbl("DNS_RESPONSE_FRAGMENT_TIMEOUT_SECONDS") ?: base.dnsResponseFragmentTimeoutSeconds,
            localDnsCachePersist = bool("LOCAL_DNS_CACHE_PERSIST_TO_FILE") ?: base.localDnsCachePersist,
            localDnsCacheFlushSec = dbl("LOCAL_DNS_CACHE_FLUSH_INTERVAL_SECONDS") ?: base.localDnsCacheFlushSec,
            // Section 4
            resolverBalancingStrategy = int("RESOLVER_BALANCING_STRATEGY") ?: base.resolverBalancingStrategy,
            packetDuplicationCount = int("PACKET_DUPLICATION_COUNT") ?: base.packetDuplicationCount,
            setupPacketDuplicationCount = int("SETUP_PACKET_DUPLICATION_COUNT") ?: base.setupPacketDuplicationCount,
            streamResolverFailoverResendThreshold = int("STREAM_RESOLVER_FAILOVER_RESEND_THRESHOLD") ?: base.streamResolverFailoverResendThreshold,
            streamResolverFailoverCooldownSec = dbl("STREAM_RESOLVER_FAILOVER_COOLDOWN") ?: base.streamResolverFailoverCooldownSec,
            recheckInactiveServersEnabled = bool("RECHECK_INACTIVE_SERVERS_ENABLED") ?: base.recheckInactiveServersEnabled,
            recheckInactiveIntervalSeconds = dbl("RECHECK_INACTIVE_INTERVAL_SECONDS") ?: base.recheckInactiveIntervalSeconds,
            recheckServerIntervalSeconds = dbl("RECHECK_SERVER_INTERVAL_SECONDS") ?: base.recheckServerIntervalSeconds,
            recheckBatchSize = int("RECHECK_BATCH_SIZE") ?: base.recheckBatchSize,
            autoDisableTimeoutServers = bool("AUTO_DISABLE_TIMEOUT_SERVERS") ?: base.autoDisableTimeoutServers,
            autoDisableTimeoutWindowSeconds = dbl("AUTO_DISABLE_TIMEOUT_WINDOW_SECONDS") ?: base.autoDisableTimeoutWindowSeconds,
            autoDisableMinObservations = int("AUTO_DISABLE_MIN_OBSERVATIONS") ?: base.autoDisableMinObservations,
            autoDisableCheckIntervalSeconds = dbl("AUTO_DISABLE_CHECK_INTERVAL_SECONDS") ?: base.autoDisableCheckIntervalSeconds,
            // Section 5
            baseEncodeData = bool("BASE_ENCODE_DATA") ?: base.baseEncodeData,
            uploadCompressionType = int("UPLOAD_COMPRESSION_TYPE") ?: base.uploadCompressionType,
            downloadCompressionType = int("DOWNLOAD_COMPRESSION_TYPE") ?: base.downloadCompressionType,
            compressionMinSize = int("COMPRESSION_MIN_SIZE") ?: base.compressionMinSize,
            // Section 6 MTU
            minUploadMTU = int("MIN_UPLOAD_MTU") ?: base.minUploadMTU,
            minDownloadMTU = int("MIN_DOWNLOAD_MTU") ?: base.minDownloadMTU,
            maxUploadMTU = int("MAX_UPLOAD_MTU") ?: base.maxUploadMTU,
            maxDownloadMTU = int("MAX_DOWNLOAD_MTU") ?: base.maxDownloadMTU,
            mtuTestRetries = int("MTU_TEST_RETRIES") ?: base.mtuTestRetries,
            mtuTestTimeout = dbl("MTU_TEST_TIMEOUT") ?: base.mtuTestTimeout,
            mtuTestParallelism = int("MTU_TEST_PARALLELISM") ?: base.mtuTestParallelism,
            saveMtuServersToFile = bool("SAVE_MTU_SERVERS_TO_FILE") ?: base.saveMtuServersToFile,
            mtuServersFileName = str("MTU_SERVERS_FILE_NAME") ?: base.mtuServersFileName,
            mtuServersFileFormat = str("MTU_SERVERS_FILE_FORMAT") ?: base.mtuServersFileFormat,
            mtuUsingSeparatorText = str("MTU_USING_SECTION_SEPARATOR_TEXT") ?: base.mtuUsingSeparatorText,
            mtuRemovedServerLogFormat = str("MTU_REMOVED_SERVER_LOG_FORMAT") ?: base.mtuRemovedServerLogFormat,
            mtuAddedServerLogFormat = str("MTU_ADDED_SERVER_LOG_FORMAT") ?: base.mtuAddedServerLogFormat,
            // Section 7 Workers
            tunnelReaderWorkers = int("TUNNEL_READER_WORKERS") ?: base.tunnelReaderWorkers,
            tunnelWriterWorkers = int("TUNNEL_WRITER_WORKERS") ?: base.tunnelWriterWorkers,
            tunnelProcessWorkers = int("TUNNEL_PROCESS_WORKERS") ?: base.tunnelProcessWorkers,
            tunnelPacketTimeoutSec = dbl("TUNNEL_PACKET_TIMEOUT_SECONDS") ?: base.tunnelPacketTimeoutSec,
            dispatcherIdlePollIntervalSeconds = dbl("DISPATCHER_IDLE_POLL_INTERVAL_SECONDS") ?: base.dispatcherIdlePollIntervalSeconds,
            txChannelSize = int("TX_CHANNEL_SIZE") ?: base.txChannelSize,
            rxChannelSize = int("RX_CHANNEL_SIZE") ?: base.rxChannelSize,
            resolverUdpConnectionPoolSize = int("RESOLVER_UDP_CONNECTION_POOL_SIZE") ?: base.resolverUdpConnectionPoolSize,
            streamQueueInitialCapacity = int("STREAM_QUEUE_INITIAL_CAPACITY") ?: base.streamQueueInitialCapacity,
            orphanQueueInitialCapacity = int("ORPHAN_QUEUE_INITIAL_CAPACITY") ?: base.orphanQueueInitialCapacity,
            dnsResponseFragmentStoreCap = int("DNS_RESPONSE_FRAGMENT_STORE_CAPACITY") ?: base.dnsResponseFragmentStoreCap,
            socksUdpAssociateReadTimeoutSeconds = dbl("SOCKS_UDP_ASSOCIATE_READ_TIMEOUT_SECONDS") ?: base.socksUdpAssociateReadTimeoutSeconds,
            clientTerminalStreamRetentionSeconds = dbl("CLIENT_TERMINAL_STREAM_RETENTION_SECONDS") ?: base.clientTerminalStreamRetentionSeconds,
            clientCancelledSetupRetentionSeconds = dbl("CLIENT_CANCELLED_SETUP_RETENTION_SECONDS") ?: base.clientCancelledSetupRetentionSeconds,
            sessionInitRetryBaseSeconds = dbl("SESSION_INIT_RETRY_BASE_SECONDS") ?: base.sessionInitRetryBaseSeconds,
            sessionInitRetryStepSeconds = dbl("SESSION_INIT_RETRY_STEP_SECONDS") ?: base.sessionInitRetryStepSeconds,
            sessionInitRetryLinearAfter = int("SESSION_INIT_RETRY_LINEAR_AFTER") ?: base.sessionInitRetryLinearAfter,
            sessionInitRetryMaxSeconds = dbl("SESSION_INIT_RETRY_MAX_SECONDS") ?: base.sessionInitRetryMaxSeconds,
            sessionInitBusyRetryIntervalSeconds = dbl("SESSION_INIT_BUSY_RETRY_INTERVAL_SECONDS") ?: base.sessionInitBusyRetryIntervalSeconds,
            // Section 8 Ping
            pingAggressiveIntervalSeconds = dbl("PING_AGGRESSIVE_INTERVAL_SECONDS") ?: base.pingAggressiveIntervalSeconds,
            pingLazyIntervalSeconds = dbl("PING_LAZY_INTERVAL_SECONDS") ?: base.pingLazyIntervalSeconds,
            pingCooldownIntervalSeconds = dbl("PING_COOLDOWN_INTERVAL_SECONDS") ?: base.pingCooldownIntervalSeconds,
            pingColdIntervalSeconds = dbl("PING_COLD_INTERVAL_SECONDS") ?: base.pingColdIntervalSeconds,
            pingWarmThresholdSeconds = dbl("PING_WARM_THRESHOLD_SECONDS") ?: base.pingWarmThresholdSeconds,
            pingCoolThresholdSeconds = dbl("PING_COOL_THRESHOLD_SECONDS") ?: base.pingCoolThresholdSeconds,
            pingColdThresholdSeconds = dbl("PING_COLD_THRESHOLD_SECONDS") ?: base.pingColdThresholdSeconds,
            // Section 9 ARQ
            maxPacketsPerBatch = int("MAX_PACKETS_PER_BATCH") ?: base.maxPacketsPerBatch,
            arqWindowSize = int("ARQ_WINDOW_SIZE") ?: base.arqWindowSize,
            arqInitialRtoSeconds = dbl("ARQ_INITIAL_RTO_SECONDS") ?: base.arqInitialRtoSeconds,
            arqMaxRtoSeconds = dbl("ARQ_MAX_RTO_SECONDS") ?: base.arqMaxRtoSeconds,
            arqControlInitialRtoSeconds = dbl("ARQ_CONTROL_INITIAL_RTO_SECONDS") ?: base.arqControlInitialRtoSeconds,
            arqControlMaxRtoSeconds = dbl("ARQ_CONTROL_MAX_RTO_SECONDS") ?: base.arqControlMaxRtoSeconds,
            arqMaxControlRetries = int("ARQ_MAX_CONTROL_RETRIES") ?: base.arqMaxControlRetries,
            arqInactivityTimeoutSeconds = dbl("ARQ_INACTIVITY_TIMEOUT_SECONDS") ?: base.arqInactivityTimeoutSeconds,
            arqDataPacketTtlSeconds = dbl("ARQ_DATA_PACKET_TTL_SECONDS") ?: base.arqDataPacketTtlSeconds,
            arqControlPacketTtlSeconds = dbl("ARQ_CONTROL_PACKET_TTL_SECONDS") ?: base.arqControlPacketTtlSeconds,
            arqMaxDataRetries = int("ARQ_MAX_DATA_RETRIES") ?: base.arqMaxDataRetries,
            arqDataNackMaxGap = int("ARQ_DATA_NACK_MAX_GAP") ?: base.arqDataNackMaxGap,
            arqDataNackInitialDelaySeconds = dbl("ARQ_DATA_NACK_INITIAL_DELAY_SECONDS") ?: base.arqDataNackInitialDelaySeconds,
            arqDataNackRepeatSeconds = dbl("ARQ_DATA_NACK_REPEAT_SECONDS") ?: base.arqDataNackRepeatSeconds,
            arqTerminalDrainTimeoutSec = dbl("ARQ_TERMINAL_DRAIN_TIMEOUT_SECONDS") ?: base.arqTerminalDrainTimeoutSec,
            arqTerminalAckWaitTimeoutSec = dbl("ARQ_TERMINAL_ACK_WAIT_TIMEOUT_SECONDS") ?: base.arqTerminalAckWaitTimeoutSec,
            // Section 10 Logging
            logLevel = str("LOG_LEVEL") ?: base.logLevel,
            // Identity lock flag (app-specific, not a Go config key)
            identityLocked = bool("IDENTITY_LOCKED") ?: base.identityLocked,
        )
    }

    // ─────────────────────────────────────────────────────────────────────────
    // EXPORT
    // ─────────────────────────────────────────────────────────────────────────

    /** Render [p] to a .toml string identical in layout to client_config.toml.simple.
     *  When [hideIdentity] is true, domains and encryption key are kept in the
     *  output (so the config is functional) but the IDENTITY_LOCKED flag is set,
     *  which tells the importing app to hide those fields in the UI. */
    fun toToml(p: ProfileEntity, hideIdentity: Boolean = false): String {
        // Build DOMAINS value — encrypted when locked
        val domainsArray = if (hideIdentity && p.domains.isNotBlank()) {
            // Encrypt the comma-separated domains string
            "\"${IdentityCipher.encrypt(p.domains)}\""
        } else if (p.domains.isBlank()) {
            "[]"
        } else {
            p.domains.split(",")
                .map { it.trim() }
                .filter { it.isNotBlank() }
                .joinToString(prefix = "[", postfix = "]") { "\"$it\"" }
        }

        // Encryption key value — encrypted when locked
        val encKeyValue = if (hideIdentity && p.encryptionKey.isNotBlank()) {
            IdentityCipher.encrypt(p.encryptionKey)
        } else {
            p.encryptionKey
        }

        fun d(v: Double) = if (v == v.toLong().toDouble()) "${v.toLong()}.0" else v.toString()
        fun b(v: Boolean) = if (v) "true" else "false"
        fun q(v: String) = "\"$v\""

        return buildString {
            appendLine("# ==============================================================================")
            appendLine("# MasterDnsVPN Go Client Configuration")
            appendLine("# Profile: ${p.name}")
            appendLine("# Exported by MasterDnsVPN Android App")
            appendLine("# ==============================================================================")
            appendLine()
            appendLine("# ------------------------------------------------------------------------------")
            appendLine("# 1) Tunnel Identity & Security")
            appendLine("# ------------------------------------------------------------------------------")
            appendLine()
            appendLine("DOMAINS = $domainsArray")
            appendLine()
            appendLine("# 0=None 1=XOR 2=ChaCha20 3=AES-128-GCM 4=AES-192-GCM 5=AES-256-GCM")
            appendLine("DATA_ENCRYPTION_METHOD = ${p.dataEncryptionMethod}")
            appendLine()
            appendLine("ENCRYPTION_KEY = ${q(encKeyValue)}")
            appendLine()
            appendLine("# ------------------------------------------------------------------------------")
            appendLine("# 2) Local Proxy Listener")
            appendLine("# ------------------------------------------------------------------------------")
            appendLine()
            appendLine("# \"SOCKS5\" or \"TCP\"")
            appendLine("PROTOCOL_TYPE = ${q(p.protocolType)}")
            appendLine()
            appendLine("LISTEN_IP = ${q(p.listenIP)}")
            appendLine("LISTEN_PORT = ${p.listenPort}")
            appendLine()
            appendLine("SOCKS5_AUTH = ${b(p.socks5Auth)}")
            appendLine("SOCKS5_USER = ${q(p.socks5User)}")
            appendLine("SOCKS5_PASS = ${q(p.socks5Pass)}")
            appendLine()
            appendLine("# ------------------------------------------------------------------------------")
            appendLine("# 3) Local DNS Service")
            appendLine("# ------------------------------------------------------------------------------")
            appendLine()
            appendLine("LOCAL_DNS_ENABLED = ${b(p.localDnsEnabled)}")
            appendLine("LOCAL_DNS_IP = ${q(p.localDnsIP)}")
            appendLine("LOCAL_DNS_PORT = ${p.localDnsPort}")
            appendLine()
            appendLine("LOCAL_DNS_CACHE_MAX_RECORDS = ${p.localDnsCacheMaxRecords}")
            appendLine()
            appendLine("LOCAL_DNS_CACHE_TTL_SECONDS = ${d(p.localDnsCacheTtlSeconds)}")
            appendLine()
            appendLine("LOCAL_DNS_PENDING_TIMEOUT_SECONDS = ${d(p.localDnsPendingTimeoutSec)}")
            appendLine()
            appendLine("DNS_RESPONSE_FRAGMENT_TIMEOUT_SECONDS = ${d(p.dnsResponseFragmentTimeoutSeconds)}")
            appendLine()
            appendLine("LOCAL_DNS_CACHE_PERSIST_TO_FILE = ${b(p.localDnsCachePersist)}")
            appendLine()
            appendLine("LOCAL_DNS_CACHE_FLUSH_INTERVAL_SECONDS = ${d(p.localDnsCacheFlushSec)}")
            appendLine()
            appendLine("# ------------------------------------------------------------------------------")
            appendLine("# 4) Resolver Selection, Duplication, and Health")
            appendLine("# ------------------------------------------------------------------------------")
            appendLine()
            appendLine("# 0=RoundRobin 1=Random 2=RoundRobin 3=LeastLoss 4=LowestLatency")
            appendLine("RESOLVER_BALANCING_STRATEGY = ${p.resolverBalancingStrategy}")
            appendLine()
            appendLine("PACKET_DUPLICATION_COUNT = ${p.packetDuplicationCount}")
            appendLine()
            appendLine("SETUP_PACKET_DUPLICATION_COUNT = ${p.setupPacketDuplicationCount}")
            appendLine()
            appendLine("STREAM_RESOLVER_FAILOVER_RESEND_THRESHOLD = ${p.streamResolverFailoverResendThreshold}")
            appendLine()
            appendLine("STREAM_RESOLVER_FAILOVER_COOLDOWN = ${d(p.streamResolverFailoverCooldownSec)}")
            appendLine()
            appendLine("RECHECK_INACTIVE_SERVERS_ENABLED = ${b(p.recheckInactiveServersEnabled)}")
            appendLine()
            appendLine("RECHECK_INACTIVE_INTERVAL_SECONDS = ${d(p.recheckInactiveIntervalSeconds)}")
            appendLine()
            appendLine("RECHECK_SERVER_INTERVAL_SECONDS = ${d(p.recheckServerIntervalSeconds)}")
            appendLine()
            appendLine("RECHECK_BATCH_SIZE = ${p.recheckBatchSize}")
            appendLine()
            appendLine("AUTO_DISABLE_TIMEOUT_SERVERS = ${b(p.autoDisableTimeoutServers)}")
            appendLine()
            appendLine("AUTO_DISABLE_TIMEOUT_WINDOW_SECONDS = ${d(p.autoDisableTimeoutWindowSeconds)}")
            appendLine()
            appendLine("AUTO_DISABLE_MIN_OBSERVATIONS = ${p.autoDisableMinObservations}")
            appendLine()
            appendLine("AUTO_DISABLE_CHECK_INTERVAL_SECONDS = ${d(p.autoDisableCheckIntervalSeconds)}")
            appendLine()
            appendLine("BASE_ENCODE_DATA = ${b(p.baseEncodeData)}")
            appendLine()
            appendLine("# ------------------------------------------------------------------------------")
            appendLine("# 5) Compression")
            appendLine("# ------------------------------------------------------------------------------")
            appendLine()
            appendLine("# 0=OFF 1=ZSTD 2=LZ4 3=ZLIB")
            appendLine("UPLOAD_COMPRESSION_TYPE = ${p.uploadCompressionType}")
            appendLine("DOWNLOAD_COMPRESSION_TYPE = ${p.downloadCompressionType}")
            appendLine()
            appendLine("COMPRESSION_MIN_SIZE = ${p.compressionMinSize}")
            appendLine()
            appendLine("# ------------------------------------------------------------------------------")
            appendLine("# 6) MTU Discovery")
            appendLine("# ------------------------------------------------------------------------------")
            appendLine()
            appendLine("MIN_UPLOAD_MTU = ${p.minUploadMTU}")
            appendLine("MIN_DOWNLOAD_MTU = ${p.minDownloadMTU}")
            appendLine()
            appendLine("MAX_UPLOAD_MTU = ${p.maxUploadMTU}")
            appendLine("MAX_DOWNLOAD_MTU = ${p.maxDownloadMTU}")
            appendLine()
            appendLine("MTU_TEST_RETRIES = ${p.mtuTestRetries}")
            appendLine("MTU_TEST_TIMEOUT = ${d(p.mtuTestTimeout)}")
            appendLine("MTU_TEST_PARALLELISM = ${p.mtuTestParallelism}")
            appendLine()
            appendLine("SAVE_MTU_SERVERS_TO_FILE = ${b(p.saveMtuServersToFile)}")
            appendLine()
            appendLine("MTU_SERVERS_FILE_NAME = ${q(p.mtuServersFileName)}")
            appendLine("MTU_SERVERS_FILE_FORMAT = ${q(p.mtuServersFileFormat)}")
            appendLine("MTU_USING_SECTION_SEPARATOR_TEXT = ${q(p.mtuUsingSeparatorText)}")
            appendLine("MTU_REMOVED_SERVER_LOG_FORMAT = ${q(p.mtuRemovedServerLogFormat)}")
            appendLine("MTU_ADDED_SERVER_LOG_FORMAT = ${q(p.mtuAddedServerLogFormat)}")
            appendLine()
            appendLine("# ------------------------------------------------------------------------------")
            appendLine("# 7) Runtime Workers, Queues, and Timers")
            appendLine("# ------------------------------------------------------------------------------")
            appendLine()
            appendLine("TUNNEL_READER_WORKERS = ${p.tunnelReaderWorkers}")
            appendLine("TUNNEL_WRITER_WORKERS = ${p.tunnelWriterWorkers}")
            appendLine("TUNNEL_PROCESS_WORKERS = ${p.tunnelProcessWorkers}")
            appendLine()
            appendLine("TUNNEL_PACKET_TIMEOUT_SECONDS = ${d(p.tunnelPacketTimeoutSec)}")
            appendLine()
            appendLine("DISPATCHER_IDLE_POLL_INTERVAL_SECONDS = ${d(p.dispatcherIdlePollIntervalSeconds)}")
            appendLine()
            appendLine("TX_CHANNEL_SIZE = ${p.txChannelSize}")
            appendLine("RX_CHANNEL_SIZE = ${p.rxChannelSize}")
            appendLine()
            appendLine("RESOLVER_UDP_CONNECTION_POOL_SIZE = ${p.resolverUdpConnectionPoolSize}")
            appendLine()
            appendLine("STREAM_QUEUE_INITIAL_CAPACITY = ${p.streamQueueInitialCapacity}")
            appendLine("ORPHAN_QUEUE_INITIAL_CAPACITY = ${p.orphanQueueInitialCapacity}")
            appendLine("DNS_RESPONSE_FRAGMENT_STORE_CAPACITY = ${p.dnsResponseFragmentStoreCap}")
            appendLine()
            appendLine("SOCKS_UDP_ASSOCIATE_READ_TIMEOUT_SECONDS = ${d(p.socksUdpAssociateReadTimeoutSeconds)}")
            appendLine()
            appendLine("CLIENT_TERMINAL_STREAM_RETENTION_SECONDS = ${d(p.clientTerminalStreamRetentionSeconds)}")
            appendLine()
            appendLine("CLIENT_CANCELLED_SETUP_RETENTION_SECONDS = ${d(p.clientCancelledSetupRetentionSeconds)}")
            appendLine()
            appendLine("SESSION_INIT_RETRY_BASE_SECONDS = ${d(p.sessionInitRetryBaseSeconds)}")
            appendLine("SESSION_INIT_RETRY_STEP_SECONDS = ${d(p.sessionInitRetryStepSeconds)}")
            appendLine("SESSION_INIT_RETRY_LINEAR_AFTER = ${p.sessionInitRetryLinearAfter}")
            appendLine("SESSION_INIT_RETRY_MAX_SECONDS = ${d(p.sessionInitRetryMaxSeconds)}")
            appendLine()
            appendLine("SESSION_INIT_BUSY_RETRY_INTERVAL_SECONDS = ${d(p.sessionInitBusyRetryIntervalSeconds)}")
            appendLine()
            appendLine("PING_AGGRESSIVE_INTERVAL_SECONDS = ${d(p.pingAggressiveIntervalSeconds)}")
            appendLine("PING_LAZY_INTERVAL_SECONDS = ${d(p.pingLazyIntervalSeconds)}")
            appendLine("PING_COOLDOWN_INTERVAL_SECONDS = ${d(p.pingCooldownIntervalSeconds)}")
            appendLine("PING_COLD_INTERVAL_SECONDS = ${d(p.pingColdIntervalSeconds)}")
            appendLine("PING_WARM_THRESHOLD_SECONDS = ${d(p.pingWarmThresholdSeconds)}")
            appendLine("PING_COOL_THRESHOLD_SECONDS = ${d(p.pingCoolThresholdSeconds)}")
            appendLine("PING_COLD_THRESHOLD_SECONDS = ${d(p.pingColdThresholdSeconds)}")
            appendLine()
            appendLine("# ------------------------------------------------------------------------------")
            appendLine("# 8) ARQ Reliability & Packing")
            appendLine("# ------------------------------------------------------------------------------")
            appendLine()
            appendLine("MAX_PACKETS_PER_BATCH = ${p.maxPacketsPerBatch}")
            appendLine()
            appendLine("ARQ_WINDOW_SIZE = ${p.arqWindowSize}")
            appendLine("ARQ_INITIAL_RTO_SECONDS = ${d(p.arqInitialRtoSeconds)}")
            appendLine("ARQ_MAX_RTO_SECONDS = ${d(p.arqMaxRtoSeconds)}")
            appendLine("ARQ_CONTROL_INITIAL_RTO_SECONDS = ${d(p.arqControlInitialRtoSeconds)}")
            appendLine("ARQ_CONTROL_MAX_RTO_SECONDS = ${d(p.arqControlMaxRtoSeconds)}")
            appendLine("ARQ_MAX_CONTROL_RETRIES = ${p.arqMaxControlRetries}")
            appendLine("ARQ_INACTIVITY_TIMEOUT_SECONDS = ${d(p.arqInactivityTimeoutSeconds)}")
            appendLine("ARQ_DATA_PACKET_TTL_SECONDS = ${d(p.arqDataPacketTtlSeconds)}")
            appendLine("ARQ_CONTROL_PACKET_TTL_SECONDS = ${d(p.arqControlPacketTtlSeconds)}")
            appendLine("ARQ_MAX_DATA_RETRIES = ${p.arqMaxDataRetries}")
            appendLine()
            appendLine("ARQ_DATA_NACK_MAX_GAP = ${p.arqDataNackMaxGap}")
            appendLine()
            appendLine("ARQ_DATA_NACK_INITIAL_DELAY_SECONDS = ${d(p.arqDataNackInitialDelaySeconds)}")
            appendLine("ARQ_DATA_NACK_REPEAT_SECONDS = ${d(p.arqDataNackRepeatSeconds)}")
            appendLine()
            appendLine("ARQ_TERMINAL_DRAIN_TIMEOUT_SECONDS = ${d(p.arqTerminalDrainTimeoutSec)}")
            appendLine("ARQ_TERMINAL_ACK_WAIT_TIMEOUT_SECONDS = ${d(p.arqTerminalAckWaitTimeoutSec)}")
            appendLine()
            appendLine("# ------------------------------------------------------------------------------")
            appendLine("# 9) Logging")
            appendLine("# ------------------------------------------------------------------------------")
            appendLine()
            appendLine("# DEBUG, INFO, WARN, ERROR")
            appendLine("LOG_LEVEL = ${q(p.logLevel)}")
            appendLine()
            if (hideIdentity) {
                appendLine("# App-specific: domains & key are hidden in importing app UI")
                append("IDENTITY_LOCKED = true")
            }
        }
    }

    // ─────────────────────────────────────────────────────────────────────────
    // INTERNAL PARSER  (no external dependencies)
    // ─────────────────────────────────────────────────────────────────────────

    /**
     * Very small line-by-line TOML KEY=VALUE parser.
     * Handles:
     *   KEY = "string"
     *   KEY = 123
     *   KEY = 1.5
     *   KEY = true / false
     *   KEY = ["a", "b"]       (single-line arrays only)
     *   # comment lines
     */
    private fun parseToml(toml: String): Map<String, String> {
        val result = mutableMapOf<String, String>()
        for (rawLine in toml.lineSequence()) {
            val line = rawLine.trim()
            if (line.isEmpty() || line.startsWith("#")) continue
            val eqIdx = line.indexOf('=')
            if (eqIdx < 1) continue
            val key = line.substring(0, eqIdx).trim()
            val value = line.substring(eqIdx + 1).trim()
            // strip inline comment (outside strings/arrays)
            val clean = stripInlineComment(value)
            result[key] = clean
        }
        return result
    }

    /** Strip `# ...` that appears after a value, handling quoted strings and arrays. */
    private fun stripInlineComment(value: String): String {
        var inString = false
        var inArray = false
        for (i in value.indices) {
            val ch = value[i]
            when {
                ch == '"' && !inArray -> inString = !inString
                ch == '[' && !inString -> inArray = true
                ch == ']' && !inString -> inArray = false
                ch == '#' && !inString && !inArray -> return value.substring(0, i).trim()
            }
        }
        return value.trim()
    }
}
