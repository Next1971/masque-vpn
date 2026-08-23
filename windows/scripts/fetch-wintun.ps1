# Fetch official wintun.dll into dist\ (needed next to masque-svc.exe and in the MSI).
$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot "..")
New-Item -ItemType Directory -Force -Path dist | Out-Null
$dll = Join-Path (Get-Location) "dist\wintun.dll"
if (Test-Path $dll) { Write-Host "wintun.dll already present"; exit 0 }
$zip = Join-Path $env:TEMP "wintun-0.14.1.zip"
Invoke-WebRequest -Uri "https://www.wintun.net/builds/wintun-0.14.1.zip" -OutFile $zip
$out = Join-Path $env:TEMP "wintun-extract"
if (Test-Path $out) { Remove-Item -Recurse -Force $out }
Expand-Archive -Path $zip -DestinationPath $out
Copy-Item -Force (Join-Path $out "wintun\bin\amd64\wintun.dll") $dll
Write-Host "Saved $dll"
