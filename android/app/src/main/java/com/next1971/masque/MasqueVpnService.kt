package com.next1971.masque

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Intent
import android.net.VpnService
import android.os.Build
import android.os.ParcelFileDescriptor
import android.util.Log
import mobile.Callback
import mobile.Config
import mobile.Mobile
import mobile.Tunnel

/**
 * MasqueVpnService — Android VpnService wrapping the shared Go core (clientcore)
 * through the gomobile bridge (the `mobile` package in masque.aar).
 *
 * Flow:
 *  1. Create TUN through VpnService.Builder (address/routes/DNS are configured by Android).
 *  2. Obtain the interface file descriptor and pass it to Go (Mobile.connect(cfg, fd, cb)).
 *  3. Go forwards traffic between the fd and the QUIC/CONNECT-IP tunnel.
 *
 * Builder configures routes/address/DNS; Go does NOT change them (unlike the
 * Windows/Linux wrappers), keeping the bridge clean and portable.
 */
class MasqueVpnService : VpnService() {

    companion object {
        const val TAG = "MasqueVpn"
        const val ACTION_CONNECT = "com.next1971.masque.CONNECT"
        const val ACTION_DISCONNECT = "com.next1971.masque.DISCONNECT"
        const val CHANNEL_ID = "masque_vpn"
        const val NOTIF_ID = 1

        // Client tunnel address. The server assigns it from the 10.8.0.0/24 pool.
        // Builder requires an address BEFORE establish(); use the known pool
        // approach: set a temporary /32 and default route. The actual address
        // comes from the server and could be reset if needed, but for
        // one client the address is predictable. Use a broad /24 here to
        // avoid depending on the exact .254 (the server routes it regardless).
        const val TUN_ADDR_FALLBACK = "10.8.0.254"
        const val TUN_PREFIX = 24
        const val TUN_MTU = 1400
    }

    private var tunnel: Tunnel? = null
    private var pfd: ParcelFileDescriptor? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_DISCONNECT -> {
                stopVpn()
                return START_NOT_STICKY
            }
            else -> startVpn()
        }
        return START_STICKY
    }

    private fun startVpn() {
        val prof = ProfileStore.load(this)
        if (prof == null) {
            Log.e(TAG, "no profile configured")
            broadcast("Error: profile not configured")
            stopSelf()
            return
        }

        startForeground(NOTIF_ID, buildNotification("Connecting…"))

        // 1. Create the TUN interface. Android configures address/routes/DNS.
        val builder = Builder()
            .setSession("MASQUE")
            .setMtu(TUN_MTU)
            .addAddress(TUN_ADDR_FALLBACK, TUN_PREFIX)
            .addRoute("0.0.0.0", 0)          // all traffic through the tunnel (full-route)
            .addDnsServer(prof.dns)          // DNS from profile (1.1.1.1 by default)

        // Exclude this app from the VPN so QUIC packets to the server do not loop.
        try {
            builder.addDisallowedApplication(packageName)
        } catch (e: Exception) {
            Log.w(TAG, "addDisallowedApplication: ${e.message}")
        }

        val iface = builder.establish()
        if (iface == null) {
            Log.e(TAG, "establish() returned null (VPN permission?)")
            broadcast("Error: VPN permission unavailable")
            stopSelf()
            return
        }
        pfd = iface

        // 2. Prepare the Go config: certificate paths in internal storage.
        val cfg = Config().apply {
            server = prof.server
            serverName = prof.serverName
            caPath = prof.caPath
            certPath = prof.certPath
            keyPath = prof.keyPath
            mtu = TUN_MTU.toLong()
        }

        val cb = object : Callback {
            override fun onStatus(msg: String?) {
                Log.i(TAG, "status: $msg")
                broadcast(msg ?: "")
                updateNotification(msg ?: "Connected")
            }
            override fun onError(msg: String?) {
                Log.e(TAG, "error: $msg")
                broadcast("Error: $msg")
                stopVpn()
            }
        }

        // 3. Start the Go core with the interface fd.
        try {
            val fd = iface.fd
            tunnel = Mobile.connect(cfg, fd.toLong(), cb)
            broadcast("Connected")
            updateNotification("VPN active")
        } catch (e: Exception) {
            Log.e(TAG, "connect failed", e)
            broadcast("Connection error: ${e.message}")
            stopVpn()
        }
    }

    private fun stopVpn() {
        try {
            tunnel?.stop()
        } catch (e: Exception) {
            Log.w(TAG, "tunnel.stop: ${e.message}")
        }
        tunnel = null
        try {
            pfd?.close()
        } catch (_: Exception) {
        }
        pfd = null
        broadcast("Disconnected")
        stopForeground(STOP_FOREGROUND_REMOVE)
        stopSelf()
    }

    override fun onDestroy() {
        stopVpn()
        super.onDestroy()
    }

    private fun broadcast(msg: String) {
        sendBroadcast(Intent("com.next1971.masque.STATUS").putExtra("msg", msg).setPackage(packageName))
    }

    // --- notification (required for the foreground service) ---

    private fun ensureChannel() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val nm = getSystemService(NotificationManager::class.java)
            if (nm.getNotificationChannel(CHANNEL_ID) == null) {
                nm.createNotificationChannel(
                    NotificationChannel(CHANNEL_ID, "MASQUE VPN", NotificationManager.IMPORTANCE_LOW)
                )
            }
        }
    }

    private fun buildNotification(text: String): Notification {
        ensureChannel()
        val pi = PendingIntent.getActivity(
            this, 0, Intent(this, MainActivity::class.java),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )
        return Notification.Builder(this, CHANNEL_ID)
            .setContentTitle("MASQUE VPN")
            .setContentText(text)
            .setSmallIcon(android.R.drawable.ic_lock_lock)
            .setContentIntent(pi)
            .setOngoing(true)
            .build()
    }

    private fun updateNotification(text: String) {
        val nm = getSystemService(NotificationManager::class.java)
        nm.notify(NOTIF_ID, buildNotification(text))
    }
}
