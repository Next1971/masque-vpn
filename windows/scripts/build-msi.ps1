# Build masque.msi with WiX (wix.exe). Requires: dist\ binaries from build.ps1.
$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot "..")

$dotnetTools = Join-Path $env:USERPROFILE ".dotnet\tools"
if (Test-Path $dotnetTools) {
    $env:PATH = "$dotnetTools;$env:PATH"
}

$wix = Get-Command wix -ErrorAction SilentlyContinue
if (-not $wix) {
    $candidate = Join-Path $dotnetTools "wix.exe"
    if (Test-Path $candidate) { $wix = $candidate }
}
if (-not $wix) {
    throw "wix.exe not found under $dotnetTools. Install: dotnet tool install --global wix"
}

foreach ($f in @("masque-svc.exe", "masque-gui.exe", "vpn-client.exe", "wintun.dll")) {
    $p = Join-Path "dist" $f
    if (-not (Test-Path $p)) { throw "missing $p - run scripts\build.ps1 first" }
}

# -acceptEula wix7: required by WiX v7 OSMF (https://wixtoolset.org/osmf/). Harmless no-op on v5.
& $wix build -acceptEula wix7 .\installer\masque.wxs -bindpath bin=dist -out dist\masque.msi -arch x64
if ($LASTEXITCODE -ne 0) { throw "wix build failed with exit $LASTEXITCODE" }
Write-Host "MSI: dist\masque.msi"
