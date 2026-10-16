#!/bin/sh
cd "$(dirname "$0")" || exit 1
host_arch=$(uname -m)
# A Terminal launched under Rosetta can report x86_64 on Apple Silicon.
if [ "$(sysctl -n hw.optional.arm64 2>/dev/null || true)" = 1 ]; then host_arch=arm64; fi
case "$host_arch" in arm64|x86_64) ;; *) echo "지원하지 않는 Mac CPU: $host_arch" >&2; exit 1 ;; esac
binary="dist/font-helper-macos-$host_arch"
status=0
if [ -x "$binary" ]; then
  "$binary" "$@" || status=$?
elif [ -x dist/font-helper-macos ] && lipo -verify_arch "$host_arch" dist/font-helper-macos >/dev/null 2>&1; then
  ./dist/font-helper-macos "$@" || status=$?
elif [ -f helper.py ] && command -v python3 >/dev/null 2>&1; then
  python3 helper.py "$@" || status=$?
else
  echo "이 Mac에 맞는 도우미($host_arch)가 없습니다. macOS-$host_arch 압축 파일을 다운로드하세요." >&2
  status=1
fi
if [ -t 0 ]; then
  printf '\n종료하려면 Enter를 누르세요.'
  read answer || true
fi
exit "$status"
