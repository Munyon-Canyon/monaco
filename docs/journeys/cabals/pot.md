---
id: cabals/pot
title: The cabal pot
version: 1
milestone: M12
requires: [auth/sign-in]
actors: [A]
flows: []
xcuitest: [apps/mobile/MonacoUITests/Journeys/CabalsPotJourney.swift, apps/mobile/MonacoUITests/Journeys/CabalsPotJourneyUITests.swift]
---

# The cabal pot

A member opens a cabal and reads what is in the pot, their slice of it, and the holdings that make it up. A holding row opens that asset. The rules are [Cabals](../../architecture/cabals.md).

Old app (`c838bd24`): Cabal -> `PotSectionView.swift` and `PotMixBar.swift`: the pot value and your slice, then "Holdings" with one row per holding (`pot-row-<symbol>`) and a "Cash" row, and a row opens the asset. Spec: `CabalPotSlot`, `CabalSliceSlot` and `CabalHoldingsSlot` in [screens.md](../../screens.md#cabal-screen-cabalroute).

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actor A has signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend` |
| P3 | `apps/mobile/qa/journeys/cabals/pot.setup.sh` ran right before the scenario. It marks A as done with onboarding and sets A's display name, then A creates the open cabal `QA slice {QA.run}` through the API. Nobody adds money: the pot is $0.00 and the journey moves no money |

## Scenarios

### S1 Pot value and your slice

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap, type, then tap | the Cabals tab, `cabals-search-field`, then the `cabals-search-result-<id>` | `QA slice {QA.run}` | `cabal-header-name` reads `QA slice {QA.run}`, and the hero shows "In the pot" within 15 s (screens.md `CabalPotSlot`: "In the pot") |
| S1.2 | A | wait | `cabal-pot-value` | | Reads "$0.00" within 10 s (screens.md `CabalPotSlot`: pot value from `GET /v1/cabals/{id}/pot`; old app: `PotSectionView` pot value) |
| S1.3 | A | wait | `cabal-slice-value` | | Under the hairline, "Your slice" over "$0.00" with "Add money to get a slice" within 10 s (screens.md `CabalSliceSlot`: No stake: "$0.00" with "Add money to get a slice") |

### S2 Holdings

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | tap, type, then tap | the Cabals tab, `cabals-search-field`, then the `cabals-search-result-<id>` | `QA slice {QA.run}` | `cabal-header-name` reads `QA slice {QA.run}` within 15 s |
| S2.2 | A | scroll to | "Holdings" | | The section header "Holdings" shows within 10 s (screens.md `CabalHoldingsSlot`: "Holdings") |
| S2.3 | A | wait | `cabal-holdings-empty` | | Reads "Add money, then propose the first buy." within 10 s (screens.md `CabalHoldingsSlot`: Zero pot; old app: `pot-empty` "Add money, then propose the first buy.") |
| S2.4 | A | tap | `cabal-holdings-cash` | | The asset screen for "Cash" shows within 10 s (screens.md: "Rows open `AssetRoute`"; old app: `pot-row-USDC`) |

## Ground truth

`apps/mobile/qa/journeys/cabals/pot.truth.sh` reads `cabals` and `cabal_members`. After any scenario, `QA slice {QA.run}` exists and A is its only member. The journey writes nothing else.

## Known failures on staging

- S1.2: no pot value. The hero shows "The pot's value shows up here soon." (`cabal-pot-coming`). Blocked by #2136 (the `GET /v1/cabals/{id}/pot` route) and #2137 (the slot).
- S1.3: no slice value. The hero shows "Your slice shows up here soon." (`cabal-slice-coming`). Blocked by #2136 and #2137.
- S2.3: no holdings rows or empty state. The section shows "Holdings show up here soon." (`cabal-holdings-coming`). Blocked by #2136 and #2137.
- S2.4: no holding row opens an asset. Blocked by #2136.

## Not covered

- `cabal-pot-value`, `cabal-slice-value`, `cabal-holdings-empty` and `cabal-holdings-cash` do not exist yet. #2137 adds them in `CabalPotSlot.swift`, `CabalSliceSlot.swift` and `CabalHoldingsSlot.swift` under `apps/mobile/Monaco/Features/Groups/CabalSlots/`; this ticket does not touch app code.
- The all-time chip "▲ $0.73 · all time", "38% of the pot", the allocation bar legend, and a stock row ("0.73 shares · $341.58") opening its asset. Each needs a funded pot with a buy, and this journey moves no money.
