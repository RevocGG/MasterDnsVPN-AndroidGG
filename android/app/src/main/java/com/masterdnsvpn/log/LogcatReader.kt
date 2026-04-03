package com.masterdnsvpn.log

import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Reads Android logcat output in a background coroutine and feeds parsed entries
 * into [LogManager] tagged with [LogEntry.Source.LOGCAT].
 *
 * Captures ONLY pure Android/system entries — NOT Go bridge logs that are already
 * emitted into the App log via the LogCallback mechanism:
 *  - All tags at WARN+ level  (system errors, OOM, ANR, etc.)
 *  - MasterDnsVPN_CRASH at VERBOSE+ (our UncaughtExceptionHandler)
 *  - AndroidRuntime at ERROR+ (JVM crashes)
 *  - GoLog and mobile are explicitly SILENCED to avoid duplication with App tab
 */
@Singleton
class LogcatReader @Inject constructor(
    private val logManager: LogManager,
) {
    private var job: Job? = null
    private var process: Process? = null

    fun start(scope: CoroutineScope) {
        if (job?.isActive == true) return
        job = scope.launch(Dispatchers.IO) {
            try {
                val proc = Runtime.getRuntime().exec(
                    arrayOf(
                        "logcat", "-v", "time",
                        "MasterDnsVPN_CRASH:V",   // our crash handler
                        "AndroidRuntime:E",        // JVM crashes
                        "GoLog:S",                 // silence — already in App tab
                        "mobile:S",                // silence — already in App tab
                        "*:W",                     // all other tags at WARN+
                    )
                )
                process = proc
                proc.inputStream.bufferedReader().use { reader ->
                    while (isActive) {
                        val line = reader.readLine() ?: break
                        parseAndEmit(line)
                    }
                }
            } catch (_: CancellationException) {
                throw CancellationException()
            } catch (_: Exception) {
            } finally {
                process?.destroy()
                process = null
            }
        }
    }

    fun stop() {
        job?.cancel()
        process?.destroy()
        process = null
    }

    // Logcat -v time format: "MM-DD HH:MM:SS.mmm  PID  TID LEVEL/TAG: message"
    // e.g.: "04-02 15:30:45.123 12345 12346 E AndroidRuntime: FATAL EXCEPTION..."
    private fun parseAndEmit(line: String) {
        val trimmed = line.trim()
        // Skip header lines printed by logcat itself
        if (trimmed.startsWith("-----") || trimmed.isEmpty()) return

        // Level char is at index 31 in -v time format, but parsing is fragile.
        // Safer: find the first " X/" or " X " pattern that looks like a level.
        val level = detectLevel(trimmed)
        logManager.append(
            LogEntry(
                level = level,
                timestamp = extractTimestamp(trimmed),
                message = trimmed,
                epochMs = System.currentTimeMillis(),
                source = LogEntry.Source.LOGCAT,
            )
        )
    }

    private fun detectLevel(line: String): LogLevel {
        // logcat -v time:  "MM-DD HH:MM:SS.mmm  PID  TID L/Tag: ..."
        // The level char L appears after two numeric groups, e.g.: "... 1234  567 W/Tag:"
        // We scan for a standalone single-char level surrounded by spaces / slashes.
        val parts = line.split(" ")
        for (part in parts) {
            when {
                part == "E" || part.startsWith("E/") -> return LogLevel.ERROR
                part == "W" || part.startsWith("W/") -> return LogLevel.WARN
                part == "I" || part.startsWith("I/") -> return LogLevel.INFO
                part == "D" || part.startsWith("D/") -> return LogLevel.DEBUG
                part == "V" || part.startsWith("V/") -> return LogLevel.DEBUG
            }
        }
        return LogLevel.DEBUG
    }

    private fun extractTimestamp(line: String): String {
        // First token is date, second is time
        val tokens = line.trimStart().split(" ", limit = 3)
        return if (tokens.size >= 2) "${tokens[0]} ${tokens[1]}" else "logcat"
    }
}
