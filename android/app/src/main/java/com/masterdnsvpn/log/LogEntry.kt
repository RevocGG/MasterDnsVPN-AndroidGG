package com.masterdnsvpn.log

data class LogEntry(
    val level: LogLevel,
    val timestamp: String,   // raw string from Go (UTC) — kept for export
    val message: String,
    val epochMs: Long = 0L,  // local-time epoch for display; 0 = "system" entries
    val source: Source = Source.APP,
) {
    enum class Source { APP, LOGCAT }
}