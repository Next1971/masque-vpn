package com.next1971.masque

import android.content.Context
import android.content.pm.PackageManager
import android.os.Build

/** Human-readable install label, e.g. "v1.4.1 (16)". */
fun Context.appVersionLabel(): String {
    return try {
        val pinfo = if (Build.VERSION.SDK_INT >= 33) {
            packageManager.getPackageInfo(packageName, PackageManager.PackageInfoFlags.of(0))
        } else {
            @Suppress("DEPRECATION")
            packageManager.getPackageInfo(packageName, 0)
        }
        val name = pinfo.versionName ?: "?"
        val code = if (Build.VERSION.SDK_INT >= 28) {
            pinfo.longVersionCode
        } else {
            @Suppress("DEPRECATION")
            pinfo.versionCode.toLong()
        }
        "v$name ($code)"
    } catch (_: Exception) {
        "v?"
    }
}
