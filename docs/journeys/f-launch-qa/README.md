# F launch journey QA

One pass of the app journeys on the `staging` tip for the F launch (phase 1), from #7016 on 2026-10-29. The pass started on 470716219 and ran `scripts/qa/journey.py run <journey> --runs 1` against a local backend. The operator capped the pass to land within the hour: the rest is #7077.

#7077 re-ran the journeys on the staging tip on 2026-10-29. It ran two streams on backend slots 0 and 1, one simulator each, and restacked onto each app fix as it landed. The journeys that still stop are in #7173. Logs are under `.logs/qa/journeys/<run>/` in the 7077 lane.

Results use the step ids of each journey doc. A test that drifted from the app was fixed in #7084, the first PR of #7016.

## Results

| Journey | Result | Ticket |
| --- | --- | --- |
| auth/sign-in | PASS (S1–S3) | |
| cabals/activity | FAIL at S1.2: no Treasury history row on a $0 pot | #7036 |
| cabals/approve-request | S1.1–S1.5 PASS. FAIL at S1.6: the "Denied." toast is not seen (test or app, not triaged) | #7173 |
| cabals/browse | S1 PASS on an earlier run. FAIL at S1.1 on the last run: no `cabals-new-tile` once A owns 20+ cabals (test) | #7173 |
| cabals/create-cabal | S1 PASS. FAIL at S2.6: the new cabal is not found in the 20+ tile grid (test) | #7173 |
| cabals/discover | PASS (S1–S2) | |
| cabals/edit-rules | S1 PASS. FAIL at S2.1: the proposal sheet does not open after the show-sides toggle (not triaged) | #7173 |
| cabals/info-and-members | PASS (S1–S2) |  |
| cabals/invite | Not run: setup stopped because B signed in as @dexter, not @qa_b, even with no other journey running | #7173 |
| cabals/join | S1.1–S1.7 PASS. FAIL at S1.8: the toast is not seen (test or app, not triaged) | #7173 |
| cabals/leave | PASS (S1–S2) | |
| cabals/pause | PASS (S1–S2) | |
| cabals/pot | PASS (S1–S3) | |
| chat/cabal-chat | PASS (S1) | |
| chat/talk-it-over | PASS (S1–S5) after #7164 and #7157 | #7157 |
| demo/story | Needs funding (A 3, B 1), not run | |
| feed/browse | PASS (S1–S3) | |
| feed/react-and-reply | PASS (S1–S3) after #7146 | #7146 |
| governance/comments | S1.1 PASS. FAIL at S1.2: no Replies header on an empty thread | #7166 |
| governance/propose-buy | Needs funding (A 2, B 1): stops at the fund step | |
| governance/propose-from-asset | PASS (S1) | |
| governance/propose-sell | FAIL at S1.2: the cabal pot returns 400 on a 16-digit multiplier, so Sell never loads | #7053 |
| governance/vote | S1.1–S1.7 PASS. FAIL at S1.8: `proposal-vote-yes` does not read "Voted Yes, 2 votes" (not triaged) | #7173 |
| governance/withdraw | PASS (S1) | |
| home/activity | PASS (S1) | #7109 |
| home/dashboard | S1.1 PASS after #7154. Last two runs stopped in setup: `GET /v1/me/portfolio` answered 503 `rpc_unavailable` (Solana RPC) | #7154, #7173 |
| money/activity | S1 PASS. S2 known failure: needs a funded setup (A 1) | |
| money/best-price-trade | Needs funding (A 2): stops at the fund step | |
| money/cash-out | Needs funding (A 2), not run | |
| money/deposit | Needs funding (A 1): S1.1 balance under $1.00 | |
| money/fund-cabal | S1 stops at S1.5, the fund step: A has $0.00. Needs funding (A 2) | |
| money/withdraw | Needs funding (A 1, B 1), not run | |
| onboarding/first-run | FAIL at S1.6: the onboarding nudge is drawn nowhere | #7042 |
| profile/edit | PASS (S1–S4) | |
| profile/find-friends | PASS (S1–S3) | |
| profile/follow-lists | PASS (S1–S3) after #7150 | #7150 |
| profile/invite | PASS (S1–S2) | |
| profile/nudge | PASS (S1–S4) | |
| profile/overview | S1–S6 PASS. FAIL at S7.1: "P&L · 1M" reads "unavailable" (not triaged) | #7173 |
| profile/user-profile | PASS (S1–S5) after #7153 | #7153 |
| ranking/leaderboards | PASS (S1–S3) | |
| settings/delete-account | PASS (S1–S3), run with no other journey running |  |
| settings/notifications | PASS (S1–S4) after #7156 and #7150 | #7156, #7150 |
| social/follow-and-boards | PASS (S1–S5) after #7150 | #7150 |
| stocks/asset-detail | S1.1–S1.8 PASS after #7155. FAIL at S1.9: the cabal row does not open the Amount screen (test) | #7155, #7173 |
| stocks/browse | PASS (S1–S3) |  |
| stocks/watchlist | PASS (S1–S2) after #7155 | #7155 |

