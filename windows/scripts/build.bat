@echo off
REM Build MASQUE Windows binaries. GUI requires MinGW (gcc) on PATH.
setlocal
cd /d "%~dp0.."
where go >nul 2>&1 || (echo [ERROR] Go not found in PATH & exit /b 1)
if not exist dist mkdir dist
powershell -ExecutionPolicy Bypass -File "%~dp0fetch-wintun.ps1" || exit /b 1
echo === Downloading modules ===
go mod download || exit /b 1
set GOOS=windows
set GOARCH=amd64
set CGO_ENABLED=0
echo === masque-svc.exe / vpn-client.exe ===
go build -trimpath -ldflags "-s -w" -o dist\vpn-client.exe .\cmd\vpn-client || exit /b 1
go build -trimpath -ldflags "-s -w" -o dist\masque-svc.exe .\cmd\vpn-service || exit /b 1
if exist C:\msys64\mingw64\bin\gcc.exe set PATH=C:\msys64\mingw64\bin;%PATH%
if exist C:\msys64\ucrt64\bin\gcc.exe set PATH=C:\msys64\ucrt64\bin;%PATH%
where gcc >nul 2>&1 || (
  echo [ERROR] gcc not found. In "MSYS2 MINGW64" run:
  echo   pacman -S --needed mingw-w64-x86_64-gcc mingw-w64-x86_64-pkg-config
  exit /b 1
)
set CGO_ENABLED=1
set CC=gcc
echo === masque-gui.exe ===
go build -trimpath -ldflags "-s -w -H windowsgui" -o dist\masque-gui.exe .\cmd\vpn-gui || exit /b 1
echo === masque-setup.exe ===
go build -trimpath -ldflags "-s -w -H windowsgui" -o dist\masque-setup.exe .\cmd\vpn-setup || exit /b 1
echo DONE: dist\masque-svc.exe dist\masque-gui.exe dist\masque-setup.exe dist\vpn-client.exe
endlocal
