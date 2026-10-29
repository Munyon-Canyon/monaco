---
id: cabals/join
title: Join a cabal
version: 9
milestone: M10
requires: [auth/sign-in]
actors: [A, B]
flows: [03]
xcuitest: [apps/mobile/MonacoUITests/Journeys/JoinJourney.swift, apps/mobile/MonacoUITests/Journeys/JoinJourneyUITests.swift]
---

# Join a cabal

Another member finds a cabal by search and joins it. An Open cabal takes them in at once. An Approval required cabal takes a request the creator approves or declines. The rules are [Cabals](../../architecture/cabals.md#join-and-access-requests).

The copy is [Join a cabal](../../screens.md#cabals-tab) and the [Cabal screen](../../screens.md#cabal-screen-cabalroute) in `screens.md`. Each Expect cell ends with the old-app tap it matches where the old app had one.

The format of this doc is in [App journeys](../README.md).

Every step that says "open the cabal" uses `openCabal(named:)`. Only S1.4 uses public search: the step reaches it from the Cabals tab by typing a name the actor has no cabal called, such as `zzqq`, in `cabals-filter-field` ("Find or search cabals"), then tapping `cabals-search-public` ("Search public cabals for …") or pressing the keyboard Search key, which pushes Discover cabals with that text in `cabals-search-field`; an actor with no cabals taps "Explore cabals" instead.

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actors A and B have each signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend`, which also builds `bin/monacoctl` |
| P3 | `apps/mobile/qa/journeys/cabals/join.setup.sh` ran right before the scenario. `scripts/qa/journey.py run` runs it. It marks A and B as done with onboarding and sets their display names to the `name` column of `accounts.tsv` (B is "Bartholomez"). It closes every pending join request A or B sent and every one waiting on a cabal A created. Then A creates the request cabal `QA pot {QA.run}`. The setup for S2 creates the approval cabal `QA open {QA.run}`, and the setup for S3 creates the approval cabal `QA gate {QA.run}` |

The setup creates both cabals through the API, so this journey does not depend on `cabals/create-cabal`.

## Scenarios

### S1 Request to join with approval

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | open | the cabal `QA pot {QA.run}` through `openCabal(named:)` | | `cabal-header-name` reads `QA pot {QA.run}` within 15 s |
| S1.2 | A | none | | | `cabal-member-count` reads "1 member" within 15 s |
| S1.3 | A | none | | | There is no `cabal-join-requests-heading`. A new request cabal has no pending requests |
| S1.4 | B | tap, type, then tap | the Cabals tab, `cabals-filter-field`, then `cabals-search-public` | `zzqq` | Discover cabals shows `cabals-search-empty` "No matching cabals" / "Try another name." within 10 s. Old app: the "No cabal called" search line in `CabalsTabView` |
| S1.5 | B | open | the cabal `QA pot {QA.run}` through `openCabal(named:)` | | `cabal-header-name` reads `QA pot {QA.run}` and `cabal-join-button` shows within 15 s. screens.md: `CabalJoinSlot`, "Request to join" |
| S1.6 | B | tap | `cabal-join-button` | | The toast "Request sent. You'll be in once the creator says yes." shows within 10 s. `cabal-join-requested` reads "Request pending" next to `cabal-join-cancel` within 15 s |
| S1.7 | A | open, tap, then tap | the cabal through `openCabal(named:)`, the Members tab, then `cabal-members-see-all` | | One `cabal-join-request-row` names B and reads "Asked to join" within 15 s |
| S1.8 | A | tap | `cabal-join-approve` in the row | | The toast "Approved." shows within 10 s. The request row is gone, and after the back button `cabal-member-count` reads "2 members" within 10 s |
| S1.9 | B | open | the cabal `QA pot {QA.run}` through `openCabal(named:)` | | `cabal-action-fund` shows within 15 s, and there is no `cabal-join-requested` or `cabal-join-button`. screens.md: `CabalActionsSlot` for a member |

### S2 Join an open cabal

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | open, tap, propose, then vote | the cabal `QA open {QA.run}`, the Rules tab, the `cabal-rules-join` row, "Open" and `cabal-rule-proposal-propose`, then `pickAndHold("yes")` | Open | A, the only voter, holds Yes. The Rules tab then reads "Open" on `cabal-rules-join` (rules change by proposal, Q4) |
| S2.2 | B | open | the cabal `QA open {QA.run}` through `openCabal(named:)` | | `cabal-join-policy` reads "Open: anyone can join." and `cabal-join-button` reads "Join" |
| S2.3 | B | tap | `cabal-join-button` | | The toast "You're in." shows, no request is sent, and `cabal-action-fund` shows within 15 s. There is no `cabal-join-button` |

### S3 Request declined, then ask again

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S3.1 | B | open, then tap | the cabal `QA gate {QA.run}`, then `cabal-join-button` | | `cabal-join-policy` reads "Approval required: the creator reviews each request.". The toast "Request sent. You'll be in once the creator says yes." shows, and `cabal-join-requested` reads "Request pending" |
| S3.2 | A | open, tap, long press, then tap | the cabal `QA gate {QA.run}`, the Members tab, `cabal-members-see-all`, the `cabal-join-request-row`, then "Decline" (`cabal-join-deny`) in its menu | | The toast "Denied." shows |
| S3.3 | B | open | the cabal `QA gate {QA.run}` | | `cabal-join-declined` reads "Request declined" and `cabal-join-button` reads "Ask again" within 15 s |
| S3.4 | B | tap | `cabal-join-button` | | The toast "Request sent. You'll be in once the creator says yes." shows, and `cabal-join-requested` reads "Request pending" |

If the creator changes the policy between taps, the app takes the other route once: an open join refused with `join_needs_request` sends a request, and a request refused with `request_not_needed` joins.

## Ground truth

After S1, B is a member of both seeded cabals, and B's request to `QA pot {QA.run}` is `approved` in `cabal_access_requests`. There is no truth script yet; the toasts are the server's answers.

## Known failures on staging

- S2.1: the Rules tab's `cabal-rules-join` row is read-only and `CabalRuleProposalModel` has no join-policy change yet, so the step cannot propose "Who can join · Open". Owning tickets: #6498 and #6510.

The ticket names the request row `members-request-<id>`. The app builds `cabal-join-request-row`, `cabal-join-approve` and `cabal-join-deny` (Decline sits in the row's long-press menu), and the journey follows the app. The request rows list in Members · all (`cabal-members-see-all`).

## Not covered

- A seeing B's request arrive on a screen already open, and B's screen turning into a member's without a relaunch. The test signs each actor in again when the actor changes, so S1.7 and S1.9 load the cabal fresh. `CabalAccessModelTests` covers the hint that refreshes an open screen.
- Cancel request. `CabalAccessModelTests` covers it on the host.
- The policy changing between a screen load and a tap. `CabalEntryTests` covers both fallbacks on the host.
