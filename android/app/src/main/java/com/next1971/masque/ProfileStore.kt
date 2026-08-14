package com.next1971.masque

import android.content.Context
import android.util.Log
import java.io.File

/**
 * ProfileStore — parses the MASQUE profile and writes certificates to internal
 * app storage.
 *
 * Profile format (.masque / .toml, self-contained, with inline PEM):
 *
 *   [server]
 *   address = "YOUR_SERVER_HOST:4433"
 *   name    = "YOUR_SERVER_HOST"
 *
 *   [tun]
 *   dns = "1.1.1.1"
 *
 *   [tls]
 *   ca = """
 *   -----BEGIN CERTIFICATE-----
 *   ...
 *   -----END CERTIFICATE-----
 *   """
 *   cert = """
 *   -----BEGIN CERTIFICATE-----
 *   ...
 *   -----END CERTIFICATE-----
 *   """
 *   key = """
 *   -----BEGIN EC PRIVATE KEY-----
 *   ...
 *   -----END EC PRIVATE KEY-----
 *   """
 *
 * Kotlin parses this file (a minimal TOML parser for only the required keys and
 * triple quotes), writes ca/cert/key to files/certs/, and passes the paths to Go.
 * This lets the core, which reads certificates from paths, work unchanged.
 */
data class Profile(
    val server: String,
    val serverName: String,
    val dns: String,
    val caPath: String,
    val certPath: String,
    val keyPath: String,
)

object ProfileStore {
    private const val TAG = "MasqueProfile"
    private const val PREFS = "masque"
    private const val KEY_CONFIGURED = "configured"

    fun isConfigured(ctx: Context): Boolean =
        ctx.getSharedPreferences(PREFS, Context.MODE_PRIVATE).getBoolean(KEY_CONFIGURED, false) &&
            File(certsDir(ctx), "server.txt").exists()

    private fun certsDir(ctx: Context): File =
        File(ctx.filesDir, "certs").apply { mkdirs() }

    /** Imports a profile from file content, writes certificates, and saves metadata. */
    fun import(ctx: Context, content: String): Result<Unit> {
        return try {
            val server = extractValue(content, "address")
                ?: extractValue(content, "server")
                ?: return Result.failure(IllegalArgumentException("missing server.address"))
            val name = extractValue(content, "name") ?: server.substringBefore(":")
            val dns = extractValue(content, "dns") ?: "1.1.1.1"
            val ca = extractBlock(content, "ca") ?: return Result.failure(IllegalArgumentException("missing tls.ca"))
            val cert = extractBlock(content, "cert") ?: return Result.failure(IllegalArgumentException("missing tls.cert"))
            val key = extractBlock(content, "key") ?: return Result.failure(IllegalArgumentException("missing tls.key"))

            val dir = certsDir(ctx)
            File(dir, "ca.crt").writeText(ca.trim() + "\n")
            File(dir, "client.crt").writeText(cert.trim() + "\n")
            File(dir, "client.key").writeText(key.trim() + "\n")
            File(dir, "server.txt").writeText("$server\n$name\n$dns\n")

            ctx.getSharedPreferences(PREFS, Context.MODE_PRIVATE).edit()
                .putBoolean(KEY_CONFIGURED, true).apply()
            Log.i(TAG, "profile imported: server=$server name=$name dns=$dns")
            Result.success(Unit)
        } catch (e: Exception) {
            Log.e(TAG, "import failed", e)
            Result.failure(e)
        }
    }

    fun load(ctx: Context): Profile? {
        val dir = certsDir(ctx)
        val meta = File(dir, "server.txt")
        if (!meta.exists()) return null
        val lines = meta.readLines()
        if (lines.size < 3) return null
        return Profile(
            server = lines[0].trim(),
            serverName = lines[1].trim(),
            dns = lines[2].trim(),
            caPath = File(dir, "ca.crt").absolutePath,
            certPath = File(dir, "client.crt").absolutePath,
            keyPath = File(dir, "client.key").absolutePath,
        )
    }

    // --- minimal parsing of the required keys ---

    /** Simple key = "value" entry (in any section). */
    private fun extractValue(content: String, key: String): String? {
        val re = Regex("""(?m)^\s*${Regex.escape(key)}\s*=\s*"([^"]*)"""")
        return re.find(content)?.groupValues?.get(1)
    }

    /** Block key = """ ... """ entry (triple-quoted, multiline). */
    private fun extractBlock(content: String, key: String): String? {
        val re = Regex("""(?s)\b${Regex.escape(key)}\s*=\s*"{3}(.*?)"{3}""")
        return re.find(content)?.groupValues?.get(1)
    }
}
