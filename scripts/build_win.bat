@echo off
REM Build ProxyToClash.exe into build\   (build\ is gitignored)
cd /d "%~dp0\.."
md build 2>nul
go build -trimpath -ldflags "-s -w" -o build\ProxyToClash.exe .
if errorlevel 1 (
  echo [ERROR] build failed. Make sure Go is installed and on PATH.
  echo See https://go.dev/dl
  pause & exit /b 1
)
echo OK: build\ProxyToClash.exe
pause