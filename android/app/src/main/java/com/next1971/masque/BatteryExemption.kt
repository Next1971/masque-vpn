package com.next1971.masque

import android.content.Context
import android.content.Intent
import android.net.Uri
import android.os.Build
import android.os.PowerManager
import android.provider.Settings

/**
 * Asks the system to exclude this app from battery optimization so QUIC
 * keepalives can still run with the screen off. On some OEMs the dialog is
 * hidden or ignored; reconnect on wake is a separate follow-up.
 */
object BatteryExemption {
    fun intentIfNeeded(context: Context): Intent? {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.M) return null
        val pm = context.getSystemService(PowerManager::class.java) ?: return null
        if (pm.isIgnoringBatteryOptimizations(context.packageName)) return null
        return Intent(Settings.ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS).apply {
            data = Uri.parse("package:${context.packageName}")
        }
    }
}
