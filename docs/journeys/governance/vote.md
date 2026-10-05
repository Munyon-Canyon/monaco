---
id: governance/vote
title: Vote on a proposal
version: 1
milestone: M13
requires: [auth/sign-in]
actors: [A, B]
flows: [10]
xcuitest: [apps/mobile/MonacoUITests/Journeys/GovernanceVoteJourney.swift, apps/mobile/MonacoUITests/Journeys/GovernanceVoteJourneyUITests.swift]
---

# Vote on a proposal

Two members of a cabal vote on a buy that a third member proposed. A votes from Home's "Needs your vote", changes the ballot twice, and B's yes makes the majority. The rules are [Proposals](../../architecture/proposals.md).

The old app (`c838bd24`) did this from the cabal screen: the "Needs your vote" card (`ProposalHistorySection`, `ProposalCardView`) with "Yes" / "No" and the toast "Vote in", the card opening `ProposalDetailView` with the same buttons and "Change", and "See all" to the open and closed list. Copy in Expect quotes [screens.md](../../screens.md): `HomePendingVotesSlot`, `CabalProposalsSlot`, the Proposal card and `ProposalDetailSlot`.

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actors A and B have each signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend`, which also builds `bin/monacoctl` |
| P3 | `apps/mobile/qa/journeys/governance/vote.setup.sh` ran right before the scenario. It marks A and B as done with onboarding and sets their display names to the `name` column of `accounts.tsv`. It closes every open proposal A or B can vote on. A new dev user, "QA host", creates the open cabal `QA vote {QA.run} <scenario>`, A and B join it, and QA host proposes a $5.00 buy with the reason `QA vote {QA.run}`. For S3 it also adds a closed proposal with the reason `QA closed {QA.run}`. It hands the test `cabalName`, `proposalID` and, for S3, `closedProposalID` |

The setup seeds the proposal, so this journey does not depend on the propose screens (#613).

## Scenarios

### S1 Vote from Home and the proposal screen

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap | the Home tab | | "Needs your vote" shows with `proposal-card-<proposalID>` within 15 s. The card reads "Closes in", "0 of 3 voted · 2 yes to pass", "Yes" and "No" |
| S1.2 | A | tap | `proposal-card-<proposalID>` | | The Proposal screen shows "Votes", "Why buy" and "“QA vote {QA.run}”" within 15 s |
| S1.3 | A | tap | "Yes" on `proposal-card-<proposalID>` | | The toast "Vote in" shows within 10 s. The card reads "✓ You voted yes" and "Change", and "1 of 3 voted · 2 yes to pass" within 10 s |
| S1.4 | A | tap, then tap | "Change", then "No" | | The toast "Vote in" shows within 10 s, and the card reads "✓ You voted no" within 10 s |
| S1.5 | A | tap, then tap | "Change", then "Yes" | | The toast "Vote in" shows within 10 s, and the card reads "✓ You voted yes" within 10 s |
| S1.6 | B | tap, then tap | the Home tab, then `proposal-card-<proposalID>` | | The Proposal screen shows "Alfred voted yes" within 15 s |
| S1.7 | B | tap | "Yes" on `proposal-card-<proposalID>` | | The toast "Vote in" shows within 10 s. Within 15 s the card no longer reads "Closes in" and shows no "Yes", "No" or "Change": the majority closed the vote |
| S1.8 | A | relaunch, then tap | the Home tab | | `proposal-card-<proposalID>` is not under "Needs your vote" within 15 s |

### S2 Vote on the cabal's card

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | tap, type, then tap | the Cabals tab, `cabals-search-field`, then the `cabals-search-result-<id>` | `{cabalName}` | `cabal-header-name` reads `{cabalName}` within 15 s. "Needs your vote" shows with `proposal-card-<proposalID>` and "See all" within 15 s |
| S2.2 | A | tap | "Yes" on `proposal-card-<proposalID>` | | The toast "Vote in" shows within 10 s, and the card reads "✓ You voted yes" within 10 s |
| S2.3 | A | tap | `proposal-card-<proposalID>` | | The Proposal screen shows "Votes" and "Why buy" within 15 s |

### S3 See all with the closed history

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S3.1 | A | tap, type, then tap | the Cabals tab, `cabals-search-field`, then the `cabals-search-result-<id>` | `{cabalName}` | "Needs your vote" shows with `proposal-card-<proposalID>` and "See all" within 15 s |
| S3.2 | A | tap | "See all" | | `proposal-card-<proposalID>` and `proposal-card-<closedProposalID>` both show within 15 s. The closed card shows its status chip ("Expired") and no "Yes" or "No" |

## Ground truth

`apps/mobile/qa/journeys/governance/vote.truth.sh` reads the hand-off. After S1, A's and B's ballots on `proposalID` are `yes` and the proposal is no longer `open`.

## Known failures on staging

| Step | What fails | Blocked by |
| --- | --- | --- |
| S2.2 | The cabal card's "Yes" and "No" do nothing: `CabalProposalsSlot` builds `ProposalCard` without a `vote:` closure, so no ballot is sent and no toast shows | #612 |
| S2.3 | The cabal card does not open `ProposalRoute`: it is not wrapped in a navigation link | #612 |
| S3.2 | "See all" pushes the same open-only, needs-your-vote list again (`CabalProposalListRoute`), so no closed proposal shows | #612 |
| none | The Proposal screen does not refresh live after another member votes. No step can show it, see Not covered | #612 |

## Not covered

- The Proposal screen refreshing while it is open when B votes. Each phase relaunches the app on its actor's simulator, so A never has the screen open while B votes. S1.8 checks the result after a relaunch. The gap is listed under Known failures with #612.
- Accessibility identifiers on the vote buttons. "Yes", "No" and "Change" in `apps/mobile/Monaco/Features/Proposals/ProposalCard.swift`, and "Needs your vote" and "See all" in `apps/mobile/Monaco/Features/Home/HomePendingVotesSlot.swift` and `apps/mobile/Monaco/Features/Groups/CabalSlots/CabalProposalsSlot.swift`, have no identifier. The test finds them by label inside `proposal-card-<proposalID>`. Adding identifiers is #612's, because `Features/Proposals` is outside this ticket.
- The paused-cabal caption and the trade that runs after the vote passes. `trading/execute` covers the trade.
- A vote refused because the proposal closed or the member is not a voter. The flow layer covers those outcomes (flow 10).
