package com.next1971.masque

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Intent
import android.net.ConnectivityManager
import android.net.Network
import android.net.NetworkCapabilities
import android.net.NetworkRequest
import android.net.VpnService
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.os.ParcelFileDescriptor
import android.util.Log
import mobile.Callback
import mobile.Config
import mobile.Mobile
import mobile.Tunnel
import java.io.FileDescriptor

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

        @Volatile
        var isRunning: Boolean = false
            private set

        const val TUN_ADDR_FALLBACK = "10.8.0.254"
        // Server still assigns a CONNECT-IP /32. VpnService needs /24 on-link
        // or some OEMs source packets from Wi-Fi (192.168.x.x); the server
        // drops them and DNS never reaches 1.1.1.1.
        const val TUN_PREFIX = 24
        const val TUN_MTU = 1400
    }

    private var tunnel: Tunnel? = null
    private var pfd: ParcelFileDescriptor? = null
    private var networksRegistered = false
    private var underlying: Network? = null
    private val rttHandler = Handler(Looper.getMainLooper())
    private val rttTick = object : Runnable {
        override fun run() {
            val t = tunnel
            if (!isRunning || t == null) return
            val ms = try {
                t.rttMillis()
            } catch (_: Exception) {
                0L
            }
            val text = if (ms > 0) "VPN active · ping $ms ms" else "VPN active"
            broadcast(text)
            updateNotification(text)
            rttHandler.postDelayed(this, 2000)
        }
    }

    private val connectivity by lazy { getSystemService(ConnectivityManager::class.java) }

    private val networkCallback = object : ConnectivityManager.NetworkCallback() {
        override fun onAvailable(network: Network) {
            Log.i(TAG, "underlying network available: $network")
            applyUnderlying(network)
        }

        override fun onCapabilitiesChanged(network: Network, caps: NetworkCapabilities) {
            applyUnderlying(network)
        }

        override fun onLost(network: Network) {
            Log.i(TAG, "underlying network lost: $network")
            if (underlying == network) {
                underlying = null
            }
        }
    }

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
        if (tunnel != null && pfd != null) {
            Log.i(TAG, "VPN already running; ignore extra CONNECT")
            return
        }

        val prof = ProfileStore.load(this)
        if (prof == null) {
            Log.e(TAG, "no profile configured")
            broadcast("Error: profile not configured")
            stopSelf()
            return
        }

        startForeground(NOTIF_ID, buildNotification("Connecting…"))

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
                if (msg == "assigned-ip-changed") {
                    Handler(Looper.getMainLooper()).post { rebuildTunnel() }
                    return
                }
                broadcast(msg ?: "")
                updateNotification(msg ?: "Connected")
                if (msg != null && msg.startsWith("reconnect")) {
                    underlying?.let { applyUnderlying(it) } ?: protectUdp()
                }
            }
            override fun onError(msg: String?) {
                Log.e(TAG, "fatal: $msg")
                broadcast("Error: $msg")
                stopVpn()
            }
        }

        try {
            val t = Mobile.dial(cfg, cb)
            tunnel = t

            var addr = t.assignedAddr()
            if (addr.isNullOrEmpty()) {
                Log.w(TAG, "server assigned no address; using fallback $TUN_ADDR_FALLBACK")
                addr = TUN_ADDR_FALLBACK
            }
            Log.i(TAG, "building TUN $addr/$TUN_PREFIX (server assigned /${t.assignedPrefixLen()})")

            val builder = Builder()
                .setSession("MASQUE")
                .setMtu(TUN_MTU)
                .addAddress(addr, TUN_PREFIX)
                // Sink IPv6 into the TUN (core drops it). Without ::/0, Telegram and
                // others use IPv6 on the underlying network and bypass the VPN.
                .addAddress("fd00::1", 128)
                .addRoute("0.0.0.0", 0)
                .addRoute("::", 0)
                .addDnsServer(prof.dns)
            if (prof.dns != "8.8.8.8") {
                builder.addDnsServer("8.8.8.8")
            }

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

            t.startWithFD(iface.fd.toLong())
            registerUnderlyingNetworks()
            protectUdp()
            isRunning = true
            broadcast("Connected")
            updateNotification("VPN active")
            rttHandler.removeCallbacks(rttTick)
            rttHandler.post(rttTick)
        } catch (e: Exception) {
            Log.e(TAG, "connect failed", e)
            broadcast("Connection error: ${e.message}")
            stopVpn()
        }
    }

    private fun registerUnderlyingNetworks() {
        if (networksRegistered) return
        val request = NetworkRequest.Builder()
            .addCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)
            .addCapability(NetworkCapabilities.NET_CAPABILITY_NOT_VPN)
            .build()
        try {
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                connectivity.registerNetworkCallback(
                    request,
                    networkCallback,
                    Handler(Looper.getMainLooper())
                )
            } else {
                connectivity.registerNetworkCallback(request, networkCallback)
            }
            networksRegistered = true
        } catch (e: Exception) {
            Log.w(TAG, "registerNetworkCallback: ${e.message}")
        }
    }

    private fun unregisterUnderlyingNetworks() {
        if (!networksRegistered) return
        try {
            connectivity.unregisterNetworkCallback(networkCallback)
        } catch (e: Exception) {
            Log.w(TAG, "unregisterNetworkCallback: ${e.message}")
        }
        networksRegistered = false
        underlying = null
    }

    private fun applyUnderlying(network: Network) {
        underlying = network
        try {
            setUnderlyingNetworks(arrayOf(network))
        } catch (e: Exception) {
            Log.w(TAG, "setUnderlyingNetworks: ${e.message}")
        }
        protectUdp()
        bindUdp(network)
    }

    private fun protectUdp() {
        val fd = tunnel?.udpFd()?.toInt() ?: return
        if (fd <= 0) return
        if (!protect(fd)) {
            Log.w(TAG, "protect($fd) failed")
        } else {
            Log.i(TAG, "protected UDP fd $fd")
        }
    }

    private fun bindUdp(network: Network) {
        val fd = tunnel?.udpFd()?.toInt() ?: return
        if (fd <= 0) return
        try {
            val javaFd = FileDescriptor()
            val field = FileDescriptor::class.java.declaredFields.firstOrNull {
                it.name == "descriptor" || it.name == "fd"
            } ?: return
            field.isAccessible = true
            field.setInt(javaFd, fd)
            network.bindSocket(javaFd)
            Log.i(TAG, "bound UDP fd $fd to $network")
        } catch (e: Exception) {
            Log.w(TAG, "bindSocket: ${e.message}")
        }
    }

    private fun rebuildTunnel() {
        Log.i(TAG, "assigned IP changed; rebuilding VPN session")
        tearDownTunnel(stopService = false)
        startVpn()
    }

    private fun stopVpn() {
        tearDownTunnel(stopService = true)
    }

    private fun tearDownTunnel(stopService: Boolean) {
        isRunning = false
        rttHandler.removeCallbacks(rttTick)
        unregisterUnderlyingNetworks()
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
        if (stopService) {
            stopForeground(STOP_FOREGROUND_REMOVE)
            stopSelf()
        }
    }

    override fun onDestroy() {
        stopVpn()
        super.onDestroy()
    }

    private fun broadcast(msg: String) {
        sendBroadcast(Intent("com.next1971.masque.STATUS").putExtra("msg", msg).setPackage(packageName))
    }

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
            .setSmallIcon(R.drawable.ic_stat_masque)
            .setContentIntent(pi)
            .setOngoing(true)
            .build()
    }

    private fun updateNotification(text: String) {
        val nm = getSystemService(NotificationManager::class.java)
        nm.notify(NOTIF_ID, buildNotification(text))
    }
}
