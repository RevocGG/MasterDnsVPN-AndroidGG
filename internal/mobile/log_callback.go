// MasterDnsVPN  Android Mobile Adapter
// File: internal/mobile/log_callback.go
package mobile

import (
"bufio"
"io"
"os"
"strings"
"sync/atomic"
"time"
)

const (
LogLevelDebug = 0
LogLevelInfo  = 1
LogLevelWarn  = 2
LogLevelError = 3
)

type LogEntry struct {
Level     int
Timestamp string
Message   string
}

type LogCallbackFn func(entry LogEntry)

var globalLogCallback atomic.Pointer[LogCallbackFn]

func SetLogCallback(fn LogCallbackFn) {
if fn == nil {
globalLogCallback.Store(nil)
} else {
globalLogCallback.Store(&fn)
}
}

func deliverLogEntry(entry LogEntry) {
ptr := globalLogCallback.Load()
if ptr == nil {
return
}
(*ptr)(entry)
}

func parseLogEntry(line string) LogEntry {
entry := LogEntry{Level: LogLevelInfo, Message: line}
if len(line) < 20 {
return entry
}
entry.Timestamp = line[:19]
rest := strings.TrimSpace(line[19:])
for _, pair := range []struct {
tag   string
level int
}{
{"[DEBUG]", LogLevelDebug},
{"[INFO]", LogLevelInfo},
{"[WARN]", LogLevelWarn},
{"[ERROR]", LogLevelError},
} {
if strings.HasPrefix(rest, pair.tag) {
entry.Level = pair.level
entry.Message = strings.TrimSpace(rest[len(pair.tag):])
return entry
}
}
entry.Message = rest
return entry
}

func StartLogWatcher(logFilePath string, stopCh <-chan struct{}) {
deadline := time.Now().Add(5 * time.Second)
var f *os.File
for f == nil {
select {
case <-stopCh:
return
default:
}
var err error
f, err = os.Open(logFilePath)
if err != nil {
if time.Now().After(deadline) {
return
}
time.Sleep(100 * time.Millisecond)
}
}
defer f.Close()
_, _ = f.Seek(0, io.SeekEnd)
reader := bufio.NewReader(f)
var partial strings.Builder
for {
select {
case <-stopCh:
return
default:
}
line, err := reader.ReadString('\n')
if len(line) > 0 {
partial.WriteString(line)
}
if err == nil {
raw := strings.TrimRight(partial.String(), "\r\n")
partial.Reset()
if raw != "" {
deliverLogEntry(parseLogEntry(raw))
}
} else if err == io.EOF {
time.Sleep(50 * time.Millisecond)
} else {
return
}
}
}
