# Changelog

## v0.1.1 — 2026-10-16

- License and documentation update only: the helper software and source are now explicitly provided under the MIT License. User fonts and third-party components retain their own terms.
- Runtime binaries and behavior are unchanged from v0.1.0.

## v0.1.0 — 2026-10-16

- First public download: native macOS ARM64, macOS x86_64 and Windows x64 ZIPs with launchers, Korean/English setup instructions, runtime notices and SHA-256 checksums.
- Authenticated loopback font matching and download; OS-exposed SFNT/TTC fonts, including extensionless activated font paths.
- Native macOS and frozen Windows executable startup, authentication, OS font discovery and downloaded-byte integrity verified in CI.
- UTF-8 startup output for redirected Windows consoles; Windows builds use the selected Python interpreter.
- No publisher signing or notarization. Actual Creative Cloud activation and Windows 10/11 desktop manual validation remain unverified.
