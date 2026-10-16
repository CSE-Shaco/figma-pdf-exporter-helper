#!/bin/sh
set -eu
cd "$(dirname "$0")"
PYTHON_BIN="${PYTHON_BIN:-python3}"
python_arch=$("$PYTHON_BIN" -c 'import platform; print(platform.machine())')
target_arch="${TARGET_ARCH:-$python_arch}"
case "$target_arch" in arm64|x86_64) ;; *) echo "Unsupported TARGET_ARCH: $target_arch (use arm64 or x86_64)" >&2; exit 1 ;; esac
if [ "$python_arch" != "$target_arch" ]; then
  echo "TARGET_ARCH=$target_arch requires a $target_arch Python interpreter; selected Python is $python_arch. Use a native runner or set PYTHON_BIN." >&2
  exit 1
fi
# Recreate the architecture-specific environment: never reuse another Python or CPU's venv.
build_env=".build-venv-$target_arch"
rm -rf "$build_env"
"$PYTHON_BIN" -m venv "$build_env"
"$build_env/bin/python" -m pip install --disable-pip-version-check 'pyinstaller==6.22.3'
"$build_env/bin/python" -m PyInstaller --noconfirm --clean --onefile --console --target-arch "$target_arch" --name "font-helper-macos-$target_arch" --distpath dist --workpath "build/$target_arch" --specpath "build/$target_arch" helper.py
binary="dist/font-helper-macos-$target_arch"
[ "$(lipo -archs "$binary")" = "$target_arch" ] || { echo 'Built executable architecture mismatch' >&2; exit 1; }
printf 'Built %s/%s\n' "$PWD" "$binary"
