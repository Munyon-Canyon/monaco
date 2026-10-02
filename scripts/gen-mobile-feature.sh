#!/usr/bin/env bash
# Copy the system-ping reference into a new UpperCamelCase domain.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
domain="${1:-}"

if [[ ! "$domain" =~ ^[A-Z][A-Za-z0-9]*$ ]]; then
  echo "gen-mobile-feature: name must be UpperCamelCase" >&2
  exit 1
fi

camel="$(printf '%s' "$domain" | awk '{ print tolower(substr($0,1,1)) substr($0,2) }')"

model_dir="$root/packages/mobile-core/Sources/MonacoCore/$domain"
test_file="$root/packages/mobile-core/Tests/MonacoCoreTests/${domain}ModelTests.swift"
view_dir="$root/apps/mobile/Monaco/Features/$domain"
fixture="$root/packages/mobile-core/Sources/MonacoAPI/Fixtures/${domain}+Sample.swift"

if [[ -e "$model_dir" || -e "$test_file" || -e "$view_dir" || -e "$fixture" ]]; then
  echo "gen-mobile-feature: $domain already exists" >&2
  exit 1
fi

render() {
  local src=$1 dst=$2
  mkdir -p "$(dirname "$dst")"
  # Keep getSystemPing and postSystemPing so the copy still compiles against the ping operations.
  sed \
    -e 's/getSystemPing/__GET_PING__/g' \
    -e 's/postSystemPing/__POST_PING__/g' \
    -e "s/SystemPing/${domain}/g" \
    -e "s/systemPing/${camel}/g" \
    -e 's/__GET_PING__/getSystemPing/g' \
    -e 's/__POST_PING__/postSystemPing/g' \
    "$src" > "$dst"
}

render "$root/packages/mobile-core/Sources/MonacoCore/SystemPing/SystemPingModel.swift" \
  "$model_dir/${domain}Model.swift"
render "$root/packages/mobile-core/Tests/MonacoCoreTests/SystemPingModelTests.swift" \
  "$test_file"
render "$root/apps/mobile/Monaco/Features/SystemPing/SystemPingView.swift" \
  "$view_dir/${domain}View.swift"
render "$root/apps/mobile/Monaco/Features/SystemPing/SystemPingRoute.swift" \
  "$view_dir/${domain}Route.swift"
render "$root/apps/mobile/Monaco/Features/SystemPing/SystemPingSampleHarness.swift" \
  "$view_dir/${domain}SampleHarness.swift"
render "$root/packages/mobile-core/Sources/MonacoAPI/Fixtures/SystemPing+Sample.swift" \
  "$fixture"

cat >> "$test_file" << EOF

@MainActor
private func ${camel}GeneratorContract(_ model: ${domain}Model) async {
    await model.send(note: "hi")
    await model.load()
    await model.observe()
}
EOF

echo "gen-mobile-feature: wrote $domain"
