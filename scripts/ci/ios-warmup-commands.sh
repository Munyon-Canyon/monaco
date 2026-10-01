#!/usr/bin/env bash
# Background warm-up for the iOS job. Own DerivedData, then actool.
# Writes 0 to $RUNNER_TEMP/warmup-status only when both commands succeed.
set -euo pipefail

echo "=== warmup: xcodebuild -showsdks ==="
# -derivedDataPath is rejected unless a scheme is set. The path is not the
# build's DerivedData, so this cannot lock the real build.
xcodebuild -showsdks -project apps/mobile/Monaco.xcodeproj -scheme Monaco -derivedDataPath "$RUNNER_TEMP/warmup"
echo "=== warmup: actool ==="
xcrun actool --compile "$RUNNER_TEMP/actool-warm" --platform iphonesimulator --minimum-deployment-target 18.0 \
  apps/mobile/Monaco/Assets.xcassets
echo "warmup commands finished"
echo 0 > "$RUNNER_TEMP/warmup-status"
