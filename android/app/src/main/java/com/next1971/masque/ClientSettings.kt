package com.next1971.masque

import android.app.Activity
import android.app.AlertDialog
import android.text.InputType
import android.view.View
import android.widget.CheckBox
import android.widget.EditText
import android.widget.LinearLayout
import android.widget.RadioButton
import android.widget.RadioGroup
import android.widget.TextView

/** Settings shared by the phone and TV activities. Values apply on the next connect. */
object ClientSettings {
    fun show(activity: Activity) {
        val pad = (activity.resources.displayMetrics.density * 16).toInt()
        val root = LinearLayout(activity).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(pad, pad / 2, pad, 0)
        }
        val group = RadioGroup(activity)
        val auto = radio(activity, "Automatic (UDP, then TCP)")
        val udp = radio(activity, "UDP only")
        val tcp = radio(activity, "TCP only")
        group.addView(auto)
        group.addView(udp)
        group.addView(tcp)
        when (ProfileStore.transport(activity)) {
            "udp" -> group.check(udp.id)
            "tcp" -> group.check(tcp.id)
            else -> group.check(auto.id)
        }
        val kill = CheckBox(activity).apply {
            text = "Kill switch (block internet if the tunnel drops)"
            isChecked = ProfileStore.killSwitch(activity)
            isFocusable = true
        }
        val mtu = EditText(activity).apply {
            inputType = InputType.TYPE_CLASS_NUMBER
            setText(ProfileStore.mtu(activity).toString())
            hint = "1340"
            isFocusable = true
        }
        val note = TextView(activity).apply {
            text = "Applies on the next connect. MTU 1280–1500, default 1340."
        }
        root.addView(TextView(activity).apply { text = "Transport" })
        root.addView(group)
        root.addView(kill)
        root.addView(TextView(activity).apply { text = "MTU" })
        root.addView(mtu)
        root.addView(note)

        AlertDialog.Builder(activity)
            .setTitle("Settings")
            .setView(root)
            .setPositiveButton("Save") { _, _ ->
                val mode = when (group.checkedRadioButtonId) {
                    udp.id -> "udp"
                    tcp.id -> "tcp"
                    else -> "auto"
                }
                ProfileStore.setTransport(activity, mode)
                ProfileStore.setKillSwitch(activity, kill.isChecked)
                val n = mtu.text?.toString()?.toIntOrNull() ?: 1340
                ProfileStore.setMTU(activity, n)
            }
            .setNegativeButton("Cancel", null)
            .show()
    }

    private fun radio(activity: Activity, label: String): RadioButton =
        RadioButton(activity).apply {
            id = View.generateViewId()
            text = label
            isFocusable = true
        }
}
