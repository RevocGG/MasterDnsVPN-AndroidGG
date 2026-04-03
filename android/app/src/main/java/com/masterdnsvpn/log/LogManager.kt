package com.masterdnsvpn.log

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import java.io.File
import java.io.IOException
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
 */
@Singleton
class LogManager @Inject constructor() {

    companion object {
        private const val MAX_ENTRIES = 5_000
    }

    private val buffer = ArrayDeque<LogEntry>(MAX_ENTRIES + 1)
    private val _entries = MutableStateFlow<List<LogEntry>>(emptyList())

    /** Observed by the UI; emits a new snapshot on each append. */
    val entries: StateFlow<List<LogEntry>> = _entries.asStateFlow()

    fun append(entry: LogEntry) {
        synchronized(buffer) {
            buffer.addLast(entry)
            if (buffer.size > MAX_ENTRIES) buffer.removeFirst()
            _entries.value = buffer.toList()
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