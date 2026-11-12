@echo off
chcp 65001 >nul
set "OPENWAVE_BIND=0.0.0.0:4567"
echo.
echo OpenWave backend for iPhone is starting on port 4567...
echo Find the PC IPv4 address below and use http://PC_IP:4567 on the iPhone.
ipconfig | findstr /C:"IPv4" /C:"IPv4-адрес"
echo.
start "OpenWave" "%~dp0OpenWave-8-personal.exe"
echo Keep OpenWave running while listening from the iPhone.
pause
