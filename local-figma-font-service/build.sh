#!/bin/sh
set -eu
service_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
service_build="$service_dir/build"
mkdir -p "$service_build"
xcrun clang -fobjc-arc -mmacosx-version-min=11.0 -O2 -Wall -Wextra -Werror -framework Foundation -lcurl "$service_dir/service.m" -o "$service_build/figma-local-font-service"
cp "$service_dir/job.plist" "$service_build/dev.figmapdf.local-font-service.plist"
plutil -replace ProgramArguments -array "$service_build/dev.figmapdf.local-font-service.plist"
plutil -insert ProgramArguments -string "$service_build/figma-local-font-service" -append "$service_build/dev.figmapdf.local-font-service.plist"
plutil -replace StandardErrorPath -string "$service_build/service.stderr.log" "$service_build/dev.figmapdf.local-font-service.plist"
chmod 600 "$service_build/dev.figmapdf.local-font-service.plist"
plutil -lint "$service_build/dev.figmapdf.local-font-service.plist"
printf '%s\n' "$service_build/dev.figmapdf.local-font-service.plist"
printf '%s\n' 'Built only. No launchd registration, installation, or process launch performed.'
