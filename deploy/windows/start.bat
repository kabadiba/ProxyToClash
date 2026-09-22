@echo off
REM Start the converter silently in background (Windows). Binary lives in build\.
cd /d "%~dp0\..\.."
if not exist "%~dp0\..\..\build\ProxyToClash.exe" (
  echo [ERROR] build\ProxyToClash.exe not found. Run scripts\build_win.bat first.
  pause & exit /b 1
)
start "" /min "%~dp0\..\..\build\ProxyToClash.exe"
echo started. Press any key to close this window.
pause>nul