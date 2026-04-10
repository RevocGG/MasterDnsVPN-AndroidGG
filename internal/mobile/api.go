// MasterDnsVPN  Android Mobile Adapter
// File: internal/mobile/api.go
package mobile

import (
"context"
"errors"
"fmt"
"path/filepath"
"runtime/debug"
"sync"

"masterdnsvpn-go/internal/client"
"masterdnsvpn-go/internal/config"
)

type tunnelHandle struct {
cl         *client.Client
ctx        context.Context
cancel     context.CancelFunc
done       chan struct{}
logStopCh  chan struct{}
profileDir string
listenAddr string
mu         sync.Mutex
lastErr    error
}

var (
instancesMu sync.Mutex
instances   = make(map[string]*tunnelHandle)
)

// ErrAlreadyRunning is returned when StartInstance is called for an ID that
// already has an active tunnel.
var ErrAlreadyRunning = errors.New("tunnel instance already running")

func StartInstance(instanceID string, profileDir string, cfg MobileClientConfig, resolversText string) (retErr error) {
defer func() {
if r := recover(); r != nil {
retErr = fmt.Errorf("panic in StartInstance: %v\n%s", r, debug.Stack())
}
}()
instancesMu.Lock()
defer instancesMu.Unlock()
if _, exists := instances[instanceID]; exists {
return ErrAlreadyRunning
}
if err := WriteConfigFiles(cfg, resolversText, profileDir); err != nil {
return fmt.Errorf("failed to prepare config files: %w", err)
}
configPath := ConfigFilePath(profileDir)
logPath := filepath.Join(profileDir, "client.log")
c, err := client.Bootstrap(configPath, logPath, config.ClientConfigOverrides{})
if err != nil {
return fmt.Errorf("bootstrap failed: %w", err)
}
c.PrintBanner()
listenAddr := fmt.Sprintf("%s:%d", cfg.ListenIP, cfg.ListenPort)
ctx, cancel := context.WithCancel(context.Background())
h := &tunnelHandle{
cl:         c,
ctx:        ctx,
cancel:     cancel,
done:       make(chan struct{}),
logStopCh:  make(chan struct{}),
profileDir: profileDir,
listenAddr: listenAddr,
}
go StartLogWatcher(logPath, h.logStopCh)
go func() {
defer close(h.done)
defer func() {
if r := recover(); r != nil {
h.mu.Lock()
h.lastErr = fmt.Errorf("panic in Run: %v\n%s", r, debug.Stack())
h.mu.Unlock()
}
}()
if runErr := c.Run(ctx); runErr != nil && !errors.Is(runErr, context.Canceled) {
h.mu.Lock()
h.lastErr = runErr
h.mu.Unlock()
}
}()
instances[instanceID] = h
return nil
}

func StopInstance(instanceID string) (retErr error) {
defer func() {
if r := recover(); r != nil {
retErr = fmt.Errorf("panic in StopInstance: %v\n%s", r, debug.Stack())
}
}()
instancesMu.Lock()
h, exists := instances[instanceID]
if !exists {
instancesMu.Unlock()
return nil
}
delete(instances, instanceID)
instancesMu.Unlock()
close(h.logStopCh)
h.cancel()
<-h.done
return nil
}

func StopAllInstances() {
instancesMu.Lock()
ids := make([]string, 0, len(instances))
for id := range instances {
ids = append(ids, id)
}
instancesMu.Unlock()
for _, id := range ids {
_ = StopInstance(id)
}
}

func IsInstanceRunning(instanceID string) bool {
instancesMu.Lock()
defer instancesMu.Unlock()
_, exists := instances[instanceID]
return exists
}

// getAnyClient returns the first running engine client, or nil if none.
// Used by the TUN bridge to call ProcessDNSQuery directly.
func getAnyClient() *client.Client {
instancesMu.Lock()
defer instancesMu.Unlock()
for _, h := range instances {
	if h.cl != nil {
		return h.cl
	}
}
return nil
}

func GetInstanceStats(instanceID string) MobileStats {
instancesMu.Lock()
h, exists := instances[instanceID]
instancesMu.Unlock()
if !exists {
return MobileStats{IsRunning: false}
}
conns := h.cl.Balancer().AllConnections()
valid := 0
for _, conn := range conns {
if conn.IsValid {
valid++
}
}
return MobileStats{
IsRunning:          true,
SessionReady:       h.cl.SessionReady(),
ResolverCount:      len(conns),
ValidResolverCount: valid,
ListenAddr:         h.listenAddr,
ProfileDir:         h.profileDir,
}
}

func GetInstanceLastError(instanceID string) error {
instancesMu.Lock()
h, exists := instances[instanceID]
instancesMu.Unlock()
if !exists {
return nil
}
h.mu.Lock()
defer h.mu.Unlock()
return h.lastErr
}
