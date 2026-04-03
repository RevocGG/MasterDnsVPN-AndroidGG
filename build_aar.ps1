# ==============================================================================
# MasterDnsVPN - Build gomobile .aar
# Usage: .\build_aar.ps1
# Requirements:
#   - Android NDK r26+ somewhere on disk; set ANDROID_NDK_HOME if needed.
#   - gomobile installed: go install golang.org/x/mobile/cmd/gomobile@latest
#                         gomobile init
# ==============================================================================

$ErrorActionPreference = "Stop"
$Root = $PSScriptRoot
$OutDir = "$Root\android\app\libs"

Write-Host "==> MasterDnsVPN - building gomobile .aar" -ForegroundColor Cyan

# Auto-detect Android NDK if ANDROID_NDK_HOME is not set
if (-not $env:ANDROID_NDK_HOME) {
    $sdkRoot = $env:ANDROID_HOME
    if (-not $sdkRoot) { $sdkRoot = "$env:USERPROFILE\AppData\Local\Android\Sdk" }
    $ndkDir = "$sdkRoot\ndk"
    if (Test-Path $ndkDir) {
        $latest = Get-ChildItem $ndkDir | Sort-Object Name -Descending | Select-Object -First 1
        if ($latest) {
            $env:ANDROID_NDK_HOME = $latest.FullName.Trim()
            Write-Host "  NDK auto-detected: $env:ANDROID_NDK_HOME" -ForegroundColor DarkGray
        }
    }
}

# Ensure gomobile is on PATH
if (-not (Get-Command gomobile -ErrorAction SilentlyContinue)) {
    Write-Host "gomobile not found. Installing..." -ForegroundColor Yellow
    go install golang.org/x/mobile/cmd/gomobile@latest
    gomobile init
}

# Allow gomobile to download its bind package (not in vendor)
$env:GOFLAGS = "-mod=mod"

Push-Location $Root
try {
    gomobile bind `
        -v `
        -target android `
        -androidapi 26 `
        -javapkg com.masterdnsvpn.gomobile `
        -o "$OutDir\masterdnsvpn.aar" `
        masterdnsvpn-go/cmd/android

    Write-Host ""
    Write-Host "==> Done! .aar written to: $OutDir\masterdnsvpn.aar" -ForegroundColor Green
} finally {
    Pop-Location
}