## Mutants

Mutants ran on the F launch journeys first, then on the other journeys that passed.

| Journey | Mutants |
| --- | --- |
| home/activity | 2/2 caught |
| ranking/leaderboards | 1/1 caught. One mutant was removed: it changed the spoken copy of unranked rows, which no longer render for a ranked viewer by #6970's design |
| social/follow-and-boards | 5/5 caught |
| feed/react-and-reply | 3/3 caught. `gif-reply-unquoted` was missed on the first run because its patch edited the wrong branch; with the patch fixed it fails S2.3 |

Mutants for the other journeys that pass (profile/find-friends, profile/invite, profile/nudge, profile/user-profile, profile/follow-lists, cabals/pot, cabals/leave, cabals/info-and-members, feed/browse, governance/withdraw, governance/propose-from-asset, stocks/browse, stocks/watchlist, settings/notifications, chat/talk-it-over) did not run in #7077 and move to #7173.

## Needs funding

These journeys move real USDC, and the pass funded no actor: demo/story, governance/propose-buy, money/best-price-trade, money/cash-out, money/deposit, money/withdraw, money/activity S2, and money/fund-cabal past its fund step (S1.5, A 2). Run them after funding each actor from the QA pot with `monacoctl qa fund` (`docs/journeys/README.md`, Journeys that move money).

## What the pass taught

- Most failures were stale tests from the F rebuild, not app defects. Controls became menus, rows became combined or contained accessibility elements, and Feed cards now open the post.
- The #7077 failures were test drift from the F rebuild again: ids moved to `.contain` containers, child texts replaced row labels, Find friends sits under Settings, Discover is behind the Cabals `+` menu, non-members see a read-only vote card, and the Proposal screen opens from a Feed proposal cell.
- A recurring app defect class is a container `.accessibilityIdentifier` without `.accessibilityElement(children: .contain)`, which replaces the child ids (#7109, #7150, #7153).
- A setup that writes ranking rows by SQL must insert a `call_board_triggers` row and wait for the run, or the board reads stale.
- Leftovers on the shared database break "empty" expectations: A owns 20+ cabals, holds AAPLx, and has old Catch up items, votes and follows. Setups now clear them or scroll past them.
- The multi-journey `journey.py run a b c` deadlocked on sim locks until #7171.
- `onboarding/first-run` signs C in by email. Every other journey signs C in by sms, and Privy treats the two as different users. Keep C's `privy_user_id` in `apps/mobile/qa/journeys/accounts.tsv` on the sms user.
- When two runs share a Mac, `journey.py` lends a busy actor's login to another run. `cabals/invite` (B must be `@qa_b`) and `settings/delete-account` need their own logins, so run them alone.
