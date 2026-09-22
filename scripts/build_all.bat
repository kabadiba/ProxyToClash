@echo off
REM Build all platforms into build\   (build\ is gitignored)
cd /d "%~dp0\.."
md build 2>nul

set CGO_ENABLED=0

echo == build linux/amd64 ==
set GOOS=linux
set GOARCH=amd64
go build -trimpath -ldflags "-s -w" -o build\ProxyToClash_linux_amd64 .

echo == build linux/arm64 ==
set GOOS=linux
set GOARCH=arm64
go build -trimpath -ldflags "-s -w" -o build\ProxyToClash_linux_arm64 .

echo == build windows/amd64 ==
set GOOS=windows
set GOARCH=amd64
go build -trimpath -ldflags "-s -w" -o build\ProxyToClash_windows_amd64.exe .

echo == done. artifacts in build\ ==
dir /b build
pause