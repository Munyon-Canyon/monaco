#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=/dev/null
source "$(git rev-parse --show-toplevel)/scripts/qa/seed.sh"
address="${MONACO_QA_REFUND_ADDRESS:?journey.py passes MONACO_QA_REFUND_ADDRESS to a funds journey}"
a="$(_qa_account A privy_user_id)"

table="$(qa_sql <<<"SELECT to_regclass('public.withdrawals') IS NOT NULL")" ||
  { echo "database cannot be reached" >&2; exit 2; }
if [[ "$table" != t ]]; then
  echo "ok: no withdrawals table until #652, nothing to read back"
  exit 0
fi

sent="$(qa_sql -v a="$a" -v address="$address" <<<"SELECT count(*) FROM withdrawals w JOIN users u ON u.id = w.user_id
  WHERE u.privy_user_id = :'a' AND w.to_address = :'address' AND w.created_at > now() - interval '15 minutes'")"
if [[ "$sent" -ge 1 ]]; then
  echo "ok: A withdrew to $address"
else
  echo "A has no withdrawal to $address in the last 15 minutes"
  exit 1
fi
