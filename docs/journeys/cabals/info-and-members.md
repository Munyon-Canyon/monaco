---
id: cabals/info-and-members
title: Members and voters on the cabal page
version: 3
milestone: UI v3
requires: [auth/sign-in]
actors: [A]
flows: []
xcuitest: [apps/mobile/MonacoUITests/Journeys/CabalsInfoAndMembersJourney.swift, apps/mobile/MonacoUITests/Journeys/CabalsInfoAndMembersJourneyUITests.swift]
---

# Members and voters on the cabal page

A cabal's creator opens the cabal through `openCabal(named:)`, reads the Members tab (admins, the record line, "See all"), then makes the second member a voter by proposal. Per founder decision Q4 the voters change by proposal, so S2 sends a "Make B a voter" proposal from the Voters tab, A holds Yes, and B shows under Voters. [cabals/edit-rules](edit-rules.md) covers the rules. The spec is [the cabal page](../../screens.md#cabal-screen-cabalroute). The rules are [Cabals](../../architecture/cabals.md#rules).

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actors A and B have signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv`. Only A drives the app |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py` starts the backend with `just run backend` |
| P3 | `apps/mobile/qa/journeys/cabals/info-and-members.setup.sh` ran right before S1. It marks A and B as done with onboarding and sets their display names. A creates the cabal `QA info {QA.run}` with "People I pick" for who votes, and B joins it through the API, so A votes and B does not. The setup hands the cabal id, A's and B's user ids and B's display name to the test |

## Scenarios

### S1 The Members tab

Starts signed in (auth/sign-in).

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | `openCabal(named:)` | the Cabals tab and the tile for the cabal | `QA info {QA.run}` | `cabal-header-name` reads `QA info {QA.run}` within 15 s |
| S1.2 | A | `openCabalTab("Members")` | `cabal-tab-members`, then `cabal-members` | | Within 10 s, the tab label reads "Members 2"; `cabal-member-<A id>` reads "Admin" and "you"; `cabal-member-<B id>` names B and reads the empty record "No votes this month"; `cabal-members-footer` ends "Make a member a voter by proposal." |
| S1.3 | A | tap, then back | `cabal-members-see-all` | | The screen titled "Members" (Members all) shows `members-list` within 15 s, and back returns to the cabal page |

### S2 Make B a voter by proposal

Starts where S1 left off: on the cabal page, Members tab.

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | `openCabalTab("Voters")` | `cabal-tab-voters`, then `cabal-voters` | | `cabal-member-<A id>` shows, and `cabal-member-<B id>` shows with "Holds a share" in its label (under Don't vote, not Vote) within 10 s |
| S2.2 | A | long press, then tap | `cabal-member-<B id>`, then `cabal-voters-propose-voter` ("Propose making a voter") | | The sheet "Propose a rule change" opens and `cabal-rule-proposal-words` reads "Make B a voter" within 10 s |
| S2.3 | A | tap | `cabal-rule-proposal-propose` | | The toast "Proposed. Your cabal votes on it." shows within 10 s |
| S2.4 | A | tap, then `pickAndHold("yes")` | the `cabal-open-proposal-<id>` row above the tabs | | A holds Yes; A is the only voter, so it passes |
| S2.5 | A | `openCabalTab("Voters")` | `cabal-member-<B id>` | | `cabal-member-<B id>`'s label no longer holds "Holds a share" (B moved under Vote) within 20 s |

## Ground truth

`apps/mobile/qa/journeys/cabals/info-and-members.truth.sh` reads `cabal_members`. `QA info {QA.run}` has two members and both can vote, so the proposal passed and reading the screens wrote nothing.

## Known failures on staging

None known.

## Not covered

- The Requests tab and approving a request. [cabals/approve-request](approve-request.md) and [cabals/join](join.md) cover them.
- Invite to this cabal and Leave cabal. [cabals/invite](invite.md) and [cabals/leave](leave.md) cover them.
- The Admins and Voters section headings with counts on the Members tab: the app shows one list with a role in each row's record line, not sections, so the journey asserts the row text. Follow-up against HC's Members tab ticket (#6534).
- Making B an admin, which shares the sheet and `cabal-voters-propose-admin`.
