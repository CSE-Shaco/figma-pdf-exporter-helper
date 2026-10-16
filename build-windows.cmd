@echo off
setlocal
cd /d "%~dp0"
py -3 -m venv .build-venv
if errorlevel 1 exit /b 1
.build-venv\Scripts\python.exe -m pip install --disable-pip-version-check pyinstaller==6.22.3
if errorlevel 1 exit /b 1
.build-venv\Scripts\pyinstaller.exe --noconfirm --clean --onefile --console --name font-helper-windows --distpath dist --workpath build --specpath build helper.py
if errorlevel 1 exit /b 1
echo Built %CD%\dist\font-helper-windows.exe
