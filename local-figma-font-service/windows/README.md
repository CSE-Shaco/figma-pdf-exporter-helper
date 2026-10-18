# Windows local Figma font service

This package installs a per-user local service for the plugin's installed-font fallback. It is a native Go executable; Node.js, Go, administrator access, and a separate runtime are not required on the user's PC.

## Install

1. Extract the ZIP for your device (`x64` for Windows 10/11 x64, or `ARM64` for Windows 11 ARM64).
2. Double-click `Install.cmd` and confirm with `Y` when prompted.
3. The installer places `font-service.exe` under `%LOCALAPPDATA%\FigmaPDFExporter\FontService`, registers it to start at login, and starts it immediately. The small listener stays running while you are signed in.

Bundled matches need no request. The plugin uses this service for unresolved faces first, and only remaining faces use the CDN. CDN font lookup works independently when the service is unavailable. For the local fallback, Figma Desktop's local font API must be available. The service listens on `localhost:38474` and requests Figma's API at `localhost:44950` with `Origin: https://www.figma.com`.

## Remove

Double-click `Uninstall.cmd`, or run `font-service.exe --uninstall`. Removal stops and deletes this service's own process and login entry for the current Windows user. It does not change other programs or services.

## Developer build

The package build uses Go 1.27.2 and produces x64 and ARM64 ZIPs, SHA-256 checksums, and license notices in `dist/`:

```sh
GO_BIN=/absolute/path/to/go bash local-figma-font-service/windows/build.sh
```

The Go service also supports `--install` (install and start), `--serve` (run the listener), and `--uninstall` command-line modes. Installer updates roll back if replacement fails. A port conflict or other setup error is reported explicitly.

## Support and verification

The intended targets are Windows 10 x64 and Windows 11 ARM64. Core service and lifecycle behavior currently target Windows; the macOS Objective-C launchd service remains unchanged. Linux users have the CDN route and can manually register font files or folders; there is no Linux native service. The Windows package has not yet been smoke-tested on a real Windows installation or Figma Desktop. Windows 10/11 login, install, removal, and ARM64 device behavior therefore remain unverified; successful builds do not establish platform support verification. The PowerShell smoke script at `local-figma-font-service/windows/tests/windows-smoke.ps1` is retained in the source checkout for developer verification and has not been run. Verification records are maintained in the private product workspace; no test-output files are included in this source repository or release packages.

The listener enforces loopback Host and Figma Origin checks, API path and font catalog allowlists, 8 MB list and 20 MB font response limits, at most 64 connections, one concurrent upstream request, a five-second incoming-header timeout, a three-second upstream connect timeout, a 12-second upstream request/header timeout, and a five-second response-send timeout. It relays TTF/OTF files and catalog-listed TTC/OTC files up to 20 MB. The plugin can enumerate static collection members with its existing parser; variable collections are unsupported. The service relays only files advertised by Figma's local font API and never reads font files from disk.

The ZIP includes the project MIT `LICENSE`, `THIRD_PARTY_NOTICES.txt`, Go and `x/sys` license files, and `SHA256SUMS.txt`. The existing plugin bundle is unchanged. Packages are not signed for public distribution.
