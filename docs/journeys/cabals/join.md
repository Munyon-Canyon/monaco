---
id: cabals/join
title: Join a cabal
version: 1
milestone: M10
requires: [auth/sign-in]
actors: [A, B]
flows: [03]
xcuitest: [apps/mobile/MonacoUITests/Journeys/JoinJourney.swift, apps/mobile/MonacoUITests/Journeys/JoinJourneyUITests.swift]
---

# Join a cabal

The creator of a request cabal copies its invite code. Another member enters the code, asks to join, and the creator approves. The same member then finds an open cabal by name and joins it in one tap. The rules are [Cabals](../../architecture/cabals.md#join-and-access-requests).

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actors A and B have each signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend`, which also builds `bin/monacoctl` |
| P3 | `apps/mobile/qa/journeys/cabals/join.setup.sh` ran right before the scenario. `scripts/qa/journey.py run` runs it. It marks A and B as done with onboarding and sets their display names to the `name` column of `accounts.tsv` (B is "Bartholomez"). It closes every pending join request A or B sent and every one waiting on a cabal A created. Then A creates the request cabal `QA pot {QA.run}` and a new dev user creates the open cabal `QA open {QA.run}` |

The setup creates both cabals through the API, so this journey does not depend on `cabals/create-cabal`.

## Scenarios

### S1 Join by code with approval, then join an open cabal

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap, then type | the Cabals tab, then `cabals-search-field` | `QA pot {QA.run}` | One `cabals-search-result-<id>` names `QA pot {QA.run}` within 10 s |
| S1.2 | A | tap | the `cabals-search-result-<id>` | | `cabal-header-name` reads `QA pot {QA.run}` and `cabal-member-count` reads "1 member" within 15 s |
| S1.3 | A | tap | `cabal-details-button` | | `cabal-invite-card` shows within 10 s. `cabal-invite-code` is 10 characters, `cabal-invite-copy` reads "Copy code" and `cabal-invite-share` reads "Share". The test hands the code to B |
| S1.4 | A | tap | `cabal-invite-copy` | | `cabal-invite-copy` reads "Copied" within 2 s |
| S1.5 | A | tap | `cabal-details-done` | | `cabal-details-button` shows within 10 s, and there is no `cabal-join-requests-heading` |
| S1.6 | B | tap, then type | the Cabals tab, then `cabals-search-field` | `zzqq` | `cabals-search-empty` reads "No cabal called “zzqq”" within 10 s |
| S1.7 | B | clear, then tap | `cabals-search-field`, then `cabals-new-button`, then `new-cabal-join-row` | | The "Join a cabal" screen shows `join-group-id` within 10 s |
| S1.8 | B | tap | `join-group-paste` | the code from S1.3, on B's clipboard | `join-group-name` reads `QA pot {QA.run}` and `join-group-submit` reads "Request to join" within 10 s |
| S1.9 | B | tap | `join-group-submit` | | The toast "Request sent. You'll be in once the creator says yes." shows within 10 s. `cabal-header-name` reads `QA pot {QA.run}`, and `cabal-join-requested` reads "Request sent" next to `cabal-join-cancel` within 15 s |
| S1.10 | A | tap, type, then tap | the Cabals tab, `cabals-search-field`, then the `cabals-search-result-<id>` | `QA pot {QA.run}` | `cabal-join-requests-heading` reads "1 person wants to join" within 2 s of `cabal-header-name` showing, with one `cabal-join-request-row` naming B |
| S1.11 | A | tap | `cabal-join-approve` | | The toast "Approved." shows within 10 s. `cabal-join-requests-heading` is gone, and `cabal-member-count` reads "2 members" within 10 s |
| S1.12 | B | tap, type, then tap | the Cabals tab, `cabals-search-field`, then the `cabals-search-result-<id>` | `QA pot {QA.run}` | `cabal-action-fund` shows within 15 s, and there is no `cabal-join-requested` or `cabal-join-button` |
| S1.13 | B | tap, clear, type, then tap | the Cabals tab, `cabals-search-field`, then the `cabals-search-result-<id>` | `QA open {QA.run}` | `cabal-join-button` reads "Join cabal" within 15 s |
| S1.14 | B | tap | `cabal-join-button` | | The toast "You're in." shows within 10 s, and `cabal-member-count` reads "2 members" within 15 s |

## Ground truth

After S1, B is a member of both seeded cabals, and B's request to `QA pot {QA.run}` is `approved` in `cabal_access_requests`. There is no truth script yet; the toasts are the server's answers.

## Known failures on staging

None known.

## Not covered

- B pasting the code A copied. Each actor has a simulator of its own, and simulators do not share a clipboard, so S1.8 puts the code on B's clipboard before tapping Paste.
- A seeing B's request arrive on a screen already open, and B's screen turning into a member's without a relaunch. Each phase relaunches the app on its actor's simulator, so S1.10 and S1.12 load the cabal fresh. `CabalAccessModelTests` covers the hint that refreshes an open screen.
- Deny and Cancel request. `CabalAccessModelTests` covers both on the host.
- Share. It opens the system share sheet, which the journey does not drive.
- The member actions after S1.14's in-place join. They stay hidden until the cabal is reopened (#2320); S1.12 checks them after a reopen.
