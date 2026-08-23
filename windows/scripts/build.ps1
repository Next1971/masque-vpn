# Build MASQUE Windows client (service + GUI + console) - PowerShell.
# Requires: Go 1.21+ in PATH. GUI build needs a C compiler (MinGW).
# Run: powershell -ExecutionPolicy Bypass -File scripts\build.ps1
$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot "..")
New-Item -ItemType Directory -Force -Path dist | Out-Null

if (-not (Get-Command go -ErrorAction SilentlyContinue)) { throw "Go not found in PATH" }
Write-Host "Using:"; go version

& (Join-Path $PSScriptRoot "fetch-wintun.ps1")

Write-Host "=== Downloading modules ==="
go mod download

Write-Host "=== Building vpn-client.exe (console / debug) ==="
$env:GOOS = "windows"; $env:GOARCH = "amd64"; $env:CGO_ENABLED = "0"
go build -trimpath -ldflags "-s -w" -o dist\vpn-client.exe .\cmd\vpn-client

Write-Host "=== Building masque-svc.exe ==="
go build -trimpath -ldflags "-s -w" -o dist\masque-svc.exe .\cmd\vpn-service

Write-Host "=== Building masque-gui.exe (needs CGO) ==="
$mingwBins = @(
    "C:\msys64\mingw64\bin",
    "C:\msys64\ucrt64\bin",
    "C:\msys64\clang64\bin"
)
foreach ($p in $mingwBins) {
    if (Test-Path (Join-Path $p "gcc.exe")) {
        $env:PATH = "$p;$env:PATH"
        Write-Host "Using gcc from $p"
        break
    }
}
if (-not (Get-Command gcc -ErrorAction SilentlyContinue)) {
    throw @"
gcc not found. MSYS2 install is not enough - install the MinGW compiler:

  Open "MSYS2 MINGW64" and run:
    pacman -S --needed mingw-w64-x86_64-gcc mingw-w64-x86_64-pkg-config

Then re-run this script (gcc lives in C:\msys64\mingw64\bin).
"@
}
$env:CGO_ENABLED = "1"
$env:CC = "gcc"
Write-Host "First Fyne/CGO compile is silent for several minutes (gcc compiling OpenGL). That is normal."
Write-Host "Subsequent builds are much faster. Progress below:"
go build -v -trimpath -ldflags "-s -w -H windowsgui" -o dist\masque-gui.exe .\cmd\vpn-gui

Write-Host ""
Write-Host "=== DONE ==="
Write-Host "Output: dist\masque-svc.exe, dist\masque-gui.exe, dist\vpn-client.exe, dist\wintun.dll"
Write-Host "Install the service (admin): sc create MasqueVpn binPath= `"$pwd\dist\masque-svc.exe`" start= auto"
Write-Host "Or build the MSI: powershell -File scripts\build-msi.ps1"
