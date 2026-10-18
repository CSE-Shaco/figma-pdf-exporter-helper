@echo off
setlocal EnableExtensions DisableDelayedExpansion
set "ERRORLEVEL="
chcp 65001 >nul
echo Figma PDF Exporter local font bridge
echo Installs for this Windows user and starts automatically at login.
echo No administrator, Node.js, or Go installation is required.
choice /C YN /N /M "Install or update? [Y/N] "
if errorlevel 2 goto declined
if not errorlevel 1 goto declined
"%~dp0font-service.exe" --install
set "result=%errorlevel%"
echo.
echo Exit code: %result%
pause
exit /b %result%
:declined
echo E_USER_DECLINED: no changes made.
pause
exit /b 23
