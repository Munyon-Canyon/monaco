---
id: governance/propose-buy
title: Propose a buy
version: 1
milestone: M13
requires: [auth/sign-in]
actors: [A]
flows: [09]
xcuitest: [apps/mobile/MonacoUITests/Journeys/GovernanceProposeBuyJourney.swift, apps/mobile/MonacoUITests/Journeys/GovernanceProposeBuyJourneyUITests.swift]
---

# Propose a buy

A voter opens a cabal, taps Propose, picks "Buy a stock", searches GOOGL, enters $25, reviews and sends the proposal to the cabal. The rules are [Proposals](../../architecture/proposals.md).

Old app (`c838bd24`): Cabal -> Propose -> `ProposeChooserView` -> `ProposeBuyView` -> `ProposeAmountView` -> `ProposeReviewView` -> Send -> toast. Spec: "Propose" in [screens.md](../../screens.md#propose-613).

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actor A has signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend` |
| P3 | `apps/mobile/qa/journeys/governance/propose-buy.setup.sh` ran right before the scenario. It marks A as done with onboarding, and A creates the open cabal `QA buy {QA.run}` through the API, so A is its creator and a voter |

## Scenarios

### S1 A voter proposes a buy

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap, type, then tap | the Cabals tab, `cabals-search-field`, then the `cabals-search-result-<id>` | `QA buy {QA.run}` | `cabal-header-name` reads `QA buy {QA.run}`, and `cabal-action-propose` "Propose" is enabled with no "Only voters can propose" caption, within 15 s |
| S1.2 | A | tap | `cabal-action-propose` | | The chooser titled "Propose" shows the row "Buy a stock" with "Your cabal votes on it first" within 10 s (old app: `ProposeChooserView`) |
| S1.3 | A | tap | "Buy a stock" | | The screen titled "Buy" shows the search "Search Apple, Tesla, NVDA…" and the section "Popular" within 10 s (old app: `ProposeBuyView`) |
| S1.4 | A | type, then tap | the search field, then the GOOGL row | `GOOGL` | The screen titled "Amount" shows the chips "$25", "$50", "$100", "Max" within 10 s (old app: `ProposeAmountView`) |
| S1.5 | A | tap, then tap | "$25", then "Review" | | The screen titled "Review" reads "Buy $25.00 of GOOGL" with the rows "Cabal gets", "Price", "Pot" and "Who votes" within 10 s (old app: `ProposeReviewView`) |
| S1.6 | A | tap | "Send to cabal" | | The toast "Proposal sent to QA buy {QA.run}" shows and the flow closes back to the cabal screen within 10 s |

## Ground truth

`apps/mobile/qa/journeys/governance/propose-buy.truth.sh` reads `proposals`. After S1, `QA buy {QA.run}` has one `buy` proposal for GOOGL by A worth 25 USDC. It skips the check when the run never created the cabal, and reports no proposal as a failure only after the setup ran.

## Known failures on staging

- S1.2 to S1.6: `ProposeRoute` shows "Propose isn't on the new backend yet." The chooser, Buy, Amount and Review screens are blocked by #613.
- S1.5: the pot is empty, so "Review" stays disabled with "More than the pot has" even once #613 lands. The setup seeds no pot money because this journey moves no money. Seeding a funded pot without real USDC needs a testkit loader that does not exist yet.

## Not covered

- Every target past S1.1 is a label, not an accessibility identifier: the screens do not exist yet, and their identifiers come with #613 (`apps/mobile/Monaco/Features/Proposals/ProposeRoute.swift`). Version 2 of this doc swaps the labels for those identifiers.
- "+ Add a reason" and the preview errors ("More than the pot has", "Can't buy <name> right now. Try a smaller amount or another stock."). Flow 09's outcomes cover them in `monacoctl flows check`.
- A member who is not a voter. `cabal-action-propose-caption` "Only voters can propose" is covered by `CabalActionsSlot`'s sample harness.
