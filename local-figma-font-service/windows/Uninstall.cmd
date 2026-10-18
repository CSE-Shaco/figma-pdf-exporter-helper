@echo off
setlocal EnableExtensions DisableDelayedExpansion
set "ERRORLEVEL="
chcp 65001 >nul
"%~dp0font-service.exe" --uninstall
set "result=%errorlevel%"
echo.
echo Exit code: %result%
pause
exit /b %result%
