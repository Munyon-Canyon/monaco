#!/usr/bin/env bash
# Select the Xcode that .xcode-version pins. Runners name that app
# /Applications/Xcode_<version>.app. `xcodes install` writes
# /Applications/Xcode-<version>.app, so this script does not select a laptop install.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
xcode="/Applications/Xcode_$(cat "$root/.xcode-version").app"
[[ -d "$xcode" ]] || { echo "no Xcode at '${xcode}'" >&2; ls -d /Applications/Xcode* >&2; exit 1; }

sudo xcode-select -s "$xcode/Contents/Developer"
xcodebuild -version
