package com.next1971.masque

import android.content.Context
import android.util.Log
import java.io.File
import java.util.Calendar
import java.util.Locale

/**
 * ProfileStore — parses the MASQUE profile and writes certificates to internal
 * app storage.
 *
 * Profile format (.masque / .toml, self-contained, with inline PEM):
 *
 *   [server]
 *   address  = "YOUR_SERVER_HOST:4433"
 *   name     = "YOUR_SERVER_HOST"
 *   alt_port = 2053   # optional second UDP port; omit for a single port
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
    val altPort: Int,
    val caPath: String,
    val certPath: String,
    val keyPath: String,
)

object ProfileStore {
    private const val TAG = "MasqueProfile"
    private const val PREFS = "masque"
    private const val KEY_CONFIGURED = "configured"
    private const val KEY_KILL_SWITCH = "kill_switch"
    private const val KEY_TRANSPORT = "transport"
    private const val KEY_MTU = "mtu"
    private const val KEY_TR_DAY = "traffic_day"
    private const val KEY_TR_MONTH = "traffic_month"
    private const val KEY_TR_DAY_BYTES = "traffic_day_bytes"
    private const val KEY_TR_MONTH_BYTES = "traffic_month_bytes"
    private const val KEY_TR_APPLIED = "traffic_applied"

    /** Block internet while the VPN session is up but the tunnel is down. Default off. */
    fun killSwitch(ctx: Context): Boolean =
        ctx.getSharedPreferences(PREFS, Context.MODE_PRIVATE).getBoolean(KEY_KILL_SWITCH, false)

    fun setKillSwitch(ctx: Context, enabled: Boolean) {
        ctx.getSharedPreferences(PREFS, Context.MODE_PRIVATE).edit()
            .putBoolean(KEY_KILL_SWITCH, enabled).apply()
    }

    /** "auto", "udp", or "tcp". Default is automatic UDP then TCP. */
    fun transport(ctx: Context): String {
        val v = ctx.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
            .getString(KEY_TRANSPORT, "auto") ?: "auto"
        return if (v == "udp" || v == "tcp") v else "auto"
    }

    fun setTransport(ctx: Context, mode: String) {
        val saved = if (mode == "udp" || mode == "tcp") mode else "auto"
        ctx.getSharedPreferences(PREFS, Context.MODE_PRIVATE).edit()
            .putString(KEY_TRANSPORT, saved).apply()
    }

    /** Tunnel MTU. Default 1340, clamped to 1280–1500. */
    fun mtu(ctx: Context): Int {
        val v = ctx.getSharedPreferences(PREFS, Context.MODE_PRIVATE).getInt(KEY_MTU, 1340)
        return if (v in 1280..1500) v else 1340
    }

    fun setMTU(ctx: Context, value: Int) {
        val saved = if (value in 1280..1500) value else 1340
        ctx.getSharedPreferences(PREFS, Context.MODE_PRIVATE).edit()
            .putInt(KEY_MTU, saved).apply()
    }

    /**
     * Folds a monotonic pump total into the local day and month.
     * A smaller total means the process restarted; only the new bytes are added.
     */
    fun noteTraffic(ctx: Context, total: Long) {
        val prefs = ctx.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
        val day = dayKey()
        val month = monthKey()
        var dayBytes = prefs.getLong(KEY_TR_DAY_BYTES, 0)
        var monthBytes = prefs.getLong(KEY_TR_MONTH_BYTES, 0)
        val applied = prefs.getLong(KEY_TR_APPLIED, 0)
        if (prefs.getString(KEY_TR_DAY, "") != day) dayBytes = 0
        if (prefs.getString(KEY_TR_MONTH, "") != month) monthBytes = 0
        var delta = total - applied
        if (total < applied) delta = total
        if (delta < 0) delta = 0
        dayBytes += delta
        monthBytes += delta
        prefs.edit()
            .putString(KEY_TR_DAY, day)
            .putString(KEY_TR_MONTH, month)
            .putLong(KEY_TR_DAY_BYTES, dayBytes)
            .putLong(KEY_TR_MONTH_BYTES, monthBytes)
            .putLong(KEY_TR_APPLIED, total)
            .apply()
    }

    fun trafficLabel(ctx: Context): String {
        val prefs = ctx.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
        val dayBytes = if (prefs.getString(KEY_TR_DAY, "") == dayKey()) {
            prefs.getLong(KEY_TR_DAY_BYTES, 0)
        } else {
            0L
        }
        val monthBytes = if (prefs.getString(KEY_TR_MONTH, "") == monthKey()) {
            prefs.getLong(KEY_TR_MONTH_BYTES, 0)
        } else {
            0L
        }
        return "Today: ${formatMB(dayBytes)}\nMonth: ${formatMB(monthBytes)}"
    }

    private fun formatMB(bytes: Long): String =
        String.format(Locale.US, "%.1f MB", bytes / 1_000_000.0)

    private fun dayKey(): String {
        val cal = Calendar.getInstance()
        return "%04d-%02d-%02d".format(
            cal.get(Calendar.YEAR),
            cal.get(Calendar.MONTH) + 1,
            cal.get(Calendar.DAY_OF_MONTH),
        )
    }

    private fun monthKey(): String {
        val cal = Calendar.getInstance()
        return "%04d-%02d".format(cal.get(Calendar.YEAR), cal.get(Calendar.MONTH) + 1)
    }

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
            val altPort = extractPort(content, "alt_port")
            val ca = extractBlock(content, "ca") ?: return Result.failure(IllegalArgumentException("missing tls.ca"))
            val cert = extractBlock(content, "cert") ?: return Result.failure(IllegalArgumentException("missing tls.cert"))
            val key = extractBlock(content, "key") ?: return Result.failure(IllegalArgumentException("missing tls.key"))

            val dir = certsDir(ctx)
            File(dir, "ca.crt").writeText(ca.trim() + "\n")
            File(dir, "client.crt").writeText(cert.trim() + "\n")
            File(dir, "client.key").writeText(key.trim() + "\n")
            File(dir, "server.txt").writeText("$server\n$name\n$dns\n$altPort\n")

            ctx.getSharedPreferences(PREFS, Context.MODE_PRIVATE).edit()
                .putBoolean(KEY_CONFIGURED, true).apply()
            Log.i(TAG, "profile imported: server=$server name=$name dns=$dns alt_port=$altPort")
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
        val alt = if (lines.size >= 4) lines[3].trim().toIntOrNull() ?: 0 else 0
        return Profile(
            server = lines[0].trim(),
            serverName = lines[1].trim(),
            dns = lines[2].trim(),
            altPort = alt,
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

    /** Integer key = 2053 or key = "2053". Zero / missing means single-port. */
    private fun extractPort(content: String, key: String): Int {
        extractValue(content, key)?.toIntOrNull()?.let { return it }
        val re = Regex("""(?m)^\s*${Regex.escape(key)}\s*=\s*(\d+)""")
        return re.find(content)?.groupValues?.get(1)?.toIntOrNull() ?: 0
    }

    /** Block key = """ ... """ entry (triple-quoted, multiline). */
    private fun extractBlock(content: String, key: String): String? {
        val re = Regex("""(?s)\b${Regex.escape(key)}\s*=\s*"{3}(.*?)"{3}""")
        return re.find(content)?.groupValues?.get(1)
    }
}
