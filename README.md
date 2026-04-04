# MasterDnsVPN Android Client

## | [ نسخه فارسی](README_FA.md) |

> **This is the Android client for [MasterDnsVPN](https://github.com/masterking32/MasterDnsVPN).**
> Before using this app, you must install and configure the **server side** from the main project first.
> The app will not work without a running MasterDnsVPN server.

---

## What is MasterDnsVPN?

[MasterDnsVPN](https://github.com/masterking32/MasterDnsVPN) is a high-performance DNS-over-UDP tunnel that encrypts and routes your traffic through a remote server, bypassing censorship and surveillance. This repository contains the **Android client app** built on top of the same Go engine.

---

## Requirements

| Tool | Version | Download |
|------|---------|----------|
| Go | 1.22+ | https://go.dev/dl |
| Android Studio | Ladybug+ | https://developer.android.com/studio |
| Android SDK | API 26+ (target API 35) | via Android Studio SDK Manager |
| Android NDK | 27.x | via Android Studio SDK Manager |
| gomobile | latest | `go install golang.org/x/mobile/cmd/gomobile@latest` |

### One-time gomobile init
```powershell
go install golang.org/x/mobile/cmd/gomobile@latest
gomobile init
```

---

## Building

### Full build (AAR + APK)
```powershell
.\build_aar.ps1
```

This script:
1. Detects the installed NDK automatically
2. Runs `gomobile bind` → builds `android/app/libs/masterdnsvpn.aar`
3. Runs `gradlew assembleDebug` → produces the APK

### Output files
| File | Path |
|------|------|
| Go AAR library | `android/app/libs/masterdnsvpn.aar` |
| Debug APK | `android/app/build/outputs/apk/debug/` |

### Manual build steps
```powershell
# Step 1 — Build AAR (arm64 only)
$env:GOFLAGS = "-mod=mod"
$env:ANDROID_HOME = "$env:USERPROFILE\AppData\Local\Android\Sdk"
$ndkVer = (Get-ChildItem "$env:ANDROID_HOME\ndk" | Sort-Object Name -Descending | Select-Object -First 1).Name
$env:ANDROID_NDK_HOME = "$env:ANDROID_HOME\ndk\$ndkVer"
gomobile bind -target android/arm64 -androidapi 26 -javapkg com.masterdnsvpn.gomobile `
  -o "android\app\libs\masterdnsvpn.aar" masterdnsvpn-go/cmd/android

# Step 2 — Build APK
cd android
.\gradlew.bat assembleDebug

# Step 3 — Install on device
adb install "app\build\outputs\apk\debug\MasterDnsVPN-1.0.0-beta-arm64-v8a.apk"
```

---

## App Features

### Profile Management
- **Create / Edit / Delete** profiles — each profile connects to one MasterDnsVPN server
- All client configuration fields exposed in the UI (server address, port, encryption, compression, DNS, SOCKS5, TUN mode, balancer strategy, MTU, etc.)
- **Resolver editor** — add/remove DNS resolvers per profile
- **Tunnel mode per profile**: SOCKS5 proxy or TUN (system-wide VPN)

### Meta Profile (Load Balancer)
- Groups multiple profiles under a single start/stop control
- Distributes connections across all sub-profiles using one of four strategies:
  - **RoundRobin** — rotate in order
  - **Random** — random selection
  - **LeastConn** — pick proxy with fewest active connections
  - **LowestLatency** — pick the fastest proxy
- Supports both SOCKS5 and TUN tunnel modes

### TUN Mode (System-wide VPN)
- Uses Android `VpnService` to capture all device traffic
- **tun2socks bridge** (gVisor netstack): converts TCP/UDP flows to SOCKS5, forwards through the Go engine
- Per-app filtering: All apps / Include list / Exclude list

### Hotspot Sharing
- Share VPN over Wi-Fi hotspot
- TCP relay on `0.0.0.0:8090` → shows address/port to configure on connected devices

### Monitoring
- Real-time per-profile stats: resolver health, listen address, session state
- CPU / RAM / upload speed / download speed updated every second
- Per-profile cumulative data usage (persisted across restarts)
- Color-coded real-time log viewer

---

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│  Android App (Kotlin + Compose)                          │
│                                                          │
│  HomeScreen ──► HomeViewModel ──► TunnelController       │
│                                        │                 │
│                          ┌─────────────┴──────────────┐  │
│                          │  DnsTunnelVpnService (TUN) │  │
│                          │  DnsTunnelProxyService     │  │
│                          └─────────────┬──────────────┘  │
│                                        │                 │
│                              GoMobileBridge (JNI)        │
└──────────────────────────────────────────────────────────┘
                                        │
                        ┌───────────────▼──────────────────┐
                        │  Go Engine (masterdnsvpn.aar)    │
                        │  StartInstance / StopInstance    │
                        │  StartTunBridge (gVisor)         │
                        │  StartSocksBalancer (meta)       │
                        │  GetStats / GetBandwidth         │
                        └──────────────────────────────────┘
```

### TUN traffic flow
```
Device apps → TUN fd → gVisor netstack → SOCKS5 (Go) → DNS tunnel → MasterDnsVPN server
```

### Meta TUN flow
```
Device apps → TUN fd → gVisor → SOCKS Balancer ─┬─ Profile-1 → server-1
                                                 ├─ Profile-2 → server-2
                                                 └─ Profile-N → server-N
```

---

## License

See [LICENSE](LICENSE).

## Related

- **Server & main project**: [github.com/masterking32/MasterDnsVPN](https://github.com/masterking32/MasterDnsVPN)
