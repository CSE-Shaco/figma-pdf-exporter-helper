#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
GO_BIN="${GO_BIN:-go}"
export GOTOOLCHAIN=local CGO_ENABLED=0
version="$("$GO_BIN" version)"
if [[ "$version" != *"go1.27.2 "* ]]; then
  echo "Expected official Go 1.27.2 (set GO_BIN to its absolute executable path)." >&2
  exit 1
fi
mkdir -p dist
for arch in amd64 arm64; do
  label=x64
  if [[ "$arch" == arm64 ]]; then label=arm64; fi
  package="dist/figma-pdf-font-service-windows-$label"
  mkdir -p "$package"
  GOOS=windows GOARCH="$arch" "$GO_BIN" build -trimpath -ldflags='-s -w -H=windowsgui' -o "$package/font-service.exe" ./cmd/font-service
  cp -f Install.cmd Uninstall.cmd README.md "$package/"
  cp -f "$("$GO_BIN" env GOROOT)/LICENSE" "$package/GO-LICENSE.txt"
  sysdir="$("$GO_BIN" list -m -f '{{.Dir}}' golang.org/x/sys)"
  cp -f "$sysdir/LICENSE" "$package/X-SYS-LICENSE.txt"
  cp -f THIRD_PARTY_NOTICES.txt ../LICENSE "$package/"
  python3 - "$package" <<'PY'
import hashlib,pathlib,sys,zipfile
root=pathlib.Path(sys.argv[1])
names=['font-service.exe','Install.cmd','Uninstall.cmd','README.md','GO-LICENSE.txt','X-SYS-LICENSE.txt','THIRD_PARTY_NOTICES.txt','LICENSE']
files=[root/name for name in sorted(names)]
(root/'SHA256SUMS.txt').write_text(''.join(hashlib.sha256(p.read_bytes()).hexdigest()+'  '+p.name+'\n' for p in files))
with zipfile.ZipFile(str(root)+'.zip','w',zipfile.ZIP_DEFLATED) as z:
    for p in files+[root/'SHA256SUMS.txt']:z.write(p,p.name)
PY
done
python3 - <<'PY'
import hashlib,pathlib
root=pathlib.Path('dist')
(root/'SHA256SUMS.txt').write_text(''.join(hashlib.sha256(p.read_bytes()).hexdigest()+'  '+p.name+'\n' for p in [root/'figma-pdf-font-service-windows-arm64.zip',root/'figma-pdf-font-service-windows-x64.zip']))
PY
