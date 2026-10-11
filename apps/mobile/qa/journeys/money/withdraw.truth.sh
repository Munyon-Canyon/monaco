#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=/dev/null
source "$(git rev-parse --show-toplevel)/scripts/qa/seed.sh"
address="${MONACO_QA_REFUND_ADDRESS:?journey.py passes MONACO_QA_REFUND_ADDRESS to a funds journey}"
# S2 checks a Base withdrawal to this address up to Confirm and never sends it.
base_address="0x000000000000000000000000000000000000dEaD"

table="$(qa_sql <<<"SELECT to_regclass('public.withdrawals') IS NOT NULL")" ||
  { echo "database cannot be reached" >&2; exit 2; }
if [[ "$table" != t ]]; then
  echo "ok: no withdrawals table until #652, nothing to read back"
  exit 0
fi

fail=0
for actor in A B; do
  did="$(_qa_account "$actor" privy_user_id)"
  sent="$(qa_sql -v did="$did" -v address="$address" <<<"SELECT count(*) FROM withdrawals w JOIN users u ON u.id = w.user_id
    WHERE u.privy_user_id = :'did' AND w.to_address = :'address' AND w.created_at > now() - interval '15 minutes'")"
  if [[ "$sent" -ge 1 ]]; then
    echo "ok: $actor withdrew to $address"
  else
    echo "$actor has no withdrawal to $address in the last 15 minutes"
    fail=1
  fi
done

b="$(_qa_account B privy_user_id)"
base="$(qa_sql -v did="$b" -v address="$base_address" <<<"SELECT count(*) FROM withdrawals w JOIN users u ON u.id = w.user_id
  WHERE u.privy_user_id = :'did' AND (lower(w.to_address) = lower(:'address') OR lower(w.evm_address) = lower(:'address'))")"
if [[ "$base" == 0 ]]; then
  echo "ok: B sent nothing to $base_address"
else
  echo "B has $base withdrawals to $base_address: S2 must stop at Confirm"
  fail=1
fi
exit "$fail"
