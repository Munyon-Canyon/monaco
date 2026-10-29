# F launch journey QA

One pass of the app journeys on the `staging` tip for the F launch (phase 1), from #7016 on 2026-10-29. The pass started on 470716219 and ran `scripts/qa/journey.py run <journey> --runs 1` against a local backend. The operator capped the pass to land within the hour: the rest is #7077.

Results use the step ids of each journey doc. A test that drifted from the app was fixed in #7084, the first PR of #7016. Runs after that fix are noted. Logs are under `.logs/qa/journeys/<run>/` in the lane worktree.

## Results

| Journey | Result | Ticket |
| --- | --- | --- |
| auth/sign-in | PASS (S1–S3) | |
| cabals/activity | FAIL at S1.2: no Treasury history row on a $0 pot | #7036 |
| cabals/approve-request | Not run: setup stopped on runner login state | #7077 |
| cabals/browse | S1 PASS. S2.1 failed on a stale step, fixed in #7084, not re-run | #7077 |
| cabals/create-cabal | FAIL at S1.2 on a stale step (the `+` is a menu), fixed in #7084, not re-run | #7077 |
| cabals/discover | PASS (S1–S2) | |
| cabals/edit-rules | FAIL at S1.2 on a stale step (contained toggle row), fixed in #7084, not re-run | #7077 |
| cabals/info-and-members | S1 PASS. S2.1 failed on a stale step, fixed in #7084, not re-run | #7077 |
| cabals/invite | Not run: setup needs B on its own login | #7077 |
| cabals/join | Not run: sign-in hit runner login state | #7077 |
| cabals/leave | Not run | #7077 |
| cabals/pause | PASS (S1–S2) | |
| cabals/pot | FAIL at S1.3 on a stale step (combined stat labels), fixed in #7084, not re-run | #7077 |
| chat/cabal-chat | PASS (S1) | |
| chat/talk-it-over | S1–S4 PASS. S5 failed on a toast over Back, fixed in #7084, not re-run | #7077 |
| demo/story | Needs funding (A 3, B 1), not run | |
| feed/browse | S1 PASS (the known failure is fixed), S2 PASS. S3.1 failed on a stale step, fixed in #7084, not re-run | #7077 |
| feed/react-and-reply | FAIL at S1.2: searching the emoji picker for "fire" shows no `emoji-cell-1f525`, not triaged (the setup bug is fixed in #7084) | #7077 |
| governance/comments | FAIL at S1.1 on a stale step (Feed cards open the post), fixed in #7084, not re-run | #7077 |
| governance/propose-buy | Needs funding (A 2, B 1): stops at the fund step | |
| governance/propose-from-asset | FAIL at S1.1 on a stale step (native Markets search), fixed in #7084, not re-run | #7077 |
| governance/propose-sell | FAIL at S1.2: the cabal pot returns 400 on a 16-digit multiplier, so Sell never loads | #7053 |
| governance/vote | FAIL at S1.2 on a stale step (spoken tally labels), fixed in #7084, not re-run | #7077 |
| governance/withdraw | FAIL at S1.1: no filter field or Discover on the Cabals tab, not triaged | #7077 |
| home/activity | FAIL at S1.1 after #7084's fix: the bell reads "Catch up, 5 new", not 3, with Catch up items left by earlier runs on the shared local database | #7077 |
| home/dashboard | Not run | #7077 |
| money/activity | S1 PASS. S2 known failure: needs a funded setup (A 1) | |
| money/best-price-trade | Needs funding (A 2): stops at the fund step | |
| money/cash-out | Needs funding (A 2), not run | |
| money/deposit | Needs funding (A 1): S1.1 balance under $1.00 | |
| money/fund-cabal | FAIL at S1.2 on a stale step (keypad amount value), fixed in #7084, not re-run. Needs funding past the fund step | #7077 |
| money/withdraw | Needs funding (A 1, B 1), not run | |
| onboarding/first-run | FAIL at S1.6: the onboarding nudge is drawn nowhere | #7042 |
| profile/edit | PASS (S1–S4) | |
| profile/find-friends | Not run | #7077 |
| profile/follow-lists | Not run | #7077 |
| profile/invite | Not run | #7077 |
| profile/nudge | Not run | #7077 |
| profile/overview | Not run | #7077 |
| profile/user-profile | Not run | #7077 |
| ranking/leaderboards | Not run: setup panics in `monacoctl flows seed` | #7079 |
| settings/delete-account | Not run | #7077 |
| settings/notifications | Not run | #7077 |
| social/follow-and-boards | Not run: setup panics in `monacoctl flows seed` | #7079 |
| stocks/asset-detail | Not run | #7077 |
| stocks/browse | Not run | #7077 |
| stocks/watchlist | Not run | #7077 |

## Mutants

Mutants were capped to the four F launch journeys: feed/react-and-reply, ranking/leaderboards, social/follow-and-boards and home/activity.

No F journey passed, so no mutants ran. ranking/leaderboards and social/follow-and-boards stop at setup (#7079), feed/react-and-reply fails at S1.2, and home/activity fails at S1.1 on leftover items. The full mutant sweep is in #7077.

## Needs funding

These journeys move real USDC, and the pass funded no actor: demo/story, governance/propose-buy, money/best-price-trade, money/cash-out, money/deposit, money/withdraw, and money/activity S2. Run them after funding each actor from the QA pot with `monacoctl qa fund` (`docs/journeys/README.md`, Journeys that move money).

## What the pass taught

- Most failures were stale tests from the F rebuild, not app defects. Controls became menus, rows became combined or contained accessibility elements, and Feed cards now open the post.
- `onboarding/first-run` signs C in by email. Every other journey signs C in by sms, and Privy treats the two as different users. Keep C's `privy_user_id` in `apps/mobile/qa/journeys/accounts.tsv` on the sms user.
- When two runs share a Mac, `journey.py` lends a busy actor's login to another run. `cabals/invite` (B must be `@qa_b`) and `settings/delete-account` need their own logins, so run them alone.
