---
id: cabals/approve-request
title: Ask to join, cancel, and the creator's answer
version: 5
milestone: M10
requires: [auth/sign-in]
actors: [A, B]
flows: [03]
xcuitest: [apps/mobile/MonacoUITests/Journeys/CabalsApproveRequestJourney.swift, apps/mobile/MonacoUITests/Journeys/CabalsApproveRequestJourneyUITests.swift]
---

# Ask to join, cancel, and the creator's answer

A member who is not in a request cabal asks to join from the cabal screen, changes their mind, and asks again. The creator sees the request as the first row of the Members tab and declines it. In a second scenario the creator approves a request the setup made. The rules are [Cabals](../../architecture/cabals.md#join-and-access-requests). `cabals/join` covers asking after a search; this journey covers the cabal screen's own buttons, Cancel and Decline. A step that says "open the cabal" uses `openCabal(named:)`, and the request row sits in Members · all (`cabal-members-see-all`), where the app lists pending requests.

Old app (`c838bd24`): Cabal (non-member) -> "Request to join" -> "Request sent" -> Cancel (`JoinGroupView.swift`); creator: `GroupDetailView.swift` "N people want to join" -> Approve or Deny. Spec: `CabalJoinSlot` in [screens.md](../../screens.md#cabal-screen-cabalroute).

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actors A and B have each signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend`, which also builds `bin/monacoctl` |
| P3 | `apps/mobile/qa/journeys/cabals/approve-request.setup.sh` ran right before the scenario. It marks A and B as done with onboarding, sets their display names to the `name` column of `accounts.tsv` (B is "Bartholomez") and closes every pending join request A or B sent or that waits on a cabal A created. For S1, A creates the request cabal `QA ask {QA.run}`. For S2, A creates the request cabal `QA ok {QA.run}` and B asks to join it through the API |

## Scenarios

### S1 Ask, cancel, ask again, and the creator denies

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | B | open | the cabal through `openCabal(named:)` | `QA ask {QA.run}` | `cabal-header-name` reads `QA ask {QA.run}` and `cabal-join-button` reads "Request to join" within 15 s (old app: the non-member cabal screen's "Request to join") |
| S1.2 | B | tap | `cabal-join-button` | | The toast "Request sent. You'll be in once the creator says yes." shows within 10 s. `cabal-join-requested` reads "Request pending" next to `cabal-join-cancel` "Cancel request" within 10 s |
| S1.3 | B | tap | `cabal-join-cancel` | | `cabal-join-button` reads "Request to join" again within 10 s, and there is no `cabal-join-requested` |
| S1.4 | B | tap | `cabal-join-button` | | `cabal-join-requested` reads "Request pending" within 10 s |
| S1.5 | A | open, then tap | the cabal through `openCabal(named:)`, the Members tab, then `cabal-members-see-all` | `QA ask {QA.run}` | One `cabal-join-request-row` names B, reads "Asked to join" and holds `cabal-join-approve` "Approve B" within 15 s |
| S1.6 | A | long press, then tap | the `cabal-join-request-row`, then "Decline" (`cabal-join-deny`) in its menu | | The toast "Declined." shows within 10 s. The request row is gone, and after the back button `cabal-member-count` still reads "1 member" |

### S2 The creator approves

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | open, then tap | the cabal through `openCabal(named:)`, the Members tab, then `cabal-members-see-all` | `QA ok {QA.run}` | One `cabal-join-request-row` names B and reads "Asked to join" within 15 s |
| S2.2 | A | tap | `cabal-join-approve` in the row | | The toast "Approved." shows within 10 s. The request row is gone, and after the back button `cabal-member-count` reads "2 members" within 10 s |

## Ground truth

`apps/mobile/qa/journeys/cabals/approve-request.truth.sh` reads `cabal_access_requests` for B on both cabals. After S1, B's newest request to `QA ask {QA.run}` is `denied`, an older one is `revoked`, and B is not a member. After S2, B's request to `QA ok {QA.run}` is `approved` and B is a member.

## Known failures on staging

None known.

The ticket names the request row `members-request-<id>` and its button `members-request-approve-<id>`. The app builds them as `cabal-join-request-row`, `cabal-join-approve` and `cabal-join-deny` (Decline sits in the row's long-press menu and swipe), and the journey follows the app.

## Not covered

- B's screen turning into a member's screen without a relaunch after A approves. `CabalAccessModelTests` covers the hint that refreshes an open screen, and `cabals/join` checks it after a reopen.
- Asking from a search row's "Request" button. `cabals/browse` covers it.
