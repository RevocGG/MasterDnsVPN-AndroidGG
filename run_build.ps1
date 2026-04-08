$ErrorActionPreference = "Continue"
$env:GOFLAGS = "-mod=mod"
$env:ANDROID_HOME = "$env:USERPROFILE\AppData\Local\Android\Sdk"
$ndkDir = Get-ChildItem "$env:ANDROID_HOME\ndk" -ErrorAction SilentlyContinue | Sort-Object Name -Descending | Select-Object -First 1
if ($ndkDir) {
    $env:ANDROID_NDK_HOME = $ndkDir.FullName
    Write-Host "NDK=$($ndkDir.FullName)"
} else {
    Write-Host "ERROR: No NDK found"
    exit 1
}

Write-Host "=== Step 1: gomobile bind ==="
& gomobile bind -v -target android -androidapi 26 -javapkg com.masterdnsvpn.gomobile -o "android\app\libs\masterdnsvpn.aar" masterdnsvpn-go/cmd/android *>&1 | ForEach-Object { Write-Host $_ }
if ($LASTEXITCODE -ne 0) {
    Write-Host "ERROR: gomobile bind failed with exit code $LASTEXITCODE"
    Write-Host "Full output above"
    exit 1
}
Write-Host "=== AAR built successfully ==="

Write-Host "=== Step 2: Gradle assembleDebug ==="
Push-Location android
.\gradlew.bat assembleDebug 2>&1
$gradleExit = $LASTEXITCODE
Pop-Location
if ($gradleExit -ne 0) {
    Write-Host "ERROR: Gradle build failed with exit code $gradleExit"
    exit 1
}
Write-Host "=== APK built successfully ==="
Get-ChildItem "android\app\build\outputs\apk\debug\*.apk" | ForEach-Object {
    Write-Host "APK: $($_.FullName) ($([math]::Round($_.Length/1MB,1)) MB)"
}
