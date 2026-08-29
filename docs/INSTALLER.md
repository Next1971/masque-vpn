# Experimental Windows VPS Setup Helper

## Status
Test pre-release. Do not use on a server you cannot rebuild.

## What it changes
- Connects over SSH as root
- Installs or updates packages with apt
- Adds firewall rules with ufw
- Configures Linux forwarding/NAT
- Installs and enables a systemd service
- Copies the selected server binary
- Generates local CA and client credentials

## What it does not do
- Does not install a new Android or Windows VPN client
- Does not include the Linux server binary
- Does not provide individual certificate revocation
- Does not guarantee compatibility with existing firewall/VPN/container setups
- Does not make a VPS secure by itself

## Prerequisites
- Fresh or disposable Debian/Ubuntu VPS
- Root SSH access
- VPS public IP or hostname
- UDP port available
- Matching amd64 or arm64 server binary
- Backup or console access

## Before clicking Install
- Confirm the server has no valuable workloads
- Confirm SSH recovery / provider console access
- Check the server architecture
- Save existing firewall and network configuration

## Expected result
- Service status
- Listening UDP port
- Created client-profile location
- Exact next steps for Android/Windows import

## Recovery and removal
...
