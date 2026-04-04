package com.masterdnsvpn.profile

import android.util.Base64
import javax.crypto.Cipher
import javax.crypto.spec.GCMParameterSpec
import javax.crypto.spec.SecretKeySpec
import java.security.SecureRandom

/**
 * Encrypts / decrypts identity fields (domains, encryption key) for
 * locked profile export.  Uses AES-256-GCM with a random 12-byte IV
 * prepended to the ciphertext.
 *
 * The key is derived from a constant embedded in the app.  This is NOT
 * meant to protect against a determined reverse-engineer — its purpose
 * is to prevent casual users from reading the values in a text editor
 * or re-sharing them.  True DRM is impossible on an open system.
 */
object IdentityCipher {

    private const val PREFIX = "ENC:"
    private const val GCM_TAG_BITS = 128
    private const val IV_BYTES = 12

    // 32-byte key (AES-256).  Split across expressions to hinder grep.
    private val KEY_BYTES: ByteArray by lazy {
        val a = byteArrayOf(
            0x4D, 0x61, 0x73, 0x74, 0x65, 0x72, 0x44, 0x6E,
            0x73, 0x56, 0x50, 0x4E, 0x2D, 0x47, 0x47, 0x2D,
        )
        val b = byteArrayOf(
            0x4C, 0x6F, 0x63, 0x6B, 0x65, 0x64, 0x49, 0x64,
            0x65, 0x6E, 0x74, 0x69, 0x74, 0x79, 0x4B, 0x31,
        )
        a + b
    }

    private fun keySpec() = SecretKeySpec(KEY_BYTES, "AES")

    /** Encrypt [plaintext] → `"ENC:<base64(iv+ciphertext)>"` */
    fun encrypt(plaintext: String): String {
        val iv = ByteArray(IV_BYTES).also { SecureRandom().nextBytes(it) }
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.ENCRYPT_MODE, keySpec(), GCMParameterSpec(GCM_TAG_BITS, iv))
        val ct = cipher.doFinal(plaintext.toByteArray(Charsets.UTF_8))
        val combined = iv + ct
        return PREFIX + Base64.encodeToString(combined, Base64.NO_WRAP)
    }

    /** Decrypt a value previously produced by [encrypt].  Returns null on failure. */
    fun decrypt(encoded: String): String? {
        if (!encoded.startsWith(PREFIX)) return null
        return try {
            val combined = Base64.decode(encoded.removePrefix(PREFIX), Base64.NO_WRAP)
            if (combined.size < IV_BYTES + 1) return null
            val iv = combined.copyOfRange(0, IV_BYTES)
            val ct = combined.copyOfRange(IV_BYTES, combined.size)
            val cipher = Cipher.getInstance("AES/GCM/NoPadding")
            cipher.init(Cipher.DECRYPT_MODE, keySpec(), GCMParameterSpec(GCM_TAG_BITS, iv))
            String(cipher.doFinal(ct), Charsets.UTF_8)
        } catch (_: Exception) {
            null
        }
    }

    /** Returns true when the value looks like an encrypted blob. */
    fun isEncrypted(value: String): Boolean = value.startsWith(PREFIX)
}
