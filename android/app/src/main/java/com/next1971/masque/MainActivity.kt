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

/**
 * MainActivity — minimal UI:
 *  - “Import Profile” (file picker → ProfileStore)
 *  - “Connect / Disconnect” (VPN permission request → MasqueVpnService)
 *  - status line (updated by broadcasts from the service)
 */
class MainActivity : ComponentActivity() {

    private lateinit var statusView: TextView
    private lateinit var connectBtn: Button
    private var connected = false

    // Receives status updates from the service.
    private val statusReceiver = object : BroadcastReceiver() {
        override fun onReceive(c: Context?, i: Intent?) {
            val msg = i?.getStringExtra("msg") ?: return
            statusView.text = "Status: $msg"
            connected = msg == "Connected" || msg == "VPN active"
            connectBtn.text = if (connected) "Disconnect" else "Connect"
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
        ProfileStore.import(this, text)
            .onSuccess {
                toast("Profile imported")
                refresh()
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

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_main)

        statusView = findViewById(R.id.status)
        connectBtn = findViewById(R.id.btnConnect)

        findViewById<Button>(R.id.btnImport).setOnClickListener {
            pickProfile.launch("*/*")
        }

        connectBtn.setOnClickListener {
            if (connected) {
                startService(Intent(this, MasqueVpnService::class.java).setAction(MasqueVpnService.ACTION_DISCONNECT))
            } else {
                if (!ProfileStore.isConfigured(this)) {
                    toast("Import a profile first")
                    return@setOnClickListener
                }
                val prep = VpnService.prepare(this)
                if (prep != null) vpnPermission.launch(prep)
                else startService(Intent(this, MasqueVpnService::class.java).setAction(MasqueVpnService.ACTION_CONNECT))
            }
        }

        refresh()
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
    }

    override fun onPause() {
        super.onPause()
        try { unregisterReceiver(statusReceiver) } catch (_: Exception) {}
    }

    private fun refresh() {
        val ok = ProfileStore.isConfigured(this)
        statusView.text = if (ok) "Status: profile ready" else "Status: profile not configured"
    }

    private fun toast(m: String) = Toast.makeText(this, m, Toast.LENGTH_SHORT).show()
}
