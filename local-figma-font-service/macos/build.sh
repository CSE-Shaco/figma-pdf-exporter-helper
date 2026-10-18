#!/bin/bash
set -euo pipefail
font_source=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
font_dist="$font_source/macos/dist"
font_name=figma-pdf-font-service-macos-universal
font_output="$font_dist/$font_name"
mkdir -p "$font_output"
xcrun clang -arch arm64 -arch x86_64 -fobjc-arc -mmacosx-version-min=11.0 -O2 -Wall -Wextra -Werror -framework Foundation -lcurl "$font_source/service.m" -o "$font_output/figma-local-font-service"
cp "$font_source/macos/Install.command" "$font_source/macos/Uninstall.command" "$font_source/macos/lifecycle.sh" "$font_source/macos/README.md" "$font_source/job.plist" "$font_source/LICENSE" "$font_output/"
chmod 755 "$font_output/figma-local-font-service" "$font_output/Install.command" "$font_output/Uninstall.command"
(cd "$font_output" && shasum -a 256 figma-local-font-service Install.command Uninstall.command lifecycle.sh README.md job.plist LICENSE > SHA256SUMS.txt)
# Explicit file list avoids accidentally adding developer artifacts on repeated builds.
rm -f "$font_dist/$font_name.zip"
(cd "$font_dist" && zip -q "$font_name.zip" "$font_name/figma-local-font-service" "$font_name/Install.command" "$font_name/Uninstall.command" "$font_name/lifecycle.sh" "$font_name/README.md" "$font_name/job.plist" "$font_name/LICENSE" "$font_name/SHA256SUMS.txt")
(cd "$font_dist" && shasum -a 256 "$font_name.zip" > SHA256SUMS.txt)
lipo -info "$font_output/figma-local-font-service"
wc -c "$font_output/figma-local-font-service" "$font_dist/$font_name.zip"
printf '%s\n' 'Built only. No installation, launch registration, or process launch performed.'
