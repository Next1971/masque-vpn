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

        // Fallback tunnel address, used ONLY if the server does not report an
        // assigned address (should not happen). The real address is obtained
        // from the server via a two-phase connect (Dial → read assigned addr →
        // build TUN with THAT address → establish → StartWithFD). This is
        // essential for multiple devices: each client gets a UNIQUE address
        // from the 10.8.0.0/24 pool, so return traffic is demultiplexed to the
        // correct device. Hardcoding one address broke concurrent clients.
        const val TUN_ADDR_FALLBACK = "10.8.0.254"
        const val TUN_PREFIX_FALLBACK = 32
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

        // Prepare the Go config: certificate paths in internal storage.
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

        try {
            // PHASE 1: establish the CONNECT-IP session WITHOUT a TUN yet, so we
            // learn the address the server assigned to THIS client.
            val t = Mobile.dial(cfg, cb)
            tunnel = t

            var addr = t.assignedAddr()
            var prefix = t.assignedPrefixLen().toInt()
            if (addr.isNullOrEmpty()) {
                Log.w(TAG, "server assigned no address; using fallback $TUN_ADDR_FALLBACK/$TUN_PREFIX_FALLBACK")
                addr = TUN_ADDR_FALLBACK
                prefix = TUN_PREFIX_FALLBACK
            }
            if (prefix <= 0 || prefix > 32) prefix = TUN_PREFIX_FALLBACK
            Log.i(TAG, "building TUN with server-assigned address $addr/$prefix")

            // PHASE 2: build the TUN interface with the server-assigned /32
            // address. Android configures address/routes/DNS.
            val builder = Builder()
                .setSession("MASQUE")
                .setMtu(TUN_MTU)
                .addAddress(addr, prefix)
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
                stopVpn()
                return
            }
            pfd = iface

            // PHASE 3: attach the fd to the session and start forwarding.
            t.startWithFD(iface.fd.toLong())
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
        // Launch the flavor's own launcher activity (MainActivity on phone,
        // TvMainActivity on TV) by resolving the package launch intent, so this
        // shared service does not hard-reference a flavor-specific class.
        val launchIntent = requireNotNull(
            packageManager.getLaunchIntentForPackage(packageName)
        ) {
            "No launcher activity found for $packageName"
        }.setPackage(packageName)
        val pi = PendingIntent.getActivity(
            this, 0, launchIntent,
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
