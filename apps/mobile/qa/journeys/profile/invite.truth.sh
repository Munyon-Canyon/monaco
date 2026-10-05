#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=/dev/null
source "$(git rev-parse --show-toplevel)/scripts/qa/seed.sh"

did="$(_qa_account A privy_user_id)"
got="$(qa_sql -v did="$did" <<<"SELECT count(r.code) FROM users u LEFT JOIN referral_codes r ON r.user_id = u.id WHERE u.privy_user_id = :'did'")" ||
  { echo "database cannot be reached" >&2; exit 2; }
if [[ "$got" == 1 ]]; then
  echo "ok: A has one referral code"
else
  echo "A referral codes: got '$got', want 1"
  exit 1
fi
