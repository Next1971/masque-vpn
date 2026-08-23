package com.next1971.masque

import android.app.Activity
import android.app.AlertDialog
import android.content.BroadcastReceiver
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.net.VpnService
import android.os.Build
import android.os.Bundle
import android.view.View
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
 * - Large, D-pad-focusable buttons (no touch required).
 * - Profile import via PASTED TEXT (on-screen keyboard) or CLIPBOARD
 * (one remote click; no IME paste, which many TVs cannot do).
 */
class TvMainActivity : ComponentActivity() {

    private lateinit var statusView: TextView
    private lateinit var connectBtn: Button
    private var connected = false

    private val statusReceiver = object : BroadcastReceiver() {
        override fun onReceive(c: Context?, i: Intent?) {
            val msg = i?.getStringExtra("msg") ?: return
            statusView.text = "Status: $msg"
            connected = VpnStatus.applyConnected(connected, msg)
            connectBtn.text = if (connected) "Disconnect" else "Connect"
        }
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

    private val batteryExemption = registerForActivityResult(
        ActivityResultContracts.StartActivityForResult()
    ) { connectVpn() }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_tv_main)

        statusView = findViewById(R.id.tvStatus)
        connectBtn = findViewById(R.id.tvBtnConnect)

        val pasteBtn = findViewById<Button>(R.id.tvBtnPaste)
        val clipboardBtn = findViewById<Button>(R.id.tvBtnClipboard)

        pasteBtn.setOnClickListener { showPasteDialog() }
        clipboardBtn.setOnClickListener { pasteFromClipboard() }

        setupTvFocusAnimation(pasteBtn)
        setupTvFocusAnimation(clipboardBtn)
        setupTvFocusAnimation(connectBtn)

        connectBtn.setOnClickListener {
            if (connected) {
                startService(Intent(this, MasqueVpnService::class.java).setAction(MasqueVpnService.ACTION_DISCONNECT))
            } else {
                if (!ProfileStore.isConfigured(this)) { toast("Import a profile first"); return@setOnClickListener }
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

        // Give the remote a sensible initial focus target.
        (if (ProfileStore.isConfigured(this)) connectBtn else clipboardBtn).requestFocus()

        refreshUi()
    }

    /** Slight scale-up on focus so the active button is obvious on TV from distance. */
    private fun setupTvFocusAnimation(view: View) {
        view.onFocusChangeListener = View.OnFocusChangeListener { v, hasFocus ->
            val scale = if (hasFocus) 1.06f else 1f
            v.animate()
                .scaleX(scale)
                .scaleY(scale)
                .setDuration(120)
                .start()
        }
    }

    /**
     * Reads the system clipboard on a user click (app is in the foreground,
     * so Android 10+ clipboard restrictions do not apply). Avoids the IME
     * paste path, which is missing or broken on some TVs.
     */
    private fun pasteFromClipboard() {
        val cm = getSystemService(ClipboardManager::class.java) ?: run {
            toast("Clipboard is unavailable on this TV")
            return
        }
        val clip = try {
            cm.primaryClip
        } catch (e: Exception) {
            toast("Cannot read clipboard: ${e.message}")
            return
        }
        if (clip == null || clip.itemCount == 0) {
            toast("Clipboard is empty. Copy the .masque profile on this TV, then try again.")
            return
        }
        val text = clip.getItemAt(0).coerceToText(this).toString()
        if (text.isBlank()) {
            toast("Clipboard is empty. Copy the .masque profile on this TV, then try again.")
            return
        }
        applyProfile(text)
    }

    /** Fallback import: type or IME-paste into a dialog. */
    private fun showPasteDialog() {
        val input = EditText(this).apply {
            setSingleLine(false)
            minLines = 6
            hint = "Paste the contents of your .masque profile here"
        }
        AlertDialog.Builder(this, R.style.Theme_MasqueTv_Dialog)
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
            .onSuccess { toast("Profile imported"); if (!MasqueVpnService.isRunning) refresh() }
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
        if (MasqueVpnService.isRunning) {
            connected = true
            statusView.text = "Status: VPN active"
            connectBtn.text = "Disconnect"
        } else {
            connected = false
            connectBtn.text = "Connect"
            refresh()
        }
    }

    private fun refresh() {
        val ok = ProfileStore.isConfigured(this)
        statusView.text = if (ok) "Status: profile ready" else "Status: profile not configured"
    }

    private fun toast(m: String) = Toast.makeText(this, m, Toast.LENGTH_LONG).show()
}
