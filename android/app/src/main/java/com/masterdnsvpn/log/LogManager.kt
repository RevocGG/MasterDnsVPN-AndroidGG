package com.masterdnsvpn.log

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import java.io.File
import java.io.IOException
import java.util.concurrent.atomic.AtomicBoolean
import javax.inject.Inject
import javax.inject.Singleton

/**
 * In-memory ring buffer for Go tunnel log entries.
 *
 * The Go layer calls [append] via the gomobile log callback.
 * The log viewer screen observes [entries] as a [StateFlow].
 *
 * Ring buffer: keeps the most recent [MAX_ENTRIES] entries.
 * Thread-safe: all mutation happens on whatever thread the Go callback arrives
 * on, protected by [synchronized].
 *
 * StateFlow emission is debounced to at most once per [FLUSH_INTERVAL_MS] to
 * avoid flooding the Compose recomposition engine during high-throughput scans.
 */
@Singleton
class LogManager @Inject constructor() {

    companion object {
        private const val MAX_ENTRIES = 5_000
        private const val FLUSH_INTERVAL_MS = 250L
    }

    private val buffer = ArrayDeque<LogEntry>(MAX_ENTRIES + 1)
    private val _entries = MutableStateFlow<List<LogEntry>>(emptyList())

    /** Observed by the UI; emits a debounced snapshot (max once per 250 ms). */
    val entries: StateFlow<List<LogEntry>> = _entries.asStateFlow()

    private val flushScope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val pendingFlush = AtomicBoolean(false)

    fun append(entry: LogEntry) {
        synchronized(buffer) {
            buffer.addLast(entry)
            if (buffer.size > MAX_ENTRIES) buffer.removeFirst()
        }
        // Schedule a debounced flush — if one is already pending, do nothing.
        if (pendingFlush.compareAndSet(false, true)) {
            flushScope.launch {
                delay(FLUSH_INTERVAL_MS)
                pendingFlush.set(false)
                val snapshot = synchronized(buffer) { buffer.toList() }
                _entries.value = snapshot
            }
        }
    }

    /** Convenience for Android-side (non-Go) log entries — uses current device time. */
    fun appendSystem(level: LogLevel, message: String) {
        append(LogEntry(level = level, timestamp = "system", message = message, epochMs = System.currentTimeMillis()))
    }

    fun clear() {
        synchronized(buffer) {
            buffer.clear()
            _entries.value = emptyList()
        }
    }

    /**
     * Export all entries to [file] as plain text.
     * Each line: "YYYY/MM/DD HH:MM:SS [LEVEL] message"
     */
    @Throws(IOException::class)
    fun exportToFile(file: File) {
        val snapshot = synchronized(buffer) { buffer.toList() }
        file.bufferedWriter().use { w ->
            snapshot.forEach { e ->
                w.write("${e.timestamp} [${e.level.label}] ${e.message}")
                w.newLine()
            }
        }
    }
}