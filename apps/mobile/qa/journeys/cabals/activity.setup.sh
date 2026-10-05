#!/usr/bin/env bash
set -euo pipefail

run="${MONACO_QA_RUN:?journey.py sets MONACO_QA_RUN}"
scenario="${1:?journey.py passes the scenario id}"
handoff="${MONACO_QA_HANDOFF:?journey.py sets MONACO_QA_HANDOFF}"
# shellcheck source=/dev/null
source "$(git rev-parse --show-toplevel)/scripts/qa/seed.sh"
qa_api_ready

case "$scenario" in
  S1 | S2) ;;
  *)
    echo "no setup for scenario $scenario" >&2
    exit 1
    ;;
esac

name="QA activity $run"
cabal="$(qa_sql -v name="$name" <<<"SELECT id FROM cabals WHERE name = :'name'")"
if [[ -z "$cabal" ]]; then
  cabal="$(qa_api A POST /v1/cabals \
    "{\"name\":\"$name\",\"join_mode\":\"open\",\"voter_mode\":\"all\",\"threshold\":\"majority\",\"proposal_expiry_seconds\":86400}" |
    python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')"
  # No route writes cabal_activity without a real swap (flow 11 has no seeder), so the rows are SQL.
  qa_sql -v cabal="$cabal" -v actor="$(qa_user_id A)" -v apple="XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp" >/dev/null <<'SQL'
INSERT INTO cabal_activity (id, cabal_id, kind, status, actor_user_id, asset, usdc_micros, units, tx_signature, occurred_at, updated_at)
SELECT gen_random_uuid(), :'cabal', 'fund', 'confirmed', :'actor', NULL, 5000000, NULL, NULL,
  now() - make_interval(mins => 60 - n), now()
FROM generate_series(1, 4) AS n;
INSERT INTO cabal_activity (id, cabal_id, kind, status, actor_user_id, asset, usdc_micros, units, tx_signature, occurred_at, updated_at)
VALUES
  (gen_random_uuid(), :'cabal', 'buy', 'confirmed', :'actor', :'apple', 25000000, 105000000,
   '5VERv8NMvzbJMEkV8xnrLkEaWRtSz9CosKDYjCJjBRnbJLgp8uirBgmQpjKhoR4tjF3ZpRzrFmBV6UjKdiSZkQUW',
   now() - interval '10 minutes', now()),
  (gen_random_uuid(), :'cabal', 'buy', 'failed', :'actor', :'apple', 10000000, NULL, NULL,
   now() - interval '1 minute', now());
SQL
fi
trade() {
  qa_sql -v cabal="$cabal" -v status="$1" <<<"SELECT id FROM cabal_activity WHERE cabal_id = :'cabal' AND kind = 'buy' AND status = :'status'"
}
confirmed="$(trade confirmed)"
failed="$(trade failed)"

python3 - "$handoff" "$confirmed" "$failed" <<'PY'
import json, os, sys
path, confirmed, failed = sys.argv[1:]
values = {}
if os.path.exists(path) and os.path.getsize(path):
    values = json.load(open(path))
values.update(confirmed_trade=confirmed, failed_trade=failed)
json.dump(values, open(path, "w"))
PY
echo "seeded: '$name' ($cabal) has four fund rows, a confirmed buy and a failed buy"
