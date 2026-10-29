---
id: cabals/pot
title: The treasury
version: 8
milestone: M12
requires: [auth/sign-in]
actors: [A]
flows: []
xcuitest: [apps/mobile/MonacoUITests/Journeys/CabalsPotJourney.swift, apps/mobile/MonacoUITests/Journeys/CabalsPotJourneyUITests.swift]
---

# The treasury

A member opens a cabal and reads the cabal's treasury, the stat block with their share, and the Treasury tab that holds it. A position row opens that asset. The rules are [Cabals](../../architecture/cabals.md).

Old app (`c838bd24`): Cabal -> `PotSectionView.swift` and `PotMixBar.swift`: the pot value and your slice, then "Holdings" with one row per holding (`pot-row-<symbol>`) and a "Cash" row, and a row opens the asset. Spec: `CabalPotSlot`, `CabalSliceSlot` and `CabalHoldingsSlot` in [screens.md](../../screens.md#cabal-screen-cabalroute).

The format of this doc is in [App journeys](../README.md).

On the F cabal page the pot is the "Treasury" line and the slice is the stat block. The ticket names `cabal-treasury-value`, `cabal-stat-row` and `cabal-empty-suggestions`. The app has `cabal-pot-value` (label "Treasury, $…"), `cabal-stat-block` and, for the starters, `cabal-starters`, so the journey uses those.

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actor A has signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend` |
| P3 | `apps/mobile/qa/journeys/cabals/pot.setup.sh` ran right before the scenario. It marks A as done with onboarding and sets A's display name, then A creates the cabal `QA slice {QA.run}` through the API. Nobody adds money: the pot is $0.00 and the journey moves no money |

## Scenarios

### S1 Treasury value and the stat block

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | call | `openCabal(named:)` | `QA slice {QA.run}` | `cabal-header-name` reads `QA slice {QA.run}`, and `cabal-pot-value` shows within 15 s |
| S1.2 | A | wait | `cabal-pot-value` | | The "Treasury" line reads "$0.00" within 10 s |
| S1.3 | A | wait | `cabal-stat-block` | | The stat block shows "Your share" "$0", "Of treasury" "0%", "Open votes" "0" and "Bot" "Off" within 10 s |

### S2 Share the cabal's gain

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | call | `openCabal(named:)` | `QA slice {QA.run}` | `cabal-header-name` reads `QA slice {QA.run}` within 15 s |
| S2.2 | A | tap | `cabal-share-button` | | F's share screen (`share-screen`, #6572) shows within 15 s |
| S2.3 | A | tap | Back | | `cabal-header-name` still reads `QA slice {QA.run}` within 5 s |

### S3 An empty treasury

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S3.1 | A | call, then call | `openCabal(named:)`, then `openCabalTab("Treasury")` | `QA slice {QA.run}` | The Treasury tab shows |
| S3.2 | A | scroll to | `cabal-holdings-empty` | | Reads "Fund, then propose the first buy." within 10 s |

## Ground truth

`apps/mobile/qa/journeys/cabals/pot.truth.sh` reads `cabals` and `cabal_members`. After any scenario, `QA slice {QA.run}` exists and A is its only member. The journey writes nothing else.

## Known failures on staging

None.

## Not covered

- "Cabals like yours start with" (`cabal-starters`, #6524). The app shows it only when the cabal is funded and has no proposal yet (`CabalEmptyGate`), and this journey moves no money, so a $0.00 cabal shows `cabal-holdings-empty` instead.
- A zero pot has no Cash row: the Treasury tab shows only `cabal-holdings-empty`. The Cash row (`cabal-holdings-cash`, which opens "Cabal cash") and a holding row (`cabal-holding-<ticker>`, which opens that asset) need a funded pot, and this journey moves no money.
- The all-time chip "▲ $0.73 · all time", "38% of the pot", the allocation bar legend, and a stock row ("0.73 shares · $341.58") opening its asset. Each needs a funded pot with a buy, and this journey moves no money.
