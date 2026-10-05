---
id: demo/story
title: The demo story
version: 1
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

Two friends start a cabal, fund the pot, vote on a stock, watch it fill, add a trading bot, cash out and see who is up. The beats are the film in [the demo storyboard](../../demo/storyboard.md). A is Maya and B is Jordan. Priya's beats (7, 8, 12) are played by B or folded into A's, because a journey run has two actors.

Old app (`c838bd24`): each beat's tap path is the one its own journey cites. Sign in is [auth/sign-in](../auth/sign-in.md), start and join are [cabals/create-cabal](../cabals/create-cabal.md) S4, the stock beats are [stocks/asset-detail](../stocks/asset-detail.md). The beats with no journey yet cite the old app's view and the [screens.md](../../screens.md) copy in their Expect cell.

Each scenario starts from state its setup script seeds, so one blocked beat does not hide the beats after it. A step that calls another journey's entry point records that journey's own step ids inside this step.

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actors A and B have signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend` |
| P3 | `apps/mobile/qa/journeys/demo/story.setup.sh` ran right before the scenario. It marks A and B done with onboarding and sets their display names. For S2 to S8, A creates the open cabal `QA story {QA.run}` through the API and B joins it. S3 also seeds the journey stock catalogue (`stocks/browse.setup.sh`) |
| P4 | Before the run, the runner sends A 3 USDC and B 1 USDC from the QA pot (`monacoctl qa fund`) or the Phantom MCP agent wallet (`funds`). No step moves money on staging, because every money beat is a known failure |

## Scenarios

### S1 Start a cabal and bring a friend

Beats 1 to 5.

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | sign in | `SignInJourney.ensureSignedIn` | A's test login | Home shows within 30 s (beat 1 "Sign in"; old app: `LoginView` number, code, then Home; screens.md Home) |
| S1.2 | A | tap | the Profile tab | | `profile-header` shows A's name within 15 s (beat 2 "Create your profile"; old app: the onboarding name then Profile; screens.md Profile header) |
| S1.3 | A | run | `CreateCabalJourney.creatorStartsAnOpenCabal` | `QA duo {QA.run}`, join "Anyone" | The cabal screen for `QA duo {QA.run}` and the toast "Cabal created." (beat 3 "Start a cabal / Set the rules"; old app: Cabals "+", "Start a cabal", `CreateGroupView`; screens.md Start a cabal) |
| S1.4 | A | wait | `cabal-invite-card` | | The 10-character `cabal-invite-code` and `cabal-invite-copy` show (beat 4 "Invite your friends with a code"; old app: cabal details, Copy code; screens.md Cabal details invite card) |
| S1.5 | B | run | `CreateCabalJourney.friendJoinsWithTheCode` | the code from S1.4 | The toast "You're in." and `cabal-member-count` reads "2 members" (beat 5 "A paste and they're in!"; old app: "Join with an invite code", paste; screens.md Join with an invite code) |

### S2 Fund the pot

Beat 6.

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | B | tap, type, then tap | the Cabals tab, `cabals-search-field`, then the `cabals-search-result-<id>` | `QA story {QA.run}` | `cabal-header-name` reads `QA story {QA.run}` within 15 s (old app: Cabals list row; screens.md Cabal screen) |
| S2.2 | B | tap | `cabal-action-fund` | | The title "Fund this cabal" and "Max" show within 10 s (beat 6 "Fund the pot"; old app: `FundGroupView`; screens.md Fund this cabal) |
| S2.3 | B | tap, then tap | "Max", then "Add $1.00 to the pot" | | The toast "Added $1.00 to QA story {QA.run}." within 30 s (screens.md Fund this cabal toasts) |

### S3 Browse stocks and Pre-IPO

Beats 10 and 16.

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S3.1 | B | run | `StocksAssetDetailJourney.openAsset` | Journey Alpha | `asset-detail-root` shows and `asset-detail-name` reads "Journey Alpha" within 15 s (beat 10 "Browse real stocks"; old app: Stocks row to `AssetDetailView`; screens.md Stocks: Asset screen) |
| S3.2 | B | tap | the 1M then 1Y range chips | | `asset-detail-range-change` shows for each (screens.md: chips 1D 1W 1M 3M 1Y ALL) |
| S3.3 | B | run | `StocksAssetDetailJourney.openAsset`, then scroll to | Journey Private, "Private-market reference" | "Private-market reference" and "Also available from" show within 10 s (beat 16 "Pre-IPO too"; old app: the Pre-IPO block; screens.md: Pre-IPO adds "Private-market reference", "Also available from") |

### S4 Propose a buy and vote

Beats 11 to 13.

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S4.1 | B | open, then tap | `QA story {QA.run}`, then `cabal-action-propose` | | The title "Propose" and the row "Buy a stock" show within 10 s (beat 11 "Propose a buy"; old app: `ProposeTradeView`; screens.md Propose chooser) |
| S4.2 | B | tap, type, then tap | "Buy a stock", the amount, the reason, "Review", "Send" | `$1`, "Super bullish. This stock will only keep growing." | The proposal card shows "1 of 2 voted" within 15 s (screens.md proposal card tracker) |
| S4.3 | A | tap | Home "Needs your vote", then "Yes" | | The toast "Vote in" within 10 s (beat 12 "Everyone votes"; old app: `ProposalCard` Yes; screens.md `HomePendingVotesSlot` and the proposal card) |
| S4.4 | A | wait | the proposal's "Status" stepper | | "Done" is reached and the card reads "Bought" within 60 s (beat 13 "Majority wins, the cabal buys"; screens.md `ProposalDetailSlot`) |

### S5 Talk it over

Beats 14 and 15.

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S5.1 | B | open, tap, type, then send | `QA story {QA.run}`, `cabal-action-chat`, "Message your cabal" | "In. Told you" | The message shows on the right within 10 s (beat 14 "Talk it over"; old app: `GroupChatView`; screens.md Chat composer "Message your cabal") |
| S5.2 | A | open, tap, type, then send | `QA story {QA.run}`, `cabal-action-chat`, "Message your cabal" | "We own Google now" | B's "In. Told you" shows on the left and A's message on the right within 10 s (beat 15; screens.md Chat) |

### S6 Add and connect a trading bot

Beats 17 to 20.

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S6.1 | A | open, tap, then tap | `QA story {QA.run}`, `cabal-action-propose`, "Add a trading bot" | `$1`, "Scout" | The proposal card for the bot shows within 15 s (beat 17 "Add a trading bot"; screens.md Propose chooser "Add a trading bot" / "Give a bot a budget from the pot") |
| S6.2 | B | open, then tap | the cabal's "Trading bot" row, then "Copy connect instructions" | | The toast "Connect instructions copied" within 10 s (beats 18 and 19 "Connect it to ClawPump"; screens.md Trading bot "Bot key" card) |
| S6.3 | B | scroll to | "Trades" | | A trade by Scout shows within 30 s (beat 20 "Watch it trade"; screens.md Trading bot "Trades") |

### S7 Cash out

Beat 21.

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S7.1 | A | open, then tap | `QA story {QA.run}`, then `cabal-action-cash-out` | | The title "Cash out" and "25%" show within 10 s (beat 21 "Cash out any time"; old app: `CashOutView`; screens.md Cash out) |
| S7.2 | A | tap, then tap | "25%", then the "Cash out $…" button | | A toast starting "Cashing out" within 15 s (screens.md Cash out: "Cashing out $50.03. It lands in your balance in about a minute") |

### S8 See who's up

Beat 22.

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S8.1 | A | tap, then scroll to | the Home tab, then "Top investors" | | The header "Top investors" shows within 15 s (beat 22 "See who's up"; old app: Home leaderboard; screens.md `HomePeopleBoardSlot`: "Top investors") |
| S8.2 | A | wait | a ranked row | | A row names B within 15 s (screens.md `HomePeopleBoardSlot`: ranked rows with avatar, name, return) |

## Ground truth

`apps/mobile/qa/journeys/demo/story.truth.sh` reads `cabals` and `cabal_members`. After S1, `QA duo {QA.run}` has exactly A and B as members, or does not exist when S1 stopped before S1.3. After S2 to S8, `QA story {QA.run}` has exactly A and B. No scenario adds or removes a member, and no money moves on staging.

## Known failures on staging

| Step | Why | Blocking ticket |
| --- | --- | --- |
| S2.2, S2.3 | Fund the pot: `FundRoute` is a stub with no amount entry, and the fund route is missing | #608, #651 |
| S3.1, S3.2, S3.3 | The Stocks tab is blank on staging, so no stock opens; the Pre-IPO block is dropped | #577 |
| S4.1, S4.2 | Propose opens a stub, not the propose flow | #613 |
| S4.3, S4.4 | No "Needs your vote", no vote, no majority buy | #612 |
| S5.1, S5.2 | Chat is a stub with no composer | #623 |
| S6.1, S6.2, S6.3 | No "Add a trading bot", no trading bot screen | #691 |
| S7.1, S7.2 | Cash out is a stub with no amount entry | #657 |
| S8.2 | The people board has no ranked rows | #617 |

## Not covered

- Priya, the third friend (beats 7, 8 and 12 as Priya). A run has two actors, A and B.
- Beats 0 and 23, the intro and outro cards. They are not app screens.
- Beat 9, Maya funding $200. It is the same flow as S2 with another actor.
- The face sheet on Profile. The storyboard keeps it off camera.
- Identifiers. The fund, propose, vote, chat, bot and cash-out screens have no accessibility identifiers yet, so S2.2 to S7.2 find them by their screens.md copy. #651 (`FundRoute`), #613 (`ProposeRoute`), #612 (`ProposalDetailSlot`), #623 (`ChatRoute`), #691 (`AgentRoute`) and #657 (`CashOutRoute`) should add them; this ticket does not touch app code.
