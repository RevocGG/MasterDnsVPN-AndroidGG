package com.masterdnsvpn.security

import android.annotation.SuppressLint
import android.content.Context
import android.content.pm.PackageManager
import android.os.Build
import java.io.File
import java.security.MessageDigest

/**
 * Verifies that a downloaded APK is signed with the same developer key as the
 * currently installed application.
 *
 * SECURITY RATIONALE
 * ──────────────────
 * The private signing key never leaves the developer's machine. Even if the
 * GitHub account is fully compromised or a MITM attack replaces the CDN response,
 * the attacker cannot produce a validly signed APK without the key. This check
 * makes those attack vectors useless.
 *
 * WHAT IS CHECKED
 * ───────────────
 * 1. The APK is parseable and contains at least one signing certificate.
 * 2. The declared package name in the APK equals the currently installed
 *    app's package name (blocks "update" to a completely different app).
 * 3. Every SHA-256 fingerprint from the installed app's signing certificate(s)
 *    is also present in the downloaded APK — proving common authorship.
 *
 * NOTE ON API < 28 (GET_SIGNATURES)
 * ──────────────────────────────────
 * The "Janus" APK injection attack only affected *verifying trust* based on a
 * signature alone. Here we are comparing two apps' certificate fingerprints
 * against each other (not against a CA), so the comparison is still sound on
 * older APIs. Nevertheless, API 28+ code path is preferred and used when
 * available.
 */
object ApkSignatureVerifier {

    /**
     * Returns `true` only when the downloaded [apkFile] passes all checks.
     * Returns `false` (never throws) on any parse error or mismatch.
     *
     * Must be called from a background thread (parsing an APK is blocking).
     */
    @SuppressLint("PackageManagerGetSignatures") // safe — handled with Build.VERSION check
    fun verify(ctx: Context, apkFile: File): Boolean {
        if (!apkFile.exists() || apkFile.length() == 0L) return false

        val pm = ctx.applicationContext.packageManager

        return try {
            // ── 1. Collect fingerprints of the currently installed app ────────────
            val installedFingerprints: Set<String> =
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
                    val info = pm.getPackageInfo(
                        ctx.packageName,
                        PackageManager.GET_SIGNING_CERTIFICATES,
                    )
                    info.signingInfo
                        ?.apkContentsSigners
                        ?.map { sha256(it.toByteArray()) }
                        ?.toSet()
                        ?: return false
                } else {
                    @Suppress("DEPRECATION")
                    val info = pm.getPackageInfo(
                        ctx.packageName,
                        PackageManager.GET_SIGNATURES,
                    )
                    @Suppress("DEPRECATION")
                    info.signatures
                        ?.map { sha256(it.toByteArray()) }
                        ?.toSet()
                        ?: return false
                }

            if (installedFingerprints.isEmpty()) return false

            // ── 2. Parse the downloaded APK ───────────────────────────────────────
            val (apkPackage, apkFingerprints) =
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
                    val info = pm.getPackageArchiveInfo(
                        apkFile.absolutePath,
                        PackageManager.GET_SIGNING_CERTIFICATES,
                    ) ?: return false
                    val fps = info.signingInfo
                        ?.apkContentsSigners
                        ?.map { sha256(it.toByteArray()) }
                        ?.toSet()
                        ?: return false
                    (info.packageName ?: return false) to fps
                } else {
                    @Suppress("DEPRECATION")
                    val info = pm.getPackageArchiveInfo(
                        apkFile.absolutePath,
                        PackageManager.GET_SIGNATURES,
                    ) ?: return false
                    @Suppress("DEPRECATION")
                    val fps = info.signatures
                        ?.map { sha256(it.toByteArray()) }
                        ?.toSet()
                        ?: return false
                    (info.packageName ?: return false) to fps
                }

            // ── 3. Package name must match exactly ────────────────────────────────
            if (apkPackage != ctx.packageName) return false

            // ── 4. Every installed cert fingerprint must be present in the APK ───
            installedFingerprints.all { it in apkFingerprints }

        } catch (_: Exception) {
            // Any parse error (broken APK, truncated download, etc.) = reject
            false
        }
    }

    /** Hex-encoded SHA-256 digest of [bytes]. */
    private fun sha256(bytes: ByteArray): String {
        val md = MessageDigest.getInstance("SHA-256")
        return md.digest(bytes).joinToString("") { "%02X".format(it) }
    }
}
