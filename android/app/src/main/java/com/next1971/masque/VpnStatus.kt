package com.next1971.masque

object VpnStatus {
    fun applyConnected(previous: Boolean, msg: String): Boolean {
        val m = msg.lowercase()
        return when {
            m == "disconnected" -> false
            m.startsWith("error:") -> false
            m == "connected" || m == "vpn active" -> true
            m.startsWith("reconnect") -> true
            m == "forwarding started" -> true
            else -> previous
        }
    }
}
