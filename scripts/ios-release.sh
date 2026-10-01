#!/usr/bin/env bash
# Archive the iOS app and upload it to TestFlight.
# Usage, from the repo root: scripts/ios-release.sh staging|production
# Secrets come from .env.local through scripts/with-dotenv-local.sh.
# There is no just recipe: release stays this script.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Set by decode_asc_key and main. Global so the EXIT trap can see them after those functions return.
asc_key_path=""
export_dir=""

usage() {
  echo "usage: scripts/ios-release.sh staging|production" >&2
}

refuse_dirty_tree() {
  if [[ -z "$(git status --porcelain)" ]]; then
    return 0
  fi
  echo "error: dirty tree, refusing to archive:" >&2
  git status --porcelain >&2
  exit 1
}

# ${!name+x} is set (including empty) under bash 3.2 with nounset.
asc_var_explicitly_empty() {
  local name="$1"
  [[ "${!name+x}" == x && -z "${!name}" ]]
}

refuse_empty_asc_vars() {
  local name
  for name in ASC_KEY_ID ASC_ISSUER_ID ASC_KEY_P8_BASE64; do
    if [[ -z "${!name:-}" ]]; then
      echo "error: ${name} is empty" >&2
      exit 1
    fi
  done
}

refuse_unpushed_head() {
  if [[ -n "$(git branch -r --contains HEAD)" ]]; then
    return 0
  fi
  echo "error: HEAD is not on any remote branch. Push it before releasing." >&2
  git rev-parse HEAD >&2
  exit 1
}

cleanup_asc_key() {
  if [[ -n "${asc_key_path}" ]]; then
    rm -f "${asc_key_path}"
    asc_key_path=""
  fi
  if [[ -n "${export_dir}" ]]; then
    rm -rf "${export_dir}"
    export_dir=""
  fi
}

# Decode ASC_KEY_P8_BASE64 into a mode-600 file. The EXIT trap removes it.
decode_asc_key() {
  trap cleanup_asc_key EXIT
  asc_key_path="$(mktemp "${TMPDIR:-/tmp}/monaco-asc-key.XXXXXX")"
  chmod 600 "${asc_key_path}"
  ASC_KEY_PATH="${asc_key_path}" python3 -c 'import base64, os
open(os.environ["ASC_KEY_PATH"], "wb").write(base64.b64decode(os.environ["ASC_KEY_P8_BASE64"]))'
}

