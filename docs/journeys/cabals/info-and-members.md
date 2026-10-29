---
id: cabals/info-and-members
title: Cabal information and Members & voters
version: 1
milestone: UI v3
requires: [auth/sign-in]
actors: [A]
flows: []
xcuitest: [apps/mobile/MonacoUITests/Journeys/CabalsInfoAndMembersJourney.swift, apps/mobile/MonacoUITests/Journeys/CabalsInfoAndMembersJourneyUITests.swift]
---

# Cabal information and Members & voters

A cabal's creator taps the information round on the cabal screen, opens Members & voters from the information screen, reads who is in and who votes, then picks the second member as a voter and saves. [cabals/edit-rules](edit-rules.md) covers the rules on the information screen and picking voters inside Cabal settings, so this journey is the Members & voters screen and its "Choose voters" sheet. The spec is [Cabal information](../../screens.md#cabal-screen-cabalroute) and [Members & voters](../../screens.md#members-voters-membersroute). The rules are [Cabals](../../architecture/cabals.md#rules).

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actors A and B have signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv`. Only A drives the app |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py` starts the backend with `just run backend` |
| P3 | `apps/mobile/qa/journeys/cabals/info-and-members.setup.sh` ran right before S1. It marks A and B as done with onboarding and sets their display names. A creates the cabal `QA info {QA.run}` with "People I pick" for who votes, and B joins it through the API, so A votes and B does not. The setup hands the cabal id, A's and B's user ids and B's display name to the test |

## Scenarios

### S1 Cabal information and the member list

Starts signed in (auth/sign-in).

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap, type, then tap | the Cabals tab, `cabals-search-field` in Discover cabals, then the `cabals-search-result-<id>` | `QA info {QA.run}` | `cabal-header-name` reads `QA info {QA.run}` within 15 s. screens.md: the cabal screen |
| S1.2 | A | tap | `cabal-details-button` | | Within 15 s, the screen titled "Cabal information" shows `cabal-members-row` reading "Members & voters" and "2 members". screens.md: the toolbar information button pushes `CabalInfoRoute`, with the rows "Members & voters", "Invite to this cabal" |
| S1.3 | A | tap | `cabal-members-row` | | Within 15 s, the screen titled "Members & voters" shows the segments `members-tab-members`, `members-tab-voters` and `members-tab-requests` (the creator sees Requests), and the helper line reads "2 members · 1 eligible voter". screens.md: Members, Voters and, for the creator, Requests; "8 members · 8 eligible voters" |
| S1.4 | A | read | the two rows of `members-list` (the rows carry the list's identifier, so they are found by label) | | Within 10 s, A's row reads "Admin" and B's row reads "Member" and B's display name. screens.md: a row per member with "Admin" or "Member" |

### S2 Pick a voter and save

Starts where S1 left off: on Members & voters, Members.

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | tap | `members-tab-voters` | | Within 10 s, `voters-member-<A id>` reads "Voter" and `voters-member-<B id>` reads "Not voting", and `members-choose-voters` shows reading "Choose voters". screens.md: a "Voter" or "Not voting" badge per member; a creator who picks voters sees "Choose voters" |
| S2.2 | A | tap | `members-choose-voters` | | Within 10 s, the sheet "Eligible voters" shows `edit-cabal-voters` listing both members, and `edit-cabal-voters-save` ("Save voters") is disabled. screens.md: the "Eligible voters" sheet with a pinned "Save voters" |
| S2.3 | A | tap | `voters-member-<B id>` inside `edit-cabal-voters` | | Within 5 s, B's row is selected and `edit-cabal-voters-save` is enabled |
| S2.4 | A | tap | `edit-cabal-voters-save` | | The toast "Voters saved." shows within 10 s, the sheet closes, `voters-member-<B id>` reads "Voter" and the helper line reads "2 members · 2 eligible voters" within 10 s. screens.md: a pinned "Save voters", toast "Voters saved." |

## Ground truth

`apps/mobile/qa/journeys/cabals/info-and-members.truth.sh` reads `cabal_members`. `QA info {QA.run}` has two members and both can vote, so Save voters wrote the change and reading the screens wrote nothing.

## Known failures on staging

None known.

## Not covered

- The Requests tab and approving a request. [cabals/approve-request](approve-request.md) and [cabals/join](join.md) cover them.
- Invite to this cabal, "Cabal treasury", "View your share" and Leave cabal. [cabals/invite](invite.md) and [cabals/leave](leave.md) cover the invite and the leave.
- A member's view of the screen, which hides Requests and "Choose voters". It needs B to drive the app, and the creator-only parts are what this journey is for.
- Removing a voter. Save voters with B unticked sends the same request in reverse, and `CabalEditModelTests` covers it.
