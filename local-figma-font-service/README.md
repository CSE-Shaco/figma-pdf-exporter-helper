# Local Figma font service

Ready-to-extract packages: [macOS universal](macos/README.md) and [Windows x64/ARM64](windows/README.md). macOS users run `Install.command` once; no compiler or extra runtime is needed. The package installs a stable per-user executable and login-persistent socket activation. This folder's original `build.sh` remains a developer-only current-session build.

A native API relay for the PDF plugin's **Figma 글꼴 찾기** action. Bundled matches need no transport. Remaining requested faces use Figma's local API first, and unresolved faces use the pinned CDN catalog. The relay requests Figma's own API with its required Origin; it does not inject code, patch Figma, or read font folders directly. The plugin never executes a native program itself.

macOS uses launchd's socket to start the service only on demand, and exits after 120 idle seconds. Windows uses its separate per-user login listener. Both require a one-time native installation and an available Figma Desktop local API. The distributed packages are unsigned; actual downloaded-package Gatekeeper, reboot and Windows/Figma editor flows remain unverified.

## Build and verify

Requires Xcode command line tools. The build targets the current machine architecture, with minimum macOS 11.0, using Foundation and the system libcurl.

```sh
sh local-figma-font-service/build.sh
xcrun clang -fobjc-arc -mmacosx-version-min=11.0 -O2 -Wall -Wextra -Werror -framework Foundation -lcurl local-figma-font-service/test.m -o local-figma-font-service/build/test-service
local-figma-font-service/build/test-service
```

`build.sh` creates an absolute-path job plist with mode `0600`; it only builds and validates files. It never registers or starts the service. The test executable checks HTTP validation, catalog schema, allowed font paths, and body limits without registering a launchd job or calling Figma.

For a reviewed development registration in the current GUI session only:

```sh
launchctl bootstrap "gui/$(id -u)" "$PWD/local-figma-font-service/build/dev.figmapdf.local-font-service.plist"
```

The job label is `dev.figmapdf.local-font-service`. Do not also run the old Node relay on port 38474. Registration reserves the socket; the first plugin request starts the executable. To remove the current-session job:

```sh
launchctl bootout "gui/$(id -u)/dev.figmapdf.local-font-service"
```

For this developer registration only, no copy into `~/Library/LaunchAgents`, root privilege, `RunAtLoad`, or persistent login registration is used. The separate macOS package installs a persistent user LaunchAgent. Re-register after logout or reboot, and after moving this checkout rebuild the absolute paths before registration. The process exits after 120 seconds without activity; launchd retains the socket for the next demand start.

## HTTP contract and bounds

The externally visible hostname is **localhost**, including the request Host header. The launchd socket is bound internally to IPv4 loopback. The executable only accepts the inherited `FontHTTP` socket at loopback port 38474 and has no standalone bind fallback. Socket stream type is mandatory; where supported, `SO_ACCEPTCONN` must confirm a listener. macOS can return `ENOPROTOOPT` for that optional query even on a listening TCP socket, so this specific unsupported-option result is accepted while retaining launchd inheritance and endpoint/type validation.

- `GET /figma/font-files?freetype_minimum_api_version=20&isolate=true&` returns the validated Figma catalog.
- `GET /figma/font-file?file=<percent-encoded-path>` returns bytes only for a valid `.ttf`, `.otf`, `.ttc` or `.otc` path included in the last successfully validated catalog in this process.
- `GET /health` returns a service identifier, activation kind, PID, session identifier, start time and idle timeout. It includes no local font paths or file bytes.
- `OPTIONS` supports `GET` with no requested custom headers; it responds with `Access-Control-Allow-Origin: null`, private-network approval, and a 600-second preflight cache lifetime. A missing Origin is permitted for local diagnostics; otherwise only the opaque `null` origin is permitted.

Upstream requests go only to `http://localhost:44950/figma/` with `Origin: https://www.figma.com`. Redirects, environment proxies and netrc credentials are disabled. There are no direct font filesystem reads, arbitrary upstream URLs, shell commands, or child processes. Catalogs are limited to 8,000,000 bytes, 50,000 file entries and 256 faces per entry; font bytes are limited to 20,000,000. Every face must have a string family. Paths must be absolute, lack control characters, URL syntax and traversal segments, and have a supported font extension.

Upstream calls time out after 12 seconds (3 seconds to connect). Incoming headers have an 8192-byte limit and a 5-second deadline. Responses have a 5-second send deadline and exact byte Content-Length, including binary fonts. Error replies and stderr use fixed error codes without paths or private bytes.

The server handles clients sequentially; this matches the plugin's bounded font-download flow. Simultaneous plugin instances can queue behind a slow upstream request. A fresh service process has an empty allowlist: clients must fetch the catalog before fetching a font. The current explicit lookup flow fetches one catalog per lookup click, then lazily downloads only exact candidate files needed by unresolved faces; collection members are matched by their internal names. Product Pro access is checked by the plugin before the client lookup; this local transport has no separate authentication code.

## Implementation references

The bounded libcurl callback returns fewer bytes to abort an oversized transfer, following [CURLOPT_WRITEFUNCTION](https://curl.se/libcurl/c/CURLOPT_WRITEFUNCTION.html). Redirects remain disabled through [CURLOPT_FOLLOWLOCATION](https://curl.se/libcurl/c/CURLOPT_FOLLOWLOCATION.html).
