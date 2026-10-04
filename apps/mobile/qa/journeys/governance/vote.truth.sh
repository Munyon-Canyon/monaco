#!/usr/bin/env bash
# The ground truth of docs/journeys/governance/vote.md: after S1, A's and B's ballots on S1's proposal
# are yes and the proposal has left open. Exits 1 when a row is wrong and 2 when it cannot be read.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
if [[ "${VOTE_TRUTH_ENV:-}" != 1 ]]; then
  VOTE_TRUTH_ENV=1 exec scripts/with-dotenv-local.sh "$0" "$@" 2> >(grep -v -e '^with-dotenv-local:' -e 'injected env' >&2)
fi

: "${MONACO_QA_HANDOFF:?journey.py sets MONACO_QA_HANDOFF}"

unreadable() {
  echo "$*" >&2
  exit 2
}

row() {
  local status=0 out
  out="$(apps/mobile/qa/journeys/psql.sh -v ON_ERROR_STOP=1 -tA -F '|' "$@")" || status=$?
  [[ $status -eq 127 ]] && unreadable "psql not found (install it, or start Compose postgres)"
  [[ $status -ne 0 ]] && unreadable "database cannot be reached"
  printf '%s' "$out"
}

proposal_id="$(python3 -c 'import json, sys; print(json.load(open(sys.argv[1])).get("S1ProposalID", ""))' \
  "$MONACO_QA_HANDOFF" 2>/dev/null || true)"
[[ -n "$proposal_id" ]] || unreadable "the hand-off has no S1ProposalID: S1's setup did not run"

IFS='|' read -r status ballot_a ballot_b <<<"$(row -v id="$proposal_id" \
  -v a="$(apps/mobile/qa/journeys/privy-user-id.sh A)" -v b="$(apps/mobile/qa/journeys/privy-user-id.sh B)" <<'SQL'
SELECT p.status,
  coalesce((SELECT v.choice FROM votes v JOIN users u ON u.id = v.voter_id
    WHERE v.proposal_id = p.id AND u.privy_user_id = :'a'), 'none'),
  coalesce((SELECT v.choice FROM votes v JOIN users u ON u.id = v.voter_id
    WHERE v.proposal_id = p.id AND u.privy_user_id = :'b'), 'none')
FROM proposals p WHERE p.id = :'id'
SQL
)"

[[ -n "${status:-}" ]] || unreadable "proposal $proposal_id has no row"

failed=0
if [[ "$ballot_a" != yes ]]; then
  echo "actor A's ballot on $proposal_id is '$ballot_a', not 'yes'"
  failed=1
fi
if [[ "$ballot_b" != yes ]]; then
  echo "actor B's ballot on $proposal_id is '$ballot_b', not 'yes'"
  failed=1
fi
if [[ "$status" == open ]]; then
  echo "proposal $proposal_id is still open after A's and B's ballots"
  failed=1
fi

[[ $failed -eq 0 ]] || exit 1
echo "A and B voted yes on $proposal_id, which is now $status"
