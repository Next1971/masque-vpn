#Requires -Version 5.1
# Encode local signing files as single-line base64 for GitHub Secrets.
# Put dist.p12, app.mobileprovision, and tunnel.mobileprovision in this folder
# (or pass -Dir), then run:  powershell -File encode-secrets.ps1

param(
    [string]$Dir = (Join-Path $PSScriptRoot "..\signing-local")
)

$ErrorActionPreference = "Stop"
if (-not (Test-Path $Dir)) {
    Write-Error "Folder not found: $Dir`nCreate it and place dist.p12, app.mobileprovision, tunnel.mobileprovision"
}

function Show-B64([string]$Path, [string]$SecretName) {
    if (-not (Test-Path $Path)) {
        Write-Host "SKIP (missing): $Path"
        return
    }
    $b64 = [Convert]::ToBase64String([IO.File]::ReadAllBytes($Path))
    $out = Join-Path $Dir "$SecretName.txt"
    Set-Content -Path $out -Value $b64 -NoNewline -Encoding ascii
    Write-Host "Wrote $out ($((Get-Item $Path).Name) -> secret $SecretName)"
}

Show-B64 (Join-Path $Dir "dist.p12") "BUILD_CERTIFICATE_BASE64"
Show-B64 (Join-Path $Dir "app.mobileprovision") "BUILD_PROVISION_PROFILE_APP_BASE64"
Show-B64 (Join-Path $Dir "tunnel.mobileprovision") "BUILD_PROVISION_PROFILE_EXT_BASE64"
Write-Host "Open each .txt, copy the one line, paste into GitHub Secrets. Do not commit signing-local/."
