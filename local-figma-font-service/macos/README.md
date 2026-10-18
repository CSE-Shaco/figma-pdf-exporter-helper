# macOS Figma API bridge (universal)

Supports Apple Silicon and Intel Macs running macOS 11 or later. Extract the ZIP, then open `Install.command` once in Finder. This copies a small native executable into this user's Application Support directory and registers a persistent per-user LaunchAgent. No administrator password, Node.js, Python, Xcode, font folder selection, or additional runtime download is required. Figma Desktop and its local font API must be running.

After installation, use the plugin's Pro Figma font lookup. Bundled matches need no request; remaining faces try the local API first, then the CDN only if still unresolved. Requests go to `http://localhost:38474/figma/`, and the bridge requests the existing Figma API at `http://localhost:44950/figma/`. It supplies the required Figma Origin using native networking. This is an API relay, not code injection, a Figma patch, or direct reading of font directories. Only catalog-listed TTF/OTF/TTC/OTC bytes needed by the plugin are returned; collection members are matched by their actual internal font metadata, and the same 20MB per-file limit applies. The API's catalog does not include every Figma cloud font.

The executable is started on demand by launchd, exits after 120 idle seconds, and starts again on the next request. The registration persists after logout/restart. You may move or delete the extracted ZIP directory after installation; the installed executable has a stable path. Keep a copy of `Uninstall.command` and `lifecycle.sh`, or extract the ZIP again to uninstall.

Installed paths:

- `~/Library/Application Support/FigmaPDFExporter/FontService/`
- `~/Library/LaunchAgents/dev.figmapdf.local-font-service.plist`

Run `Uninstall.command` to unregister only this owned job and remove its owned executable, marker, plist and regular log. Unknown files are preserved. Reinstalling updates the same owned installation. A foreign or development job with the same label is refused; remove that earlier job explicitly first. A failed launch registration restores prior installation files.

For an existing development registration from this repository only, review its executable with `launchctl print "gui/$(id -u)/dev.figmapdf.local-font-service"`, then remove that known job with `launchctl bootout "gui/$(id -u)/dev.figmapdf.local-font-service"` before installing this package. The installer does not stop an unknown job. Concurrent installation/uninstallation is refused using an exclusive lock directory. If an installation is forcibly interrupted, review the preserved `.font-stage.*` backup and `.font-install.lock` under `~/Library/Application Support/FigmaPDFExporter` before retrying. A rollback error reports the backup path and never deletes un-restored backup files.

This package is unsigned and not notarized. macOS may block files received from the Internet; use your normal macOS security review process. The build and isolated lifecycle tests do not prove a downloaded-package Gatekeeper launch, reboot, or actual Figma editor flow. No installation is performed by the package build script.

For developers: run `bash local-figma-font-service/macos/build.sh` with Xcode command line tools. The ZIP contains only the universal binary, three shell files, a job template, this README, the MIT license and checksums. System Foundation and libcurl are the only linked libraries beyond the standard system libraries. Windows has its own separate package; Linux has no native bridge package.
