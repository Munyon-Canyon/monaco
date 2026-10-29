---
id: cabals/leave
title: Leave a cabal
version: 6
milestone: M10
requires: [auth/sign-in]
actors: [A, B]
flows: [04]
xcuitest: [apps/mobile/MonacoUITests/Journeys/CabalsLeaveJourney.swift, apps/mobile/MonacoUITests/Journeys/CabalsLeaveJourneyUITests.swift]
---

# Leave a cabal

A member opens the cabal's “…” menu, taps "Leave cabal", confirms, and the cabal's tile is gone from the Cabals tab. A creator with other members sees why they cannot leave yet. The rules are [Cabals](../../architecture/cabals.md#leave).

Old app (`c838bd24`): `GroupDetailView.swift` -> "Leave cabal" -> the dialog "Leave X?" with "Leave cabal" (`GroupDetailsSheet.swift` "Leaving…"). The old dialog also had "Sell and leave" for a member with a stake; the app now refuses that leave and offers "Cash out" first ([Cabals](../../architecture/cabals.md#leave)). Spec: `CabalLeaveSlot` in [screens.md](../../screens.md#cabal-screen-cabalroute), now offered from the cabal page's “…” menu.

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actors A and B have each signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend` |
| P3 | `apps/mobile/qa/journeys/cabals/leave.setup.sh` ran right before the scenario. It marks A and B as done with onboarding and sets their display names. For S1, a new dev user "QA host" creates the cabal `QA leave {QA.run}` and B joins it through the API. For S2, A creates the cabal `QA stay {QA.run}` and B joins it through the API. No one has a stake: the journey moves no money |

## Scenarios

### S1 A member leaves

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | B | `openCabal(named:)` | the Cabals tab and the tile | `QA leave {QA.run}` | `cabal-header-name` reads `QA leave {QA.run}` and `cabal-member-count` reads "2 members" within 15 s |
| S1.2 | B | tap, then tap | `cabal-more-button`, then `cabal-more-leave` | | The Leave cabal sheet shows `cabalLeaveButton` reading "Leave cabal" within 10 s |
| S1.3 | B | tap | `cabalLeaveButton` | | The dialog "Leave QA leave {QA.run}?" shows `cabalLeaveConfirmButton` "Leave cabal" within 5 s |
| S1.4 | B | tap | `cabalLeaveConfirmButton` | | The toast "You left QA leave {QA.run}." shows within 10 s and `cabal-header-name` is gone within 10 s |
| S1.5 | B | tap | the Cabals tab | | No `cabals-tile-<id>` for `QA leave {QA.run}` shows within 10 s: B is no longer a member |

### S2 The creator cannot leave while others remain

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | `openCabal(named:)` | the Cabals tab and the tile | `QA stay {QA.run}` | `cabal-member-count` reads "2 members" within 15 s |
| S2.2 | A | tap, then tap | `cabal-more-button`, then `cabal-more-leave` | | The sheet shows `cabalLeaveCreatorNote` reading "You can leave once everyone else has left." within 10 s, and there is no `cabalLeaveButton` |

## Ground truth

`apps/mobile/qa/journeys/cabals/leave.truth.sh` reads `cabal_members`. After S1, B is not a member of `QA leave {QA.run}`. After S2, A and B are both still members of `QA stay {QA.run}`.

## Known failures on staging

None.

## Not covered

- A member with a stake leaving. The app refuses it with the toast "Cash out your share of the pot before you leave this cabal." and a "Cash out" button (`cabalLeaveCashOutButton`), and there is no "Sell and leave" in the dialog. Reaching it needs a funded stake, and this journey moves no money.
- The last member leaving a cabal whose pot still holds money. That needs a funded pot too. `LeaveCabalModelTests` covers the copy.
