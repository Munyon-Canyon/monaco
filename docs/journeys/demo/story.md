---
id: demo/story
title: The demo story
version: 8
milestone: M8.5
requires: [auth/sign-in, cabals/create-cabal, stocks/asset-detail]
actors: [A, B]
flows: []
funds:
  A: 3
  B: 1
xcuitest: [apps/mobile/MonacoUITests/Journeys/DemoStoryJourney.swift, apps/mobile/MonacoUITests/Journeys/DemoStoryJourneyUITests.swift]
---

# The demo story

Two friends start a cabal, fund the pot, vote on a stock, watch it fill, cash out and see who is up. The beats are the film in [the demo storyboard](../../demo/storyboard.md). A is Maya and B is Jordan. Priya's beats (7, 8, 12) are played by B or folded into A's, because a journey run has two actors.

Old app (`c838bd24`): each beat's tap path is the one its own journey cites. Sign in is [auth/sign-in](../auth/sign-in.md), start and join are [cabals/create-cabal](../cabals/create-cabal.md) S4, the stock beats are [stocks/asset-detail](../stocks/asset-detail.md). The beats with no journey yet cite the old app's view and the [screens.md](../../screens.md) copy in their Expect cell.

Each scenario starts from state its setup script seeds, so one blocked beat does not hide the beats after it. A step that calls another journey's entry point records that journey's own step ids inside this step.

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actors A and B have signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend` |
| P3 | `apps/mobile/qa/journeys/demo/story.setup.sh` ran right before the scenario. It marks A and B done with onboarding and sets their display names. For S2 to S8, A creates the cabal `QA story {QA.run}` through the API and B joins it. S3 also seeds the journey stock catalogue (`stocks/browse.setup.sh`) |
| P4 | Before the run, the runner sends A 3 USDC and B 1 USDC from the QA pot (`monacoctl qa fund`) or the Phantom MCP agent wallet (`funds`). The test moves no money: S2 deposits from B's funded balance, S4 buys with that deposit, and S7 cashes out; the runner withdraws what is left after the run |

## Scenarios

### S1 Start a cabal and bring a friend

Beats 1 to 4.

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | sign in | `SignInJourney.ensureSignedIn` | A's test login | Home shows within 30 s (beat 1 "Sign in"; old app: `LoginView` number, code, then Home; screens.md Home) |
| S1.2 | A | tap | the Profile tab | | `profile-card` shows A's name within 15 s (beat 2 "Create your profile"; old app: the onboarding name then Profile; screens.md Profile header) |
| S1.3 | A | run | `CreateCabalJourney.creatorStartsAnOpenCabal` | `QA duo {QA.run}`, join "Anyone" | The cabal screen for `QA duo {QA.run}` and the toast "Cabal created." (beat 3 "Start a cabal / Set the rules"; old app: Cabals "+", "Start a cabal", `CreateGroupView`; screens.md Start a cabal) |
| S1.4 | B | run | `CreateCabalJourney.friendRequestsToJoin` | `QA duo {QA.run}` | `cabal-join-button` shows, then the toast "Request sent. You'll be in once the creator says yes." and `cabal-join-requested` reads "Request sent" (beat 4 "Find the cabal and ask to join"; screens.md `CabalJoinSlot`) |

### S2 Fund the pot

Beat 6.

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | B | tap, type, then tap | `openCabal(named:)` | `QA story {QA.run}` | `cabal-header-name` reads `QA story {QA.run}` within 15 s (old app: Cabals list row; screens.md Cabal screen) |
| S2.2 | B | tap | `cabal-action-fund` | | `amount-keypad-screen` shows within 15 s (beat 6 "Fund the pot"; screens.md Fund this cabal) |
| S2.3 | B | type, tap, then tap | `amount-keypad-1`, `amount-keypad-action`, then `amount-keypad-close` | `1` | `amount-keypad-settled` reads "$1 to QA story {QA.run}" within 120 s and the line reads "Added just now" (#6576) (screens.md Fund this cabal) |

### S3 Browse stocks and Pre-IPO

Beats 10 and 16.

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S3.1 | B | run | `StocksAssetDetailJourney.openAsset` | Journey Alpha | `asset-detail-root` shows and `asset-detail-name` reads "Journey Alpha" within 15 s (beat 10 "Browse real stocks"; old app: Stocks row to `AssetDetailView`; screens.md Stocks: Asset screen) |
| S3.2 | B | tap | the 1M then All range chips | | `asset-detail-range-change` shows for each (ranges 1H 1D 1W 1M 3M 6M All) |
| S3.3 | B | run | `StocksAssetDetailJourney.openAsset`, then scroll to | Journey Private, "Round stats" | "Round stats" shows within 10 s (beat 16 "Pre-IPO too") |

### S4 Propose a buy and vote

Beats 11 to 13.

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S4.1 | B | open, then tap | `QA story {QA.run}` (`openCabal(named:)`), then `cabal-action-propose` | | The Propose sheet shows `propose-kind-buy` "Buy stock" within 10 s (beat 11 "Propose a buy"; screens.md Propose chooser) |
| S4.2 | B | tap, type, then tap | `propose-kind-buy`, the GOOGL row, the amount `1` on the keypad, `amount-keypad-action`, then `propose-review-send` | | The sheet reads "Proposal sent"; after Close the cabal's proposal card reads "2 yes votes needed" within 15 s (screens.md Review sheet and proposal card) |
| S4.3 | A | tap, then hold | the Home tab's `proposal-card-<id>`, then `pickAndHold("yes")` | | The toast ends, then the permission sheet "Hear how votes end?" shows and the test answers "Not now" (`answerVotePermissionSheet(turnOn: false)`) within 20 s (beat 12 "Everyone votes"; screens.md `HomePendingVotesSlot` and the proposal screen) |
| S4.4 | B | tap, then hold | the Home tab's `proposal-card-<id>`, then `pickAndHold("yes")` | | The toast "Voted Yes. Passed 2–0." shows, and the status line reads "filled at $…" within 120 s (beat 13 "Majority wins, the cabal buys"; screens.md `ProposalDetailSlot`) |

### S5 Talk it over

Beats 14 and 15.

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S5.1 | B | open, tap, type, then send | `QA story {QA.run}`, `cabal-more-button`, `cabal-more-chat`, "Message your cabal" | "In. Told you" | The message shows on the right within 10 s (beat 14 "Talk it over"; old app: `GroupChatView`; screens.md Chat composer "Message your cabal") |
| S5.2 | A | open, tap, type, then send | `QA story {QA.run}`, `cabal-more-button`, `cabal-more-chat`, "Message your cabal" | "We own Google now" | B's "In. Told you" shows on the left and A's message on the right within 10 s (beat 15; screens.md Chat) |

### S7 Cash out

Beat 21.

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S7.1 | A | open, then tap | `QA story {QA.run}`, then `cabal-treasury-cash-out` | | The title "Cash out" and the 25% / 50% / All keys show within 10 s (beat 21 "Cash out any time"; old app: `CashOutView`; screens.md Cash out) |
| S7.2 | A | tap, then tap | "25%", then `cash-out-submit` | | A toast starting "Cashing out" within 15 s (screens.md Cash out: "Cashing out $50.03. The final amount is known when it settles") |

### S8 See who's up

Beat 22.

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S8.1 | A | tap | the Home tab, then `home-leaderboard-row` | | `leaderboard-page` shows within 15 s (beat 22 "See who's up"; Leaderboard C) |
| S8.2 | A | scroll, then wait | `leaderboard-section-unranked`, then B's `leaderboard-row-<id>` | | B, with one resolved vote, shows under "Not ranked yet" reading "4 more to rank" with Week selected within 15 s (#6514; `ranking/leaderboards` S1.4 asserts the same rows) |

## Ground truth

`apps/mobile/qa/journeys/demo/story.truth.sh` reads `cabals` and `cabal_members`. After S1, `QA duo {QA.run}` has exactly A and B as members, or does not exist when S1 stopped before S1.3. After S2 to S8, `QA story {QA.run}` has exactly A and B. No scenario adds or removes a member, and no money moves on staging.

## Known failures on staging

None known.

## Not covered

- Priya, the third friend (beats 7, 8 and 12 as Priya). A run has two actors, A and B.
- Beats 0 and 23, the intro and outro cards. They are not app screens.
- Beat 9, Maya funding $200. It is the same flow as S2 with another actor.
- The face sheet on Profile. The storyboard keeps it off camera.
- Identifiers. The fund, propose, vote, chat and cash-out screens have no accessibility identifiers yet, so S2.2 to S7.2 find them by their screens.md copy. #651 (`FundRoute`), #613 (`ProposeRoute`), #612 (`ProposalDetailSlot`), #623 (`ChatRoute`) and #657 (`CashOutRoute`) should add them; this ticket does not touch app code.
