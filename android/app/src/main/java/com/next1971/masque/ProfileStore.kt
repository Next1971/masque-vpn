package com.next1971.masque

import android.content.Context
import android.util.Log
import java.io.File
import java.security.MessageDigest
import java.util.Calendar
import java.util.Locale

/**
 * ProfileStore — parses the MASQUE profile and writes certificates to internal
 * app storage. Several profiles can be stored at once; one of them is the
 * active profile that Connect uses.
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
 * triple quotes), writes ca/cert/key into files/profiles/<id>/, and passes the
 * paths to Go. This lets the core, which reads certificates from paths, work
 * unchanged. files/profiles/active_id in SharedPreferences names the active
 * profile. Transport, MTU, kill switch, and traffic counters are per-device
 * settings and stay global.
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

/** One stored profile, for pickers. */
data class ProfileInfo(
    val id: String,
    val name: String,
    val server: String,
    val isActive: Boolean,
)

object ProfileStore {
    private const val TAG = "MasqueProfile"
    private const val PREFS = "masque"
    private const val KEY_ACTIVE = "active_id"
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

    // --- stored profiles ---

    fun isConfigured(ctx: Context): Boolean = activeId(ctx) != null

    /** Display name of the active profile, or "" when none is stored. */
    fun activeName(ctx: Context): String = load(ctx)?.serverName ?: ""

    private fun prefs(ctx: Context) = ctx.getSharedPreferences(PREFS, Context.MODE_PRIVATE)

    private fun profilesDir(ctx: Context): File = File(ctx.filesDir, "profiles").apply { mkdirs() }

    private fun profileDir(ctx: Context, id: String): File = File(profilesDir(ctx), id)

    /** Profile ids sorted by name; a directory counts only with server.txt inside. */
    private fun storedIds(ctx: Context): List<String> =
        profilesDir(ctx).listFiles { f -> f.isDirectory && File(f, "server.txt").exists() }
            ?.map { it.name }
            ?.sorted()
            ?: emptyList()

    private fun activeId(ctx: Context): String? {
        migrate(ctx)
        val ids = storedIds(ctx)
        if (ids.isEmpty()) return null
        val saved = prefs(ctx).getString(KEY_ACTIVE, null)
        if (saved != null && ids.contains(saved)) return saved
        // Stale pointer (deleted profile): fall back to the first one.
        prefs(ctx).edit().putString(KEY_ACTIVE, ids.first()).apply()
        return ids.first()
    }

    /** Switches the active profile. Returns false when the id is not stored. */
    fun setActive(ctx: Context, id: String): Boolean {
        migrate(ctx)
        if (!File(profileDir(ctx, id), "server.txt").exists()) return false
        prefs(ctx).edit().putString(KEY_ACTIVE, id).apply()
        return true
    }

    /** Removes one stored profile. Deleting the active one reactivates the first remaining. */
    fun delete(ctx: Context, id: String): Boolean {
        migrate(ctx)
        val dir = profileDir(ctx, id)
        if (!File(dir, "server.txt").exists()) return false
        val wasActive = prefs(ctx).getString(KEY_ACTIVE, null) == id
        if (!dir.deleteRecursively()) return false
        if (wasActive) {
            val ids = storedIds(ctx)
            if (ids.isNotEmpty()) {
                prefs(ctx).edit().putString(KEY_ACTIVE, ids.first()).apply()
            } else {
                prefs(ctx).edit().remove(KEY_ACTIVE).apply()
            }
        }
        return true
    }

    fun list(ctx: Context): List<ProfileInfo> {
        migrate(ctx)
        val active = prefs(ctx).getString(KEY_ACTIVE, null)
        return storedIds(ctx).map { id ->
            val lines = runCatching { File(profileDir(ctx, id), "server.txt").readLines() }
                .getOrDefault(emptyList())
            ProfileInfo(
                id = id,
                name = lines.getOrNull(1)?.trim()?.ifEmpty { null } ?: "(invalid)",
                server = lines.getOrNull(0)?.trim() ?: "",
                isActive = id == active,
            )
        }.sortedBy { it.name }
    }

