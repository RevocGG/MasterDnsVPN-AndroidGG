# MasterDnsVPN ProGuard Rules

# ── gomobile generated AAR ───────────────────────────────────────────────────
# The gomobile AAR exposes the 'mobile' package (no -javapkg flag used).
# R8 obfuscation would rename these classes, breaking JNI FindClass() calls.
-keep class mobile.** { *; }
-keepclassmembers class mobile.** { *; }
-dontwarn mobile.**
# Go runtime internals
-keep class go.** { *; }
-dontwarn go.**
# Legacy name (keep for safety if any remnants remain)
-keep class masterdnsvpn.** { *; }
-dontwarn masterdnsvpn.**

# ── Hilt ─────────────────────────────────────────────────────────────────────
-keep class dagger.hilt.** { *; }
-keep class javax.inject.** { *; }
-keep @dagger.hilt.android.lifecycle.HiltViewModel class * { *; }

# ── Room ─────────────────────────────────────────────────────────────────────
-keep class * extends androidx.room.RoomDatabase { *; }
-keep @androidx.room.Entity class * { *; }
-keep @androidx.room.Dao interface * { *; }

# ── Kotlin coroutines ────────────────────────────────────────────────────────
-keepnames class kotlinx.coroutines.internal.MainDispatcherFactory {}
-keepnames class kotlinx.coroutines.CoroutineExceptionHandler {}
-keepclassmembernames class kotlinx.** { volatile <fields>; }

# ── Kotlin serialization ────────────────────────────────────────────────────
-keepattributes *Annotation*, InnerClasses
-dontnote kotlinx.serialization.**
-keepclassmembers class kotlinx.serialization.json.** { *** Companion; }

# ── VPN service ──────────────────────────────────────────────────────────────
-keep class com.masterdnsvpn.service.** { *; }
-keep class com.masterdnsvpn.bridge.** { *; }

# ── General Android ──────────────────────────────────────────────────────────
-keepattributes SourceFile,LineNumberTable
-keepattributes Signature
-keepattributes Exceptions
-keep public class * extends android.app.Activity
-keep public class * extends android.app.Service
-keep public class * extends android.content.BroadcastReceiver