# Stop before upload when the archived Info.plist does not match this release.
# plutil is a macOS tool invoked by absolute path so the scripts tool manifest,
# which only knows binaries from just install, does not flag it.
check_archived_plist() {
  local plist="$1"
  local want_version="$2"
  local want_env="$3"
  local dump got_version got_env got_url
  dump="$(/usr/bin/plutil -p "$plist")"
  got_version="$(printf '%s\n' "$dump" | awk -F ' => ' '/"CFBundleVersion"/ { gsub(/"/, "", $2); print $2; exit }')"
  got_env="$(printf '%s\n' "$dump" | awk -F ' => ' '/"MONACO_ENVIRONMENT"/ { gsub(/"/, "", $2); print $2; exit }')"
  got_url="$(printf '%s\n' "$dump" | awk -F ' => ' '/"MONACO_API_BASE_URL"/ { gsub(/"/, "", $2); print $2; exit }')"
  if [[ "$got_version" != "$want_version" || "$got_env" != "$want_env" ]]; then
    echo "error: archive Info.plist has CFBundleVersion=${got_version:-missing} MONACO_ENVIRONMENT=${got_env:-missing}, want CFBundleVersion=${want_version} MONACO_ENVIRONMENT=${want_env}" >&2
    return 1
  fi
  if [[ "$got_url" != https://?* ]]; then
    echo "error: archive Info.plist MONACO_API_BASE_URL must be an https:// URL, got ${got_url:-missing}" >&2
    return 1
  fi
}

run_xcodebuild() {
  if [[ -x "${repo_root}/.bin/xcsift" ]]; then
    "${repo_root}/scripts/qa/xcode-lock.sh" "$@" 2>&1 | "${repo_root}/.bin/xcsift"
  else
    "${repo_root}/scripts/qa/xcode-lock.sh" "$@"
  fi
}

main() {
  local env_name name build_number day archive_dir archive plist tag commit url_var
  cd "$repo_root"

  if [[ $# -ne 1 ]]; then
    usage
    exit 1
  fi
  env_name="$1"
  case "$env_name" in
    staging|production) ;;
    *)
      usage
      exit 1
      ;;
  esac

  [[ "$(uname)" == Darwin ]] || { echo "error: macOS only" >&2; exit 1; }

  refuse_dirty_tree

  # An explicit empty value fails before dotenv, so a planted missing-key run
  # does not decrypt .env.local. Unset means the value is loaded from the file.
  if [[ "${MONACO_IOS_RELEASE_WRAPPED:-}" != 1 ]]; then
    for name in ASC_KEY_ID ASC_ISSUER_ID ASC_KEY_P8_BASE64; do
      if asc_var_explicitly_empty "$name"; then
        echo "error: ${name} is empty" >&2
        exit 1
      fi
    done
    refuse_unpushed_head
    exec "${repo_root}/scripts/with-dotenv-local.sh" env MONACO_IOS_RELEASE_WRAPPED=1 "${repo_root}/scripts/ios-release.sh" "$@"
  fi

  refuse_unpushed_head
  refuse_empty_asc_vars

  case "$env_name" in
    staging) url_var=MONACO_STAGING_API_BASE_URL ;;
    production) url_var=MONACO_PRODUCTION_API_BASE_URL ;;
  esac
  if [[ "${!url_var:-}" != https://?* ]]; then
    echo "error: ${url_var} must be an https:// URL for a ${env_name} release" >&2
    exit 1
  fi

  if ! build_number="$(git rev-list --count HEAD)"; then
    echo "error: could not count commits for the build number" >&2
    exit 1
  fi
  decode_asc_key
  "${repo_root}/scripts/ensure-ios-privy-config.sh" generate

  day="$(date +%Y-%m-%d)"
  archive_dir="${HOME}/Library/Developer/Xcode/Archives/${day}"
  mkdir -p "$archive_dir"
  archive="${archive_dir}/Monaco-${env_name}-${build_number}.xcarchive"

  echo "archiving ${archive} (build ${build_number}, ${env_name})"
  run_xcodebuild xcodebuild archive -project apps/mobile/Monaco.xcodeproj -scheme Monaco -configuration Release -destination 'generic/platform=iOS' -archivePath "$archive" "MONACO_ENVIRONMENT=${env_name}" "CURRENT_PROJECT_VERSION=${build_number}" -authenticationKeyPath "$asc_key_path" -authenticationKeyID "$ASC_KEY_ID" -authenticationKeyIssuerID "$ASC_ISSUER_ID" -allowProvisioningUpdates

  plist="${archive}/Products/Applications/Monaco.app/Info.plist"
  check_archived_plist "$plist" "$build_number" "$env_name"

  export_dir="$(mktemp -d "${TMPDIR:-/tmp}/monaco-ios-export.XXXXXX")"
  echo "uploading ${archive}"
  run_xcodebuild xcodebuild -exportArchive -archivePath "$archive" -exportOptionsPlist apps/mobile/ExportOptions-testflight.plist -exportPath "$export_dir" -authenticationKeyPath "$asc_key_path" -authenticationKeyID "$ASC_KEY_ID" -authenticationKeyIssuerID "$ASC_ISSUER_ID" -allowProvisioningUpdates

  tag="ios/${env_name}/${build_number}"
  git tag "$tag" HEAD
  git push origin "$tag"
  commit="$(git rev-parse HEAD)"
  printf 'build %s\ncommit %s\ntag %s\n' "$build_number" "$commit" "$tag"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
