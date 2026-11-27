package com.next1971.masque

import android.app.Activity
import android.app.AlertDialog
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.net.VpnService
import android.os.Build
import android.os.Bundle
import android.view.View
import android.widget.AdapterView
import android.widget.ArrayAdapter
import android.widget.Button
import android.widget.Spinner
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
    private lateinit var profileSpinner: Spinner
    private lateinit var deleteProfileBtn: Button
    private var profiles: List<ProfileInfo> = emptyList()
    private var activeLabel: String? = null
    private var connected = false
    private var connecting = false

    // Receives status updates from the service.
    private val statusReceiver = object : BroadcastReceiver() {
        override fun onReceive(c: Context?, i: Intent?) {
            val msg = i?.getStringExtra("msg") ?: return
            statusView.text = VpnStatus.statusLabel(msg)
            connected = VpnStatus.applyConnected(connected, msg)
            connecting = VpnStatus.applyConnecting(connecting, connected, msg)
            connectBtn.text = if (connected || connecting) "Disconnect" else "Connect"
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
                val name = ProfileStore.activeName(this)
                toast(if (name.isNotEmpty()) "Profile imported: $name" else "Profile imported")
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
            connecting = false
            connectBtn.text = "Connect"
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
        profileSpinner = findViewById(R.id.profileSpinner)
        deleteProfileBtn = findViewById(R.id.btnDeleteProfile)
        findViewById<TextView>(R.id.version).text = appVersionLabel()

        profileSpinner.onItemSelectedListener = object : AdapterView.OnItemSelectedListener {
            override fun onItemSelected(parent: AdapterView<*>?, view: View?, position: Int, id: Long) {
                val label = (parent?.adapter as? ArrayAdapter<String>)?.getItem(position) ?: return
                if (label == activeLabel) return // programmatic set, or already active
                val info = profiles.getOrNull(position) ?: return
                if (info.isActive) return
                val switch = {
                    if (MasqueVpnService.isRunning) {
                        startService(
                            Intent(this@MainActivity, MasqueVpnService::class.java)
                                .setAction(MasqueVpnService.ACTION_DISCONNECT)
                        )
                    }
                    ProfileStore.setActive(this@MainActivity, info.id)
                    refreshUi()
                    toast("Active profile: ${info.name}")
                }
                if (connected || connecting) {
                    AlertDialog.Builder(this@MainActivity)
                        .setTitle("Switch profile")
                        .setMessage("Switching to \"${info.name}\" disconnects the current VPN session.")
                        .setPositiveButton("Switch") { _, _ -> switch() }
                        .setNegativeButton("Cancel") { _, _ -> refreshProfiles() }
                        .show()
                } else {
                    switch()
                }
            }

            override fun onNothingSelected(parent: AdapterView<*>?) {}
        }
        deleteProfileBtn.setOnClickListener {
            val info = profiles.getOrNull(profileSpinner.selectedItemPosition) ?: return@setOnClickListener
            AlertDialog.Builder(this)
                .setTitle("Delete profile")
                .setMessage(
                    "Delete profile \"${info.name}\"? Its certificates are removed from this device. " +
                        "The profile file you imported stays where it is."
                )
                .setPositiveButton("Delete") { _, _ ->
                    if (ProfileStore.delete(this, info.id)) {
                        toast("Profile deleted")
                    } else {
                        toast("Unable to delete profile")
                    }
                    refreshUi()
                }
                .setNegativeButton("Cancel", null)
                .show()
        }

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
            if (connected || connecting) {
                connecting = false
                connectBtn.text = "Connect"
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
        connecting = true
        connectBtn.text = "Disconnect"
        statusView.text = "Status: Connecting…"
        val prep = VpnService.prepare(this)
        if (prep != null) vpnPermission.launch(prep)
        else startService(Intent(this, MasqueVpnService::class.java).setAction(MasqueVpnService.ACTION_CONNECT))
    }

    private fun refreshUi() {
        trafficView.text = ProfileStore.trafficLabel(this)
        refreshProfiles()
        if (MasqueVpnService.isRunning) {
            connected = true
            connecting = false
            statusView.text = if (MasqueVpnService.restoring) {
                "Status: Restoring connection…"
            } else {
                "Status: VPN active"
            }
            if (MasqueVpnService.restoring) pingView.text = "Ping: —"
            connectBtn.text = "Disconnect"
        } else if (!connecting) {
            connected = false
            connectBtn.text = "Connect"
            pingView.text = "Ping: —"
            refresh()
        }
    }

    private fun refresh() {
        val name = ProfileStore.activeName(this)
        statusView.text = if (name.isNotEmpty()) "Status: profile ready · $name" else "Status: profile not configured"
    }

    /** Syncs the profile picker with what ProfileStore has on disk. The
     *  onItemSelected handler ignores the programmatic set because the label it
     *  receives then equals activeLabel. */
    private fun refreshProfiles() {
        profiles = ProfileStore.list(this)
        val labels = profiles.map { labelFor(it) }
        activeLabel = profiles.firstOrNull { it.isActive }?.let { labelFor(it) }
        val adapter = ArrayAdapter(this, android.R.layout.simple_spinner_item, labels)
        adapter.setDropDownViewResource(android.R.layout.simple_spinner_dropdown_item)
        profileSpinner.adapter = adapter
        val idx = profiles.indexOfFirst { it.isActive }
        if (idx >= 0) profileSpinner.setSelection(idx, false)
        deleteProfileBtn.isEnabled = idx >= 0
    }

    /** Duplicate profile names get the server address appended. */
    private fun labelFor(info: ProfileInfo): String {
        val dup = profiles.count { it.name == info.name } > 1
        return if (dup && info.server.isNotEmpty()) "${info.name} — ${info.server}" else info.name
    }

    private fun toast(m: String) = Toast.makeText(this, m, Toast.LENGTH_SHORT).show()
}
