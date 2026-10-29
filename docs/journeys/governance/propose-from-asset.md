---
id: governance/propose-from-asset
title: Propose a buy from a stock
version: 10
milestone: M13
requires: [auth/sign-in]
actors: [A]
flows: [09]
xcuitest: [apps/mobile/MonacoUITests/Journeys/GovernanceProposeFromAssetJourney.swift, apps/mobile/MonacoUITests/Journeys/GovernanceProposeFromAssetJourneyUITests.swift]
---

# Propose a buy from a stock

A voter opens GOOGL on the Markets tab, taps "Propose buy", picks a cabal and reaches the Phantom keypad, where an amount over the cabal's empty pot is refused. The rules are [Proposals](../../architecture/proposals.md).

Old app (`c838bd24`): Asset `AssetTradeBar` "Propose buy" -> `GroupPickerForProposalView` -> `ProposeAmountView` -> `ProposeReviewView` -> Send -> toast. Spec: "Choose a cabal" and "Amount" under "Propose" and "Stock screen" in [screens.md](../../screens.md#propose-613).

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actor A has signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend`, and the asset catalogue lists GOOGL |
| P3 | `apps/mobile/qa/journeys/governance/propose-from-asset.setup.sh` ran right before the scenario. It marks A as done with onboarding, and A creates the cabal `QA asset {QA.run}` through the API. A votes in earlier runs' cabals too, so the picker always shows |

## Scenarios

### S1 A voter proposes a buy from the asset screen

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap, type, then tap | the Markets tab (`tab-assets`), the search field `monaco-search-field` ("Stocks and Pre-IPO"), then `assets-row-GOOGL…` | `GOOGL` | `asset-detail-root` shows, and `asset-detail-propose-buy` reads "Propose buy", within 15 s |
| S1.2 | A | tap | `asset-detail-propose-buy` | | A half sheet titled "Choose a cabal" (`propose-pick-cabal-sheet`) opens over the stock and asks "Which cabal should buy GOOGL?" (`propose-pick-cabal-question`) and lists `QA asset {QA.run}` within 10 s (old app: `GroupPickerForProposalView`) |
| S1.3 | A | tap | the `QA asset {QA.run}` row (`propose-pick-cabal-<id>`) | | The sheet closes and the Amount screen (`propose-amount-screen`, which holds the keypad) shows within 10 s, `amount-keypad-source` names `QA asset {QA.run}` and its cash, and the chips `amount-keypad-preset-<label>` read "$25", "$50", "$100", "Max", with no Buy search step |
| S1.4 | A | tap | `amount-keypad-5` | | `amount-keypad-limit-line` reads "… available · … over" in place of the chips within 10 s, and `amount-keypad-action` does not show (founder decision Q2: the keypad replaces the chips with one grey limit line) |

## Ground truth

`apps/mobile/qa/journeys/governance/propose-from-asset.truth.sh` reads `proposals`. After S1, `QA asset {QA.run}` has no proposal, because the screens refused the amount and sent nothing. It skips the check when the run never created the cabal.

## Known failures on staging

None known.

## Not covered

- Review and Send. The cabal `QA asset {QA.run}` has an empty pot, and seeding cash needs real USDC, so the Review sheet cannot open here. [governance/propose-buy](propose-buy.md) funds a pot and covers the thesis, Review proposal, Send to vote and the vote.
- One cabal (straight to Amount) and no cabal (the sheet "Join a cabal first" with "Find a cabal"). The QA account votes in many cabals, so neither state is reachable without a fresh account.
- "Can't buy right now" for a stock that cannot trade.
