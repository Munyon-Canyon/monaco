---
id: governance/propose-from-asset
title: Propose a buy from a stock
version: 1
milestone: M13
requires: [auth/sign-in]
actors: [A]
flows: [09]
xcuitest: [apps/mobile/MonacoUITests/Journeys/GovernanceProposeFromAssetJourney.swift, apps/mobile/MonacoUITests/Journeys/GovernanceProposeFromAssetJourneyUITests.swift]
---

# Propose a buy from a stock

A voter opens GOOGL on the Stocks tab, taps "Propose buy", picks a cabal, enters $25, reviews and sends the proposal. The rules are [Proposals](../../architecture/proposals.md).

Old app (`c838bd24`): Asset `AssetTradeBar` "Propose buy" -> `GroupPickerForProposalView` -> `ProposeAmountView` -> `ProposeReviewView` -> Send -> toast. Spec: "From a stock" under "Propose" and "Asset screen" in [screens.md](../../screens.md#propose-613).

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actor A has signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend`, and the asset catalogue lists GOOGL |
| P3 | `apps/mobile/qa/journeys/governance/propose-from-asset.setup.sh` ran right before the scenario. It marks A as done with onboarding, and A creates the open cabal `QA asset {QA.run}` through the API. A votes in earlier runs' cabals too, so the picker always shows |

## Scenarios

### S1 A voter proposes a buy from the asset screen

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap, type, then tap | the Stocks tab, the search "Search Apple, Tesla, NVDA…", then `assets-row-GOOGL…` | `GOOGL` | `asset-detail-root` shows, and `asset-detail-propose-buy` reads "Propose buy" over "Your cabal votes before anything is bought", within 15 s |
| S1.2 | A | tap | `asset-detail-propose-buy` | | The picker titled "Pick a cabal" asks "Which cabal should buy GOOGL?" and lists `QA asset {QA.run}` within 10 s (old app: `GroupPickerForProposalView`) |
| S1.3 | A | tap | the `QA asset {QA.run}` row | | The screen titled "Amount" shows the chips "$25", "$50", "$100", "Max" within 10 s, with no Buy search step (old app: `ProposeAmountView`) |
| S1.4 | A | tap, then tap | "$25", then "Review" | | The screen titled "Review" reads "Buy $25.00 of GOOGL" within 10 s (old app: `ProposeReviewView`) |
| S1.5 | A | tap | "Send to cabal" | | The toast "Proposal sent to QA asset {QA.run}" shows and the flow closes within 10 s |

## Ground truth

`apps/mobile/qa/journeys/governance/propose-from-asset.truth.sh` reads `proposals`. After S1, `QA asset {QA.run}` has one `buy` proposal for GOOGL by A worth 25 USDC. It skips the check when the run never created the cabal.

## Known failures on staging

- S1.2 to S1.5: `ProposeFromAssetRoute` shows "Propose isn't on the new backend yet." The cabal picker and every later step are blocked by #613.
- S1.4: the pot is empty, so "Review" stays disabled with "More than the pot has" even once #613 lands. Seeding a funded pot without real USDC needs a testkit loader that does not exist yet.

## Not covered

- Every target past S1.1 is a label, not an accessibility identifier: the screens do not exist yet, and their identifiers come with #613 (`apps/mobile/Monaco/Features/Proposals/ProposeFromAssetRoute.swift`). The Stocks search field has no identifier either (`apps/mobile/Monaco/Features/Assets/StocksTabView.swift`, `MonacoSearchField`), so S1.1 finds it by its placeholder.
- One cabal (straight to Amount) and no cabal (the sheet "Join a cabal first" with "Browse cabals"). The QA account votes in many cabals, so neither state is reachable without a fresh account.
- "Can't buy right now" for a stock that cannot trade.
