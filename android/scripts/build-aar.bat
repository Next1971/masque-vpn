@echo off
REM ============================================================
REM  Build masque.aar from the Go core through gomobile (Windows).
REM  Requires: Go, Android SDK + NDK (installed through Android Studio),
REM  and the ANDROID_HOME and ANDROID_NDK_HOME variables.
REM ============================================================
setlocal

REM --- 1. Path to the masque-vpn Go module source ---
set GOSRC=%~dp0..\go-src\masque-vpn

echo === Checking environment ===
where go >nul 2>&1 || (echo [ERROR] Go was not found in PATH & exit /b 1)
if "%ANDROID_HOME%"=="" (echo [ERROR] ANDROID_HOME is not set & exit /b 1)
if "%ANDROID_NDK_HOME%"=="" (echo [ERROR] ANDROID_NDK_HOME is not set & exit /b 1)
go version

echo === Installing gomobile ===
go install golang.org/x/mobile/cmd/gomobile@latest
go install golang.org/x/mobile/cmd/gobind@latest
REM  Add %GOPATH%\bin to PATH for the duration of the build
for /f "delims=" %%i in ('go env GOPATH') do set GOPATH=%%i
set PATH=%GOPATH%\bin;%PATH%

echo === gomobile init ===
gomobile init

echo === Building AAR ===
pushd "%GOSRC%"
REM  Build the mobile package specifically; output is masque.aar in app\libs
gomobile bind -target=android -androidapi 24 -o "%~dp0..\app\libs\masque.aar" ./mobile
set RC=%ERRORLEVEL%
popd

if %RC%==0 (
  echo === DONE: app\libs\masque.aar built ===
) else (
  echo [ERROR] gomobile bind returned code %RC%
)
endlocal & exit /b %RC%
