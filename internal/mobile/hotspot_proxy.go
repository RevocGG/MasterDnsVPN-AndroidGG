//go:build linux

package mobile

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
)

var (
	hotspotListener    net.Listener
	hotspotRunning     atomic.Bool
	hotspotMu          sync.Mutex
	hotspotUpstream    string
	hotspotPacServer   *http.Server
	hotspotPacPort     int
	hotspotPacSocksIP  string
	hotspotPacSocksPort int
)

// StartHotspotProxy starts a TCP relay that listens on all interfaces at
// the given port and forwards every connection directly to the local
// SOCKS5 proxy (upstreamSocksAddr, e.g. "127.0.0.1:1080").
//
// It also starts a PAC file HTTP server on listenPort+1 that serves a
// Proxy Auto-Config file pointing all traffic to the hotspot SOCKS5 address.
//
// Returns the actual "IP:port" the SOCKS relay is listening on, or an error.
func StartHotspotProxy(listenPort int32, upstreamSocksAddr string) (string, error) {
	hotspotMu.Lock()
	defer hotspotMu.Unlock()

	// Stop any previous instance first.
	stopHotspotProxyLocked()

	ln, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", listenPort))
	if err != nil {
		return "", fmt.Errorf("hotspot proxy listen: %w", err)
	}

	hotspotListener = ln
	hotspotUpstream = upstreamSocksAddr
	hotspotRunning.Store(true)

	go hotspotAcceptLoop(ln, upstreamSocksAddr)

	// Start PAC file server on listenPort+1
	pacPort := int(listenPort) + 1
	hotspotPacPort = pacPort
	mux := http.NewServeMux()
	mux.HandleFunc("/proxy.pac", hotspotPacHandler)
	mux.HandleFunc("/", hotspotPacHandler) // convenience alias
	srv := &http.Server{Addr: fmt.Sprintf("0.0.0.0:%d", pacPort), Handler: mux}
	hotspotPacServer = srv
	go func() {
		_ = srv.ListenAndServe()
	}()

	return ln.Addr().String(), nil
}

// GetHotspotPacPort returns the port the PAC HTTP server is listening on.
// PAC URL would be http://<hotspot-ip>:<port>/proxy.pac
func GetHotspotPacPort() int32 {
	hotspotMu.Lock()
	defer hotspotMu.Unlock()
	return int32(hotspotPacPort)
}

// SetHotspotPacSocksAddr sets the SOCKS proxy address embedded in the PAC file.
// Call this after discovering the hotspot AP interface IP.
// format: "ip:port", e.g. "192.168.43.1:8090"
func SetHotspotPacSocksAddr(addr string) {
	hotspotMu.Lock()
	defer hotspotMu.Unlock()
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return
	}
	hotspotPacSocksIP = host
	var p int
	if n, _ := fmt.Sscanf(portStr, "%d", &p); n == 1 {
		hotspotPacSocksPort = p
	}
}

func hotspotPacHandler(w http.ResponseWriter, r *http.Request) {
	hotspotMu.Lock()
	socksIP := hotspotPacSocksIP
	socksPort := hotspotPacSocksPort
	hotspotMu.Unlock()

	if socksIP == "" {
		http.Error(w, "SOCKS address not configured", http.StatusServiceUnavailable)
		return
	}

	pac := fmt.Sprintf(`function FindProxyForURL(url, host) {
    return "SOCKS5 %s:%d; SOCKS %s:%d; DIRECT";
}`, socksIP, socksPort, socksIP, socksPort)

	w.Header().Set("Content-Type", "application/x-ns-proxy-autoconfig")
	_, _ = fmt.Fprint(w, pac)
}

// StopHotspotProxy stops the hotspot relay.
func StopHotspotProxy() {
	hotspotMu.Lock()
	defer hotspotMu.Unlock()
	stopHotspotProxyLocked()
}

// IsHotspotProxyRunning returns true if the hotspot relay is active.
func IsHotspotProxyRunning() bool {
	return hotspotRunning.Load()
}

func stopHotspotProxyLocked() {
	hotspotRunning.Store(false)
	if hotspotListener != nil {
		_ = hotspotListener.Close()
		hotspotListener = nil
	}
	if hotspotPacServer != nil {
		_ = hotspotPacServer.Close()
		hotspotPacServer = nil
	}
	hotspotPacPort = 0
	hotspotPacSocksIP = ""
	hotspotPacSocksPort = 0
}

func hotspotAcceptLoop(ln net.Listener, upstream string) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			// Listener was closed — normal shutdown.
			break
		}
		go hotspotRelay(conn, upstream)
	}
}

// hotspotRelay pipes bytes between the hotspot client and the upstream
// SOCKS5 server. Both sides speak SOCKS5, so no protocol translation is
// needed — we are purely a transparent TCP relay.
func hotspotRelay(client net.Conn, upstreamAddr string) {
	defer client.Close()

	upstream, err := net.Dial("tcp", upstreamAddr)
	if err != nil {
		return
	}
	defer upstream.Close()

	done := make(chan struct{}, 2)

	go func() {
		_, _ = io.Copy(upstream, client)
		// Signal the other direction to stop by closing the write half.
		if tc, ok := upstream.(*net.TCPConn); ok {
			_ = tc.CloseWrite()
		}
		done <- struct{}{}
	}()

	go func() {
		_, _ = io.Copy(client, upstream)
		if tc, ok := client.(*net.TCPConn); ok {
			_ = tc.CloseWrite()
		}
		done <- struct{}{}
	}()

	// Wait for both directions to finish.
	<-done
	<-done
}
