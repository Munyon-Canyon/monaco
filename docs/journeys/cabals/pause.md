---
id: cabals/pause
title: A paused cabal
version: 1
milestone: M12
requires: [auth/sign-in]
actors: [A]
flows: []
xcuitest: [apps/mobile/MonacoUITests/Journeys/CabalsPauseJourney.swift, apps/mobile/MonacoUITests/Journeys/CabalsPauseJourneyUITests.swift]
---

# A paused cabal

Monaco pauses a cabal when USDC lands in its treasury straight from outside, or when an operator pauses it. A member who opens a paused cabal sees a warning with the reason and when funding and cash outs resume. A cabal that is not paused shows no warning. The rules are [Cabals](../../architecture/cabals.md).

Old app (`c838bd24`): `GroupDetailView.swift`, the pause warning shown after USDC was sent straight to the treasury. Spec: `CabalPauseSlot` in [screens.md](../../screens.md#cabal-screen-cabalroute).

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actor A has signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend` |
| P3 | `apps/mobile/qa/journeys/cabals/pause.setup.sh` ran right before the scenario. It marks A as done with onboarding and sets A's display name. For S1, A creates the open cabal `QA paused {QA.run}` through the API and the setup pauses it with `monacoctl ops pause --note "QA {QA.run}"`. For S2, A creates the open cabal `QA running {QA.run}` and nothing pauses it. Once a flow seeds the external-deposit pause, S1 seeds it with `qa_flow_seed` instead |

## Scenarios

### S1 A paused cabal shows why

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap, type, then tap | the Cabals tab, `cabals-search-field`, then the `cabals-search-result-<id>` | `QA paused {QA.run}` | `cabal-header-name` reads `QA paused {QA.run}` within 15 s |
| S1.2 | A | wait | `cabal-pause-banner` | | The warning row shows the reason and "Funding and cash outs resume after" within 10 s (screens.md `CabalPauseSlot`: "A warning row when the cabal is paused, with the reason and "Funding and cash outs resume after…""; old app: `GroupDetailView` pause warning) |

### S2 A running cabal shows no warning

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | tap, type, then tap | the Cabals tab, `cabals-search-field`, then the `cabals-search-result-<id>` | `QA running {QA.run}` | `cabal-header-name` reads `QA running {QA.run}` within 15 s |
| S2.2 | A | wait | `cabal-pause-banner` | | There is no `cabal-pause-banner` after 5 s (screens.md `CabalPauseSlot`: "Hidden otherwise") |

## Ground truth

`apps/mobile/qa/journeys/cabals/pause.truth.sh` reads `cabal_pauses`. `QA paused {QA.run}` has one open pause, and `QA running {QA.run}` has none. Opening a cabal never resolves a pause.

## Known failures on staging

- S1.2: no warning row. `CabalPauseSlot` is not live and draws nothing, and the app has no route to read a cabal's pause. Blocked by #657.

## Not covered

- `cabal-pause-banner` does not exist yet. #657 adds it in `apps/mobile/Monaco/Features/Groups/CabalSlots/CabalPauseSlot.swift`; this ticket does not touch app code.
- The pause shown on the fund and cash out screens (#651, #657), and the paused caption on a proposal card. Those belong to the fund, cash out and proposal journeys.
- Resuming a cabal. That is an operator command (`monacoctl ops resume`), not a member action.
