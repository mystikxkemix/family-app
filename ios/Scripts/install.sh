#!/usr/bin/env bash
# Builds the app with one signing profile and installs it on an iPhone.
# Usage: Scripts/install.sh <profile> [device-id]
# The profile is Config/Signing/<profile>.xcconfig (see Config/Signing/README.md).
set -euo pipefail

cd "$(dirname "$0")/.."
profile="${1:?usage: $0 <profile> [device-id]}"
signing="Config/Signing/${profile}.xcconfig"
[ -f "$signing" ] || { echo "Missing $signing (copy Config/Signing/example.xcconfig)"; exit 1; }

xcodegen generate --quiet

build_dir="build/${profile}"
xcodebuild build \
  -project FamilyApp.xcodeproj -scheme FamilyApp -configuration Debug \
  -destination 'generic/platform=iOS' \
  -xcconfig "$signing" \
  -derivedDataPath "$build_dir" \
  -allowProvisioningUpdates \
  -quiet
app="${build_dir}/Build/Products/Debug-iphoneos/FamilyApp.app"

device="${2:-}"
if [ -z "$device" ]; then
  # First connected iPhone.
  device=$(xcrun devicectl list devices --hide-default-columns --columns Identifier --filter "State == 'connected' AND Platform == 'iOS'" 2>/dev/null | awk 'NR>2 && NF {print $1; exit}')
fi
[ -n "$device" ] || { echo "No connected iPhone found; pass a device id (xcrun devicectl list devices)"; exit 1; }

xcrun devicectl device install app --device "$device" "$app"
echo "Installed on $device with profile ${profile}"
