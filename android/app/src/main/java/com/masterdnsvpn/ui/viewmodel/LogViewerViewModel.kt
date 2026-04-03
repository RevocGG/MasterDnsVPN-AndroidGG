package com.masterdnsvpn.ui.viewmodel

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.masterdnsvpn.bridge.GoMobileBridge
import com.masterdnsvpn.log.LogEntry
import com.masterdnsvpn.log.LogLevel
import com.masterdnsvpn.log.LogManager
import com.masterdnsvpn.log.LogcatReader
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map
import java.io.File
import javax.inject.Inject

@HiltViewModel
class LogViewerViewModel @Inject constructor(
    private val logManager: LogManager,
    private val bridge: GoMobileBridge,
    private val logcatReader: LogcatReader,
) : ViewModel() {

    init {
        // Start capturing logcat as soon as the log screen is created
        logcatReader.start(viewModelScope)
    }

    fun ensureLogCallbackRegistered() {
        bridge.registerLogCallback()
    }

    /** Source filter: null = all, APP = app only, LOGCAT = system only */
    fun filteredEntries(level: LogLevel?, source: LogEntry.Source?): Flow<List<LogEntry>> {
        return logManager.entries.map { entries ->
            entries.filter { entry ->
                (level == null || entry.level == level) &&
                (source == null || entry.source == source)
            }
        }
    }

    fun clear() = logManager.clear()

    fun export(file: File) = logManager.exportToFile(file)

    override fun onCleared() {
        logcatReader.stop()
        super.onCleared()
    }
}