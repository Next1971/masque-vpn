# Build MASQUE Windows client (vpn-client.exe) — PowerShell.
# Requires: Go 1.21+ in PATH. Run: powershell -ExecutionPolicy Bypass -File scripts\build.ps1
$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot "..")

if (-not (Get-Command go -ErrorAction SilentlyContinue)) { throw "Go not found in PATH" }
Write-Host "Using:"; go version

Write-Host "=== Downloading modules ==="
go mod download

Write-Host "=== Building vpn-client.exe (windows/amd64) ==="
$env:GOOS = "windows"; $env:GOARCH = "amd64"
go build -trimpath -ldflags "-s -w" -o dist\vpn-client.exe .\cmd\vpn-client

Write-Host ""
Write-Host "=== DONE ==="
Write-Host "Output: dist\vpn-client.exe (wintun.dll already in dist\)"
