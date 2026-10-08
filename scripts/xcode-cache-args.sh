#!/usr/bin/env bash
# Prints the xcodebuild arguments that share one compilation cache and one SourcePackages
# checkout across every worktree of this clone, one argument per line. Read them into an
# array with: while IFS= read -r a; do args+=("$a"); done < <(scripts/xcode-cache-args.sh)
# Optional $1 is the -derivedDataPath the caller uses (default: <worktree>/.build/DerivedData).
# These stay command-line settings: an xcconfig does not reach the Swift package targets.
# Prefix mapping makes the worktree and DerivedData paths the same in every cache key, so a
# second worktree hits the first one's entries. SourcePackages is keyed by a hash of
# Package.resolved so lanes on different package versions stay apart.
# See docs/how-to/local-simulator.md#shared-compilation-cache.
set -euo pipefail

wt=$(git rev-parse --show-toplevel)
cd "$wt"
primary=$(dirname "$(git rev-parse --path-format=absolute --git-common-dir)")
dd=${1:-$wt/.build/DerivedData}
pins=$(shasum apps/mobile/Monaco.xcodeproj/project.xcworkspace/xcshareddata/swiftpm/Package.resolved | cut -c1-12)

printf '%s\n' \
  -clonedSourcePackagesDirPath "$primary/.build/SourcePackages/$pins" \
  -onlyUsePackageVersionsFromResolvedFile \
  COMPILATION_CACHE_ENABLE_CACHING=YES \
  "COMPILATION_CACHE_CAS_PATH=$primary/.build/CompilationCache" \
  SWIFT_ENABLE_PREFIX_MAPPING=YES CLANG_ENABLE_PREFIX_MAPPING=YES \
  SWIFT_ENABLE_PROJECT_PREFIX_MAPPING=YES CLANG_ENABLE_PROJECT_PREFIX_MAPPING=YES \
  "SWIFT_OTHER_PREFIX_MAPPINGS=$dd=/^dd $wt=/^wt" \
  "CLANG_OTHER_PREFIX_MAPPINGS=$dd=/^dd $wt=/^wt" \
  ONLY_ACTIVE_ARCH=YES COMPILER_INDEX_STORE_ENABLE=NO