    /** The id derives from the server name plus the client certificate hash, so
     *  re-importing the same bundle updates that entry instead of duplicating
     *  it, and two devices on one server stay separate profiles. */
    fun profileId(name: String, certPem: String): String {
        val digest = MessageDigest.getInstance("SHA-256").digest(certPem.trim().toByteArray())
        val hash = digest.joinToString("") { "%02x".format(it) }.take(8)
        return slugify(name) + "-" + hash
    }

    private fun slugify(s: String): String {
        val sb = StringBuilder()
        for (ch in s.trim().lowercase()) {
            sb.append(if (ch in 'a'..'z' || ch in '0'..'9') ch else '-')
        }
        var out = sb.toString().trim('-')
        if (out.length > 40) out = out.take(40).trim('-')
        return out.ifEmpty { "profile" }
    }

    /** Moves a pre-2.2 single-profile install (files/certs/) into
     *  files/profiles/<id>/ and activates it. The legacy server.txt is removed
     *  after a successful move, so a rerun finds nothing to migrate. */
    fun migrate(ctx: Context) {
        val legacyDir = File(ctx.filesDir, "certs")
        val legacy = File(legacyDir, "server.txt")
        if (!legacy.exists()) return
        runCatching {
            val lines = legacy.readLines()
            if (lines.size < 3) return
            val server = lines[0].trim()
            val name = lines[1].trim()
            val dns = lines[2].trim()
            val alt = if (lines.size >= 4) lines[3].trim().toIntOrNull() ?: 0 else 0
            val cert = File(legacyDir, "client.crt").readText()
            val id = profileId(name, cert)
            val dir = profileDir(ctx, id)
            dir.mkdirs()
            File(legacyDir, "ca.crt").copyTo(File(dir, "ca.crt"), overwrite = true)
            File(legacyDir, "client.crt").copyTo(File(dir, "client.crt"), overwrite = true)
            File(legacyDir, "client.key").copyTo(File(dir, "client.key"), overwrite = true)
            File(dir, "server.txt").writeText("$server\n$name\n$dns\n$alt\n")
            prefs(ctx).edit().putString(KEY_ACTIVE, id).apply()
            legacy.delete()
            File(legacyDir, "ca.crt").delete()
            File(legacyDir, "client.crt").delete()
            File(legacyDir, "client.key").delete()
            Log.i(TAG, "migrated legacy profile to $id")
        }.onFailure { Log.e(TAG, "migration failed", it) }
    }

    /** Imports a profile from file content, writes certificates under its id,
     *  and makes it the active profile. */
    fun import(ctx: Context, content: String): Result<Unit> {
        return try {
            migrate(ctx)
            val server = extractValue(content, "address")
                ?: extractValue(content, "server")
                ?: return Result.failure(IllegalArgumentException("missing server.address"))
            val name = extractValue(content, "name") ?: server.substringBefore(":")
            val dns = extractValue(content, "dns") ?: "1.1.1.1"
            val altPort = extractPort(content, "alt_port")
            val ca = extractBlock(content, "ca") ?: return Result.failure(IllegalArgumentException("missing tls.ca"))
            val cert = extractBlock(content, "cert") ?: return Result.failure(IllegalArgumentException("missing tls.cert"))
            val key = extractBlock(content, "key") ?: return Result.failure(IllegalArgumentException("missing tls.key"))

            val dir = profileDir(ctx, profileId(name, cert))
            dir.mkdirs()
            File(dir, "ca.crt").writeText(ca.trim() + "\n")
            File(dir, "client.crt").writeText(cert.trim() + "\n")
            File(dir, "client.key").writeText(key.trim() + "\n")
            File(dir, "server.txt").writeText("$server\n$name\n$dns\n$altPort\n")
            prefs(ctx).edit().putString(KEY_ACTIVE, dir.name).apply()

            Log.i(TAG, "profile imported: server=$server name=$name dns=$dns alt_port=$altPort")
            Result.success(Unit)
        } catch (e: Exception) {
            Log.e(TAG, "import failed", e)
            Result.failure(e)
        }
    }

    /** Loads the active profile, or null when none is stored. */
    fun load(ctx: Context): Profile? {
        val id = activeId(ctx) ?: return null
        val dir = profileDir(ctx, id)
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
