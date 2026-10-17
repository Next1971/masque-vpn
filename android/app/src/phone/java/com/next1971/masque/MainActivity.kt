package com.next1971.masque

import android.app.Activity
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.net.VpnService
import android.os.Build
import android.os.Bundle
import android.widget.Button
import android.widget.TextView
import android.widget.Toast
import androidx.activity.ComponentActivity
import androidx.activity.result.contract.ActivityResultContracts
import com.journeyapps.barcodescanner.ScanContract
import com.journeyapps.barcodescanner.ScanOptions

/**
 * MainActivity — minimal UI:
 *  - “Import Profile” (file picker → ProfileStore)
 *  - “Scan QR” (camera → the same profile text)
 *  - “Connect / Disconnect” (VPN permission request → MasqueVpnService)
 *  - status line (updated by broadcasts from the service)
 */
class MainActivity : ComponentActivity() {

    private lateinit var statusView: TextView
    private lateinit var pingView: TextView
    private lateinit var trafficView: TextView
    private lateinit var connectBtn: Button
    private var connected = false

    // Receives status updates from the service.
    private val statusReceiver = object : BroadcastReceiver() {
        override fun onReceive(c: Context?, i: Intent?) {
            val msg = i?.getStringExtra("msg") ?: return
            statusView.text = VpnStatus.statusLabel(msg)
            connected = VpnStatus.applyConnected(connected, msg)
            connectBtn.text = if (connected) "Disconnect" else "Connect"
            pingView.text = VpnStatus.pingLabel(msg) ?: if (connected) pingView.text else "Ping: —"
            trafficView.text = ProfileStore.trafficLabel(this@MainActivity)
        }
    }

    // Profile file picker.
    private val pickProfile = registerForActivityResult(
        ActivityResultContracts.GetContent()
    ) { uri ->
        if (uri == null) return@registerForActivityResult
        val text = contentResolver.openInputStream(uri)?.bufferedReader()?.use { it.readText() }
        if (text == null) {
            toast("Unable to read file")
            return@registerForActivityResult
        }
        applyProfile(text)
    }

    private val scanProfile = registerForActivityResult(ScanContract()) { result ->
        val text = result.contents
        if (text.isNullOrBlank()) return@registerForActivityResult
        applyProfile(text)
    }

    private fun applyProfile(text: String) {
        ProfileStore.import(this, text)
            .onSuccess {
                toast("Profile imported")
                if (!MasqueVpnService.isRunning) refresh()
            }
            .onFailure { toast("Profile error: ${it.message}") }
    }

    // VpnService.prepare() permission request.
    private val vpnPermission = registerForActivityResult(
        ActivityResultContracts.StartActivityForResult()
    ) { res ->
        if (res.resultCode == Activity.RESULT_OK) {
            startService(Intent(this, MasqueVpnService::class.java).setAction(MasqueVpnService.ACTION_CONNECT))
        } else {
            toast("VPN permission denied")
        }
    }

    // Battery-exemption result is unreliable on many OEMs; continue either way.
    private val batteryExemption = registerForActivityResult(
        ActivityResultContracts.StartActivityForResult()
    ) { connectVpn() }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_main)

        statusView = findViewById(R.id.status)
        pingView = findViewById(R.id.ping)
        trafficView = findViewById(R.id.traffic)
        connectBtn = findViewById(R.id.btnConnect)
        findViewById<TextView>(R.id.version).text = appVersionLabel()

        findViewById<Button>(R.id.btnImport).setOnClickListener {
            pickProfile.launch("*/*")
        }
        findViewById<Button>(R.id.btnScan).setOnClickListener {
            val options = ScanOptions()
            options.setDesiredBarcodeFormats(ScanOptions.QR_CODE)
            options.setPrompt("Point at the profile QR from masque-setup")
            options.setBeepEnabled(false)
            options.setOrientationLocked(false)
            scanProfile.launch(options)
        }

        findViewById<Button>(R.id.btnSettings).setOnClickListener {
            ClientSettings.show(this)
        }

        connectBtn.setOnClickListener {
            if (connected) {
                startService(Intent(this, MasqueVpnService::class.java).setAction(MasqueVpnService.ACTION_DISCONNECT))
            } else {
                if (!ProfileStore.isConfigured(this)) {
                    toast("Import a profile first")
                    return@setOnClickListener
                }
                try {
                    val bat = BatteryExemption.intentIfNeeded(this)
                    if (bat != null) {
                        batteryExemption.launch(bat)
                        return@setOnClickListener
                    }
                } catch (_: Exception) {
                }
                connectVpn()
            }
        }

        refreshUi()
    }

    override fun onResume() {
        super.onResume()
        val filter = IntentFilter("com.next1971.masque.STATUS")
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            registerReceiver(statusReceiver, filter, Context.RECEIVER_NOT_EXPORTED)
        } else {
            @Suppress("UnspecifiedRegisterReceiverFlag")
            registerReceiver(statusReceiver, filter)
        }
        refreshUi()
    }

    override fun onPause() {
        super.onPause()
        try { unregisterReceiver(statusReceiver) } catch (_: Exception) {}
    }

    private fun connectVpn() {
        val prep = VpnService.prepare(this)
        if (prep != null) vpnPermission.launch(prep)
        else startService(Intent(this, MasqueVpnService::class.java).setAction(MasqueVpnService.ACTION_CONNECT))
    }

    private fun refreshUi() {
        trafficView.text = ProfileStore.trafficLabel(this)
        if (MasqueVpnService.isRunning) {
            connected = true
            statusView.text = "Status: VPN active"
            connectBtn.text = "Disconnect"
        } else {
            connected = false
            connectBtn.text = "Connect"
            pingView.text = "Ping: —"
            refresh()
        }
    }

    private fun refresh() {
        val ok = ProfileStore.isConfigured(this)
        statusView.text = if (ok) "Status: profile ready" else "Status: profile not configured"
    }

    private fun toast(m: String) = Toast.makeText(this, m, Toast.LENGTH_SHORT).show()
}
