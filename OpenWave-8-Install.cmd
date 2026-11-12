@echo off
setlocal
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0OpenWave-8-Install.ps1"
if errorlevel 1 (
  echo.
  echo Installation failed. Press any key to close.
  pause >nul
)