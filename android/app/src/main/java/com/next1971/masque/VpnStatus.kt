package com.next1971.masque

object VpnStatus {
    fun applyConnected(previous: Boolean, msg: String): Boolean {
        val m = msg.lowercase()
        return when {
            m == "disconnected" -> false
            m.startsWith("error:") -> false
            m == "connected" || m.startsWith("connected ") || m.startsWith("vpn active") -> true
            m.startsWith("reconnect") -> true
            m == "forwarding started" -> true
            else -> previous
        }
    }

    fun pingLabel(msg: String): String? {
        val match = Regex("""ping (\d+) ms""", RegexOption.IGNORE_CASE).find(msg)
        return match?.let { "Ping: ${it.groupValues[1]} ms" }
    }

    fun statusLabel(msg: String): String {
        val cleaned = msg.replace(Regex("""\s*·\s*ping \d+ ms""", RegexOption.IGNORE_CASE), "")
        return "Status: $cleaned"
    }
}
