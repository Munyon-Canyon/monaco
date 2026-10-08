---
id: cabals/join
title: Join a cabal
version: 5
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
| S1.1 | A | tap, then type | the Cabals tab, then `cabals-search-field` | `QA pot {QA.run}` | One `cabals-search-result-<id>` names `QA pot {QA.run}` within 10 s. Old app: the "Find a cabal by name" search in `CabalsTabView` |
| S1.2 | A | tap | the `cabals-search-result-<id>` | | `cabal-header-name` reads `QA pot {QA.run}` and `cabal-member-count` reads "1 member" within 15 s. Old app: a search result pushed `GroupDetailView` |
| S1.3 | A | none | | | There is no `cabal-join-requests-heading`. A new request cabal has no pending requests |
| S1.4 | B | tap, then type | the Cabals tab, then `cabals-search-field` | `zzqq` | `cabals-search-empty` reads "No cabal called “zzqq”" within 10 s. Old app: the same empty search line in `CabalsTabView` |
| S1.5 | B | clear, type, then tap | `cabals-search-field`, then the `cabals-search-result-<id>` | `QA pot {QA.run}` | `cabal-header-name` reads `QA pot {QA.run}` and `cabal-join-button` shows within 15 s. screens.md: `CabalJoinSlot`, "Request to join" |
| S1.6 | B | tap | `cabal-join-button` | | The toast "Request sent. You'll be in once the creator says yes." shows within 10 s. `cabal-join-requested` reads "Request sent" next to `cabal-join-cancel` within 15 s |
| S1.7 | A | tap, type, then tap | the Cabals tab, `cabals-search-field`, then the `cabals-search-result-<id>` | `QA pot {QA.run}` | `cabal-join-requests-heading` reads "1 person wants to join" within 2 s of `cabal-header-name` showing, with one `cabal-join-request-row` naming B. screens.md: `CabalJoinSlot`, "2 people want to join" with Approve and Deny. Old app: the join requests on `GroupDetailView` |
| S1.8 | A | tap | `cabal-join-approve` | | The toast "Approved." shows within 10 s. `cabal-join-requests-heading` is gone, and `cabal-member-count` reads "2 members" within 10 s. Old app: Approve on `GroupDetailView` toasted "<name> is in"; staging toasts "Approved." |
| S1.9 | B | tap, type, then tap | the Cabals tab, `cabals-search-field`, then the `cabals-search-result-<id>` | `QA pot {QA.run}` | `cabal-action-fund` shows within 15 s, and there is no `cabal-join-requested` or `cabal-join-button`. screens.md: `CabalActionsSlot` for a member. Old app: `GroupDetailView`'s member action row |

### S2 Join an open cabal

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | tap, then edit | the cabal `QA open {QA.run}`, then the `cabal-rules-join` row and its `edit-rule-join` row | Open | Save enables, and after Save the toast "Cabal updated." shows. The `cabal-rules-join` row reads "Open" |
| S2.2 | B | tap, type, then tap | the Cabals tab, `cabals-search-field`, then the `cabals-search-result-<id>` | `QA open {QA.run}` | The row reads "Open", and `cabal-join-policy` reads "Open: anyone can join." `cabal-join-button` reads "Join" |
| S2.3 | B | tap | `cabal-join-button` | | The toast "You're in." shows, no request is sent, and `cabal-action-fund` shows within 15 s. There is no `cabal-join-button` |

### S3 Request declined, then ask again

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S3.1 | B | open, then tap | the cabal `QA gate {QA.run}`, then `cabal-join-button` | | `cabal-join-policy` reads "Approval required: the creator reviews each request.". The toast "Request sent. You'll be in once the creator says yes." shows, and `cabal-join-requested` reads "Request sent" |
| S3.2 | A | open, then tap | the cabal `QA gate {QA.run}`, then `cabal-join-deny` | | The toast "Denied." shows |
| S3.3 | B | open | the cabal `QA gate {QA.run}` | | `cabal-join-declined` reads "Request declined" and `cabal-join-button` reads "Ask again" within 15 s |
| S3.4 | B | tap | `cabal-join-button` | | The toast "Request sent. You'll be in once the creator says yes." shows, and `cabal-join-requested` reads "Request sent" |

If the creator changes the policy between taps, the app takes the other route once: an open join refused with `join_needs_request` sends a request, and a request refused with `request_not_needed` joins.

## Ground truth

After S1, B is a member of both seeded cabals, and B's request to `QA pot {QA.run}` is `approved` in `cabal_access_requests`. There is no truth script yet; the toasts are the server's answers.

## Known failures on staging

None known.

## Not covered

- A seeing B's request arrive on a screen already open, and B's screen turning into a member's without a relaunch. The test signs each actor in again when the actor changes, so S1.7 and S1.9 load the cabal fresh. `CabalAccessModelTests` covers the hint that refreshes an open screen.
- Cancel request. `CabalAccessModelTests` covers it on the host.
- The policy changing between a screen load and a tap. `CabalEntryTests` covers both fallbacks on the host.
