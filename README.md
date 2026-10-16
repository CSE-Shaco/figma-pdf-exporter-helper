# Figma PDF Exporter Local Font Helper

Local font helper downloads and installation guide for Figma PDF Exporter.

This repository will host the helper download and setup instructions. **No customer download or published release is available yet.** Do not treat a future release link as available until a release is published here.

## Planned setup

When a release is available, choose the archive for your operating system, download it from this repository's Releases page, extract it, and start the helper with the operating-system-specific command included in the archive:

- macOS: `start-macos.command`
- Windows: `start-windows.cmd`

The helper displays a 64-digit pairing code. Enter that code in the Local Fonts area of PDF Exporter Pro or Preview, then use the plugin to search fonts installed on your computer.

The helper is designed to listen on the local loopback interface. Font search requests stay on the local machine; the plugin sends text requests rather than font files. Pairing uses a private token, and the existing font access checks remain in effect.

## Availability and limitations

- A macOS Apple Silicon helper exists, but it is currently unsigned and has not been notarized.
- An Intel Mac build is not ready.
- A Windows PowerShell helper exists, but a packaged executable has not been built or validated.
- Creative Cloud activated Adobe Fonts have not been verified with the helper. Adobe Fonts availability is not guaranteed; see the product documentation for current compatibility details.

These limitations may change. Check the release notes before installing a published build.

## Support

Please use this repository's Issues for helper installation and startup problems. Do not include pairing codes, access tokens, font files, or other private data in an issue.
