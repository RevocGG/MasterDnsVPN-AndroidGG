# کلاینت اندروید MasterDnsVPN

## | [🇬🇧 English Version](README.md) |

> **این اپ، کلاینت اندروید پروژه [MasterDnsVPN](https://github.com/masterking32/MasterDnsVPN) است.**
> قبل از استفاده از این اپ، باید ابتدا **سمت سرور** پروژه اصلی را نصب و راه‌اندازی کنید.
> بدون داشتن یک سرور MasterDnsVPN فعال، این اپ کار نخواهد کرد.

---

## دانلود مستقیم

> **نمی‌خواهید سورس رو بیلد کنید؟** آخرین نسخه APK را مستقیماً از صفحه [**Releases**](https://github.com/RevocGG/MasterDnsVPN-AndroidGG/releases/latest) دانلود کنید.

| فایل APK | معماری | مناسب برای |
|----------|--------|------------|
| `...-arm64-v8a.apk` | ARM 64-bit | اکثر گوشی‌های اندروید جدید |
| `...-armeabi-v7a.apk` | ARM 32-bit | گوشی‌های اندروید قدیمی‌تر |
| `...-universal.apk` | Universal | وقتی مطمئن نیستید |

---

## MasterDnsVPN چیست؟

[MasterDnsVPN](https://github.com/masterking32/MasterDnsVPN) یک تونل DNS با کارایی بالا است که ترافیک شما را رمزنگاری کرده و از طریق یک سرور راه دور ارسال می‌کند. این مخزن شامل **اپ اندروید** است که بر اساس همان موتور Go ساخته شده است.

---

## پیش‌نیازها

| ابزار | ورژن | لینک |
|-------|------|------|
| Go | 1.22+ | https://go.dev/dl |
| Android Studio | Ladybug+ | https://developer.android.com/studio |
| Android SDK | API 26+ (target 35) | از SDK Manager |
| Android NDK | 27.x | از SDK Manager |
| gomobile | latest | دستور زیر |

```powershell
go install golang.org/x/mobile/cmd/gomobile@latest
gomobile init
```

---

## بیلد گرفتن

### بیلد کامل (AAR + APK)
```powershell
.\build_aar.ps1
```

این اسکریپت:
1. NDK نصب‌شده را پیدا می‌کند
2. با `gomobile bind` موتور Go را به `android/app/libs/masterdnsvpn.aar` تبدیل می‌کند
3. با `gradlew assembleDebug` فایل APK نهایی را می‌سازد

### فایل‌های خروجی
| فایل | مسیر |
|------|------|
| کتابخانه Go | `android/app/libs/masterdnsvpn.aar` |
| Debug APK | `android/app/build/outputs/apk/debug/` |

### مراحل دستی
```powershell
# مرحله ۱ — بیلد AAR (فقط arm64)
$env:GOFLAGS = "-mod=mod"
$env:ANDROID_HOME = "$env:USERPROFILE\AppData\Local\Android\Sdk"
$ndkVer = (Get-ChildItem "$env:ANDROID_HOME\ndk" | Sort-Object Name -Descending | Select-Object -First 1).Name
$env:ANDROID_NDK_HOME = "$env:ANDROID_HOME\ndk\$ndkVer"
gomobile bind -target android/arm64 -androidapi 26 -javapkg com.masterdnsvpn.gomobile `
  -o "android\app\libs\masterdnsvpn.aar" masterdnsvpn-go/cmd/android

# مرحله ۲ — بیلد APK
cd android
.\gradlew.bat assembleDebug

# مرحله ۳ — نصب روی گوشی
adb install "app\build\outputs\apk\debug\MasterDnsVPN-1.0.0-beta-arm64-v8a.apk"
```

---

## قابلیت‌های اپ

### مدیریت پروفایل
- **ساخت / ویرایش / حذف** پروفایل — هر پروفایل به یک سرور MasterDnsVPN متصل می‌شود
- تمام فیلدهای تنظیمات کلاینت در UI (آدرس سرور، پورت، رمزنگاری، فشرده‌سازی، DNS، SOCKS5، حالت TUN، استراتژی Balancer، MTU و ...)
- **ویرایشگر Resolver** — اضافه/حذف DNS resolver به ازای هر پروفایل
- **حالت تونل**: SOCKS5 proxy یا TUN (VPN کل گوشی)

### متا پروفایل (Load Balancer)
- چند پروفایل را زیر یک کنترل Start/Stop گروه می‌کند
- ترافیک را طبق یکی از چهار استراتژی توزیع می‌کند:
  - **RoundRobin** — به نوبت
  - **Random** — تصادفی
  - **LeastConn** — کمترین اتصال
  - **LowestLatency** — کم‌ترین تأخیر
- پشتیبانی از حالت SOCKS5 و TUN

### حالت TUN (VPN کل گوشی)
- از `VpnService` اندروید برای رهگیری کل ترافیک گوشی استفاده می‌کند
- **پل tun2socks** (gVisor netstack): جریان‌های TCP/UDP را به SOCKS5 تبدیل کرده و از طریق موتور Go ارسال می‌کند
- فیلتر per-app: همه اپ‌ها / لیست Include / لیست Exclude

### اشتراک‌گذاری VPN روی Hotspot
- VPN را از طریق Wi-Fi hotspot به اشتراک بگذارید
- TCP relay روی `0.0.0.0:8090` → آدرس و پورت برای تنظیم روی دستگاه‌های متصل نمایش داده می‌شود

### مانیتورینگ
- آمار real-time هر پروفایل: وضعیت resolver، آدرس listen، حالت session
- CPU / RAM / سرعت آپلود / دانلود هر ثانیه به‌روز می‌شود
- حجم مصرفی تجمعی به ازای هر پروفایل (بعد از ری‌استارت هم ذخیره می‌ماند)
- لاگ‌ویور real-time با رنگ‌بندی بر اساس سطح

---

## معماری

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
                        │  موتور Go (masterdnsvpn.aar)     │
                        │  StartInstance / StopInstance    │
                        │  StartTunBridge (gVisor)         │
                        │  StartSocksBalancer (meta)       │
                        │  GetStats / GetBandwidth         │
                        └──────────────────────────────────┘
```

### جریان ترافیک TUN
```
اپ‌های گوشی → TUN fd → gVisor → SOCKS5 (Go) → تونل DNS → سرور MasterDnsVPN
```

### جریان ترافیک متا TUN
```
اپ‌های گوشی → TUN fd → gVisor → SOCKS Balancer ─┬─ پروفایل ۱ → سرور ۱
                                                 ├─ پروفایل ۲ → سرور ۲
                                                 └─ پروفایل N → سرور N
```

---

## حمایت از پروژه

اگر این پروژه برایتان مفید بوده، می‌توانید از توسعه آن حمایت کنید:

| شبکه | آدرس |
|------|------|
| TON | `UQBW_LoEhcYPIzZL_dzp-OMsqI5uAwv8p6dXy8wzzkPU-CQQ` |
| BNB / USDT (BEP-20) | `0x951acaf8d4b61a000d3b5c697abcabf52973d0cf` |
| TRX | `TL4Kej6DjJmT9gQ5ghmQcvsEUHPdnNNPyj` |
| SOL | `45kAfGyh13bcyYTdbNLkVfBGtMgq4WMijLgdBK9G9ugN` |

---

## لایسنس

فایل [LICENSE](LICENSE) را ببینید.

## پروژه اصلی

- **سرور و پروژه اصلی**: [github.com/masterking32/MasterDnsVPN](https://github.com/masterking32/MasterDnsVPN)
