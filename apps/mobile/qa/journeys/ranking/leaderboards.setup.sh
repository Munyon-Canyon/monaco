#!/usr/bin/env bash
set -euo pipefail

scenario="${1:?journey.py passes the scenario id}"
handoff="${MONACO_QA_HANDOFF:?journey.py sets MONACO_QA_HANDOFF}"
# shellcheck source=/dev/null
source "$(git rev-parse --show-toplevel)/scripts/qa/seed.sh"
qa_api_ready

case "$scenario" in
  S1) ;;
  S2 | S3) exit 0 ;;
  *)
    echo "no setup for scenario $scenario" >&2
    exit 1
    ;;
esac

did="$(_qa_account A privy_user_id)"
qa_sql -v did="$did" >/dev/null \
  <<<"UPDATE users SET auth_state = 'ONBOARDING_COMPLETED', auth_state_changed_at = now() WHERE privy_user_id = :'did' AND auth_state <> 'ONBOARDING_COMPLETED'"
qa_api A PATCH /v1/me "{\"display_name\":\"$(_qa_account A name)\"}" >/dev/null
me="$(qa_user_id A)"

# F19aResolveCallsOK seeds eight voters with 5, 8, 7, 6, 5, 5, 5 and 2 resolved
# votes this month under fresh users. A takes over the seventh voter's votes
# (7 resolved, 4 right, below the top three) and user_7 stays the one under the
# five-vote floor. Nothing else in the seed is touched.
seed="$(qa_flow_seed 19a ok)"
read -r voter unranked < <(python3 -c 'import json,sys; ids=json.load(sys.stdin)["ids"]; print(ids["user_2"], ids["user_7"])' <<<"$seed")
qa_sql -v me="$me" -v voter="$voter" >/dev/null \
  <<<"UPDATE call_voters SET user_id = :'me' WHERE user_id = :'voter'"

python3 - "$handoff" "$me" "$unranked" <<'PY'
import json, pathlib, sys
path = pathlib.Path(sys.argv[1])
values = json.loads(path.read_text()) if path.exists() and path.read_text().strip() else {}
values.update(meID=sys.argv[2], unrankedID=sys.argv[3])
path.write_text(json.dumps(values))
PY
echo "seeded: F19aResolveCallsOK, A holds the votes of its third voter, $unranked has two resolved votes"
