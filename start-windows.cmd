@echo off
cd /d "%~dp0"
if exist "dist\font-helper-windows.exe" (
  "dist\font-helper-windows.exe"
) else (
  powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0helper-windows.ps1"
)
pause
