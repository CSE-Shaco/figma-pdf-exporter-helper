# Figma PDF Exporter Font Service

A small native relay for **Figma PDF Exporter → Pro → Figma 글꼴 찾기**. It requests Figma Desktop’s existing local font API with the Origin required by Figma and returns only catalog-listed fonts. It does not scan font folders, bundle fonts, inject code, or patch Figma.

## Download and install

Get the ZIP for your computer from [v0.2.0 releases](https://github.com/CSE-Shaco/figma-pdf-exporter-helper/releases/tag/v0.2.0). Extract it before running the installer. No administrator access, compiler, Node.js, Python, or separate runtime is required on the user’s computer.

| Computer | ZIP | First-time setup |
| --- | --- | --- |
| macOS 11+, Apple Silicon or Intel | `figma-pdf-font-service-macos-universal.zip` | Open `Install.command` once |
| Windows 10/11 x64 | `figma-pdf-font-service-windows-x64.zip` | Open `Install.cmd`, confirm with `Y` |
| Windows 11 ARM64 | `figma-pdf-font-service-windows-arm64.zip` | Open `Install.cmd`, confirm with `Y` |

Keep Figma Desktop running, then use the plugin’s font lookup. Bundled matches need no network request. Remaining faces try the local API first, then the CDN if still unresolved. The local catalog does not contain every Figma cloud font.

On macOS, launchd starts the relay on demand and it exits after 120 idle seconds. On Windows, a per-user login entry starts a listener which remains running while signed in. Run the matching `Uninstall.command` or `Uninstall.cmd` to remove this service. See the [macOS guide](local-figma-font-service/macos/README.md) and [Windows guide](local-figma-font-service/windows/README.md) for installation, ownership checks, and recovery details.

Packages are unsigned; the macOS package is not notarized. Real downloaded-package Gatekeeper, reboot, Windows install/login/uninstall, ARM64 device, and complete Figma editor flows have not yet been verified. Linux has no native package; the plugin’s CDN and manual font registration remain available.

## API and implementation

The public local endpoint is `http://localhost:38474/figma/`; upstream is fixed to `http://localhost:44950/figma/` with `Origin: https://www.figma.com`. Use **localhost**, not another hostname. Requests accept only loopback Host and missing/opaque-null Origin, allowlisted API paths, and files advertised by a validated Figma catalog. Redirects and environment proxies are disabled. Catalog responses are limited to 8 MB and font responses to 20 MB. TTF, OTF, TTC, and OTC transport is supported; the plugin matches collection members by internal metadata, with variable collections unsupported.

macOS uses Objective-C, Foundation, system libcurl, and launchd socket activation. Windows uses Go 1.27.2 and `golang.org/x/sys` with per-user lifecycle management. These are separate implementations of the same API contract. [Protocol and native development details](local-figma-font-service/README.md).

## Build and verify

```sh
# macOS, Xcode command-line tools required
bash local-figma-font-service/macos/build.sh
mkdir -p local-figma-font-service/build
xcrun clang -fobjc-arc -mmacosx-version-min=11.0 -O2 -Wall -Wextra -Werror -framework Foundation -lcurl local-figma-font-service/test.m -o local-figma-font-service/build/test-service
local-figma-font-service/build/test-service
python3 local-figma-font-service/macos/tests/lifecycle_test.py

# Go 1.27.2 and Python 3 required; produces Windows x64 + ARM64 packages
bash local-figma-font-service/windows/build.sh
(cd local-figma-font-service/windows && go test ./... && go vet ./...)
```

Builds and CI do not install services or publish releases automatically. CI validates native fixtures and Go tests, cross-builds both Windows architectures, and uploads package artifacts. Actual Windows lifecycle verification is documented in `local-figma-font-service/windows/tests/windows-smoke.ps1` and requires an explicit isolated test environment.

## Previous helper

**v0.1.0 and v0.1.1 are legacy releases** of the previous font-folder helper and are not compatible with this API-relay setup. New installations should use v0.2.0 or later. Older implementation history and tags are preserved in Git; the current source tree contains the replacement service only. The repository name is retained to keep the existing download location.

Project source is MIT licensed; Windows packages also include Go and x/sys BSD license notices. No proprietary font fixtures or plugin data are distributed here.
