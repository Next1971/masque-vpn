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
import android.widget.Button
import android.widget.EditText
import android.widget.TextView
import android.widget.Toast
import androidx.activity.ComponentActivity
import androidx.activity.result.contract.ActivityResultContracts

/**
 * TvMainActivity — Android TV (leanback) entry point.
 *
 * Same VPN core as the phone build (MasqueVpnService + ProfileStore + Go
 * masque.aar). The only differences are TV-oriented:
 *   - Large, D-pad-focusable buttons (no touch required).
 *   - Profile import via PASTED TEXT (works with the remote's on-screen
 *     keyboard), plus a file-picker fallback for TVs that expose a file
 *     provider or a plugged-in USB drive.
 *
 * Import is the only genuinely TV-specific concern: most TVs have no share
 * sheet or file manager, so paste-text is the reliable primary path.
 */
class TvMainActivity : ComponentActivity() {

    private lateinit var statusView: TextView
    private lateinit var connectBtn: Button
    private var connected = false

    private val statusReceiver = object : BroadcastReceiver() {
        override fun onReceive(c: Context?, i: Intent?) {
            val msg = i?.getStringExtra("msg") ?: return
            statusView.text = "Status: $msg"
            connected = msg == "Connected" || msg == "VPN active"
            connectBtn.text = if (connected) "Disconnect" else "Connect"
        }
    }

    // Optional file-picker fallback (USB drive / documents provider).
    private val pickProfile = registerForActivityResult(
        ActivityResultContracts.GetContent()
    ) { uri ->
        if (uri == null) return@registerForActivityResult
        val text = contentResolver.openInputStream(uri)?.bufferedReader()?.use { it.readText() }
        if (text == null) { toast("Unable to read file"); return@registerForActivityResult }
        applyProfile(text)
    }

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
        setContentView(R.layout.activity_tv_main)

        statusView = findViewById(R.id.tvStatus)
        connectBtn = findViewById(R.id.tvBtnConnect)

        findViewById<Button>(R.id.tvBtnPaste).setOnClickListener { showPasteDialog() }
        findViewById<Button>(R.id.tvBtnImportFile).setOnClickListener { pickProfile.launch("*/*") }

        connectBtn.setOnClickListener {
            if (connected) {
                startService(Intent(this, MasqueVpnService::class.java).setAction(MasqueVpnService.ACTION_DISCONNECT))
            } else {
                if (!ProfileStore.isConfigured(this)) { toast("Import a profile first"); return@setOnClickListener }
                val prep = VpnService.prepare(this)
                if (prep != null) vpnPermission.launch(prep)
                else startService(Intent(this, MasqueVpnService::class.java).setAction(MasqueVpnService.ACTION_CONNECT))
            }
        }

        // Give the remote a sensible initial focus target.
        (if (ProfileStore.isConfigured(this)) connectBtn
         else findViewById<Button>(R.id.tvBtnPaste)).requestFocus()

        refresh()
    }

    /** Paste-text import: reliable on TV where file managers are absent. */
    private fun showPasteDialog() {
        val input = EditText(this).apply {
            setSingleLine(false)
            minLines = 6
            hint = "Paste the contents of your .masque profile here"
        }
        AlertDialog.Builder(this)
            .setTitle("Import profile (paste text)")
            .setView(input)
            .setPositiveButton("Import") { _, _ ->
                val text = input.text?.toString().orEmpty()
                if (text.isBlank()) toast("Nothing pasted") else applyProfile(text)
            }
            .setNegativeButton("Cancel", null)
            .show()
    }

    private fun applyProfile(text: String) {
        ProfileStore.import(this, text)
            .onSuccess { toast("Profile imported"); refresh() }
            .onFailure { toast("Profile error: ${it.message}") }
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

    private fun toast(m: String) = Toast.makeText(this, m, Toast.LENGTH_LONG).show()
}
