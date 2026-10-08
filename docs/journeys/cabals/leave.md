---
id: cabals/leave
title: Leave a cabal
version: 2
milestone: M10
requires: [auth/sign-in]
actors: [A, B]
flows: [04]
xcuitest: [apps/mobile/MonacoUITests/Journeys/CabalsLeaveJourney.swift, apps/mobile/MonacoUITests/Journeys/CabalsLeaveJourneyUITests.swift]
---

# Leave a cabal

A member opens a cabal's details, taps "Leave cabal", confirms, and lands back on the Cabals tab. A creator with other members sees why they cannot leave yet. A member with a stake is offered to sell and leave. The rules are [Cabals](../../architecture/cabals.md#leave).

Old app (`c838bd24`): `GroupDetailView.swift` -> "Leave cabal" -> the dialog "Leave X?" with "Leave cabal", and "Sell and leave" for a member with a stake (`GroupDetailsSheet.swift` "Leaving…"). Spec: `CabalLeaveSlot` in [screens.md](../../screens.md#cabal-screen-cabalroute), the details sheet's last slot.

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actors A and B have each signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend` |
| P3 | `apps/mobile/qa/journeys/cabals/leave.setup.sh` ran right before the scenario. It marks A and B as done with onboarding and sets their display names. For S1, a new dev user "QA host" creates the cabal `QA leave {QA.run}`, and for S3 the cabal `QA sell {QA.run}`. B joins each through the API. For S2, A creates the cabal `QA stay {QA.run}` and B joins it through the API. No one has a stake: the journey moves no money |

## Scenarios

### S1 A member leaves

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | B | tap, type, then tap | the Cabals tab, `cabals-search-field`, then the `cabals-search-result-<id>` | `QA leave {QA.run}` | `cabal-header-name` reads `QA leave {QA.run}` and `cabal-member-count` reads "2 members" within 15 s |
| S1.2 | B | tap, then scroll to | `cabal-details-button`, then `cabalLeaveButton` | | The "Cabal details" sheet shows, and `cabalLeaveButton` reads "Leave cabal" within 10 s |
| S1.3 | B | tap | `cabalLeaveButton` | | The dialog "Leave QA leave {QA.run}?" shows `cabalLeaveConfirmButton` "Leave cabal" within 5 s |
| S1.4 | B | tap | `cabalLeaveConfirmButton` | | The toast "You left QA leave {QA.run}." shows within 10 s, and the Cabals tab (`cabals-root`) shows within 10 s |
| S1.5 | B | type | `cabals-search-field` | `QA leave {QA.run}` | The `cabals-search-enter-<id>` for `QA leave {QA.run}` is labelled "Join QA leave {QA.run}" within 10 s: B is no longer a member |

### S2 The creator cannot leave while others remain

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | tap, type, then tap | the Cabals tab, `cabals-search-field`, then the `cabals-search-result-<id>` | `QA stay {QA.run}` | `cabal-member-count` reads "2 members" within 15 s |
| S2.2 | A | tap, then scroll to | `cabal-details-button`, then `cabalLeaveCreatorNote` | | `cabalLeaveCreatorNote` reads "You can leave once everyone else has left." within 10 s, and there is no `cabalLeaveButton` |

### S3 Sell and leave

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S3.1 | B | tap, type, then tap | the Cabals tab, `cabals-search-field`, then the `cabals-search-result-<id>` | `QA sell {QA.run}` | `cabal-header-name` reads `QA sell {QA.run}` within 15 s |
| S3.2 | B | tap, scroll to, then tap | `cabal-details-button`, then `cabalLeaveButton` | | The dialog "Leave QA sell {QA.run}?" offers "Sell and leave" next to "Leave cabal" within 5 s (old app: `GroupDetailView.swift` "Sell and leave") |

## Ground truth

`apps/mobile/qa/journeys/cabals/leave.truth.sh` reads `cabal_members`. After S1, B is not a member of `QA leave {QA.run}`. After S2, A and B are both still members of `QA stay {QA.run}`.

## Known failures on staging

- S3.2: the leave dialog has no "Sell and leave". Selling the stake before leaving is blocked by #657. Today a member with a stake gets the toast "Cash out your share of the pot before you leave this cabal." and a "Cash out" button (`cabalLeaveCashOutButton`) instead.

## Not covered

- The "Cash out" button a member with a stake gets after the refused leave. Reaching it needs a funded stake, and this journey moves no money.
- The last member leaving a cabal whose pot still holds money. That needs a funded pot too. `LeaveCabalModelTests` covers the copy.
