# Install MASQUE VPN as a Windows service without an MSI.
# Run from an elevated PowerShell: powershell -ExecutionPolicy Bypass -File scripts\install-service.ps1
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$dist = Join-Path $root "dist"
$installDir = Join-Path $env:ProgramFiles "MASQUE"

foreach ($f in @("masque-svc.exe", "masque-gui.exe", "vpn-client.exe", "wintun.dll")) {
    $p = Join-Path $dist $f
    if (-not (Test-Path $p)) { throw "missing $p - run scripts\build.ps1 first" }
}

$id = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = New-Object Security.Principal.WindowsPrincipal($id)
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw "Run this script in an elevated PowerShell (Run as administrator)."
}

New-Item -ItemType Directory -Force -Path $installDir | Out-Null
Copy-Item -Force (Join-Path $dist "*") $installDir
$svcExe = Join-Path $installDir "masque-svc.exe"

$existing = Get-Service -Name "MasqueVpn" -ErrorAction SilentlyContinue
if ($existing) {
    if ($existing.Status -eq "Running") { Stop-Service MasqueVpn -Force }
    sc.exe delete MasqueVpn | Out-Null
    Start-Sleep -Seconds 1
}

sc.exe create MasqueVpn binPath= "`"$svcExe`"" start= auto DisplayName= "MASQUE VPN" | Out-Null
sc.exe description MasqueVpn "MASQUE VPN tunnel (Wintun + QUIC)" | Out-Null
Start-Service MasqueVpn
Write-Host "Service MasqueVpn installed and started."
Write-Host "GUI (no admin): $installDir\masque-gui.exe"
Write-Host "Log: $env:ProgramData\MASQUE\masque-svc.log"
