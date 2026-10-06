---
id: governance/propose-sell
title: Propose a sell
version: 2
milestone: M13
requires: [auth/sign-in]
actors: [A]
flows: [09]
xcuitest: [apps/mobile/MonacoUITests/Journeys/GovernanceProposeSellJourney.swift, apps/mobile/MonacoUITests/Journeys/GovernanceProposeSellJourneyUITests.swift]
---

# Propose a sell

A voter in a cabal that holds a stock taps Propose, picks "Sell something the cabal owns", picks the holding, enters 25%, reviews and sends the proposal. The rules are [Proposals](../../architecture/proposals.md).

Old app (`c838bd24`): Cabal -> Propose -> `ProposeChooserView` -> `ProposeSellView` -> `ProposeAmountView` -> `ProposeReviewView` -> Send -> toast. Spec: "Sell" under "Propose" in [screens.md](../../screens.md#propose-613).

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actor A has signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend` |
| P3 | `apps/mobile/qa/journeys/governance/propose-sell.setup.sh` ran right before the scenario. It marks A as done with onboarding, seeds `monacoctl dev seed-scenario cabal-with-confirmed-trade` with A as owner (a cabal that holds AAPLx), renames it `QA sell {QA.run}`, and writes AAPLx samples to `price_points` every 4 minutes for the next 16 minutes, because the pot refuses a price older than 5 minutes and the journey backend runs no price poller |

## Scenarios

### S1 A voter proposes a sell

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap, type, then tap | the Cabals tab, `cabals-search-field`, then the `cabals-search-result-<id>` | `QA sell {QA.run}` | `cabal-header-name` reads `QA sell {QA.run}`, and `cabal-action-propose` "Propose" is enabled within 15 s |
| S1.2 | A | tap | `cabal-action-propose` | | The chooser titled "Propose" shows `propose-kind-sell` within 10 s, and it is enabled, not "Nothing to sell yet" (old app: `ProposeChooserView`) |
| S1.3 | A | tap | `propose-kind-sell` | | The screen titled "Sell" shows the header "What the cabal owns" and the AAPL row within 10 s (old app: `ProposeSellView`) |
| S1.4 | A | tap | `propose-sell-<symbol>` for AAPL | | The amount screen shows the chips "25%", "50%", "All" and the helper "The cabal holds …" within 10 s |
| S1.5 | A | tap, then tap | "25%", then "Review" | | The screen titled "Review" reads "Sell … of AAPL" with the rows "Raises", "Cabal keeps" and "Who votes" within 10 s |
| S1.6 | A | tap | "Send to cabal" | | The toast "Proposal sent to QA sell {QA.run}" shows and the flow closes back to the cabal screen within 10 s |

## Ground truth

`apps/mobile/qa/journeys/governance/propose-sell.truth.sh` reads `proposals`. After S1, `QA sell {QA.run}` has one `sell` proposal for AAPL by A. It skips the check when the run never created the cabal.

## Known failures on staging

None.

## Not covered

- S1.5 and S1.6 target labels. The amount screen gives its chips no identifier, and XCUITest finds no `propose-amount-review` under the screen's own `propose-amount-screen` identifier (run 20261006T005749Z), so the steps tap "Review" and "Send to cabal" the way `governance/propose-buy` does.
- A holding without a price, which takes a "Shares" or "Tokens" quantity. Flow 09's outcomes cover the server side.
- "Propose sell" from the asset screen (`asset-detail-sell`). `governance/propose-from-asset` covers the buy entry; the sell entry is the same route with the sell kind.
