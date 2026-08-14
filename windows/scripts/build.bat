@echo off
REM ============================================================
REM  Build MASQUE Windows client (vpn-client.exe).
REM  Requires: Go 1.21+ installed and in PATH.
REM  Run from anywhere; paths are resolved relative to this script.
REM ============================================================
setlocal
cd /d "%~dp0.."

where go >nul 2>&1 || (echo [ERROR] Go not found in PATH & exit /b 1)
echo Using: & go version

echo === Downloading modules ===
go mod download || (echo [ERROR] go mod download failed & exit /b 1)

echo === Building vpn-client.exe (windows/amd64) ===
set GOOS=windows
set GOARCH=amd64
go build -trimpath -ldflags "-s -w" -o dist\vpn-client.exe .\cmd\vpn-client
if errorlevel 1 (echo [ERROR] build failed & exit /b 1)

echo.
echo === DONE ===
echo Output: dist\vpn-client.exe
echo Make sure dist\wintun.dll is present next to the EXE (already included).
endlocal
