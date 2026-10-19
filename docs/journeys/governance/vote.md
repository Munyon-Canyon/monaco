---
id: governance/vote
title: Vote on a proposal
version: 4
milestone: M13
requires: [auth/sign-in]
actors: [A, B]
flows: [10]
xcuitest: [apps/mobile/MonacoUITests/Journeys/GovernanceVoteJourney.swift, apps/mobile/MonacoUITests/Journeys/GovernanceVoteJourneyUITests.swift]
---

# Vote on a proposal

Two members of a cabal vote on a buy that a third member proposed. A votes from Home's "Needs your vote", changes the ballot twice, and B's yes makes the majority. The rules are [Proposals](../../architecture/proposals.md).

Members vote on the Proposal screen, not on the card. The "Needs your vote" card (`ProposalCard`) offers "Review" and a footer that open it, and the dock above the composer carries "Vote No" and "Vote Yes", then "Change vote". Copy in Expect quotes [screens.md](../../screens.md): `HomePendingVotesSlot`, `CabalProposalsSlot`, the Proposal card and `ProposalDetailSlot`.

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actors A and B have each signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend`, which also builds `bin/monacoctl` |
| P3 | `apps/mobile/qa/journeys/governance/vote.setup.sh` ran right before the scenario. It marks A and B as done with onboarding and sets their display names to the `name` column of `accounts.tsv`. It closes every open proposal A or B can vote on. A new dev user, "QA host", creates the cabal `QA vote {QA.run} <scenario>`, A and B join it, and QA host proposes a $5.00 buy with the reason `QA vote {QA.run}`. For S3 it also adds a closed proposal with the reason `QA closed {QA.run}`. It hands the test `cabalName`, `proposalID` and, for S3, `closedProposalID` |

The setup seeds the proposal, so this journey does not depend on the propose screens (#613).

## Scenarios

### S1 Vote from Home and the proposal screen

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap | the Home tab | | "Needs your vote" shows with `proposal-card-<proposalID>` within 15 s. The card reads "Needs your vote", "2 yes votes needed" and "Closes in" |
| S1.2 | A | tap | the footer of `proposal-card-<proposalID>` | | The Proposal screen shows "Votes", "Thesis" and "QA vote {QA.run}" within 15 s |
| S1.3 | A | tap | "Vote Yes" in the dock | | The toast "Vote recorded." shows within 10 s. The dock shows `proposal-change-vote`, and the vote bar reads "1 yes, 0 no, 2 not voted" within 10 s |
| S1.4 | A | tap, then tap | "Change vote", then "Vote No" | | The toast "Vote recorded." shows within 10 s, and the vote bar reads "0 yes, 1 no, 2 not voted" within 10 s |
| S1.5 | A | tap, then tap | "Change vote", then "Vote Yes" | | The toast "Vote recorded." shows within 10 s, and the vote bar reads "1 yes, 0 no, 2 not voted" within 10 s |
| S1.6 | B | tap, then tap, then tap | the Home tab, the footer of `proposal-card-<proposalID>`, then `proposal-votes-see-all` | | The Proposal screen shows the vote bar "1 yes, 0 no, 2 not voted" within 15 s, and the Votes sheet shows "Alfred voted yes" within 15 s. "Close" dismisses it |
| S1.7 | B | tap | "Vote Yes" in the dock | | The toast "Vote recorded." shows within 10 s. Within 15 s the dock shows no "Vote Yes", "Vote No" or "Change vote": the majority closed the vote |
| S1.8 | A | relaunch, then tap | the Home tab | | `proposal-card-<proposalID>` is not under "Needs your vote" within 15 s |

### S2 Vote on the cabal's card

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | tap, type, then tap | the Cabals tab, `cabals-search-field`, then the `cabals-search-result-<id>` | `{cabalName}` | `cabal-header-name` reads `{cabalName}` within 15 s. "Needs your vote" shows with `proposal-card-<proposalID>` and "See all" within 15 s |
| S2.2 | A | tap | the footer of `proposal-card-<proposalID>` | | The Proposal screen shows "Votes" and "Thesis" within 15 s |
| S2.3 | A | tap | "Vote Yes" in the dock | | The toast "Vote recorded." shows within 10 s, and the dock shows `proposal-change-vote` within 10 s |

### S3 See all with the closed history

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S3.1 | A | tap, type, then tap | the Cabals tab, `cabals-search-field`, then the `cabals-search-result-<id>` | `{cabalName}` | "Needs your vote" shows with `proposal-card-<proposalID>` and "See all" within 15 s |
| S3.2 | A | tap, then tap | "See all", then "Failed" | | `proposal-card-<proposalID>` shows under Open, and `proposal-card-<closedProposalID>` shows under Failed, both within 15 s. The closed card reads "Expired" and "Voting ended without enough yes votes." |

## Ground truth

`apps/mobile/qa/journeys/governance/vote.truth.sh` reads the hand-off. After S1, A's and B's ballots on `proposalID` are `yes` and the proposal is no longer `open`.

## Known failures on staging

None.

## Not covered

- The Proposal screen refreshing while it is open when B votes. The test signs A out while B acts and signs A in again after, so A never has the screen open while B votes. S1.8 checks the result after a relaunch. The gap is listed under Known failures with #612.
- Accessibility identifiers on "Needs your vote" and "See all" in `apps/mobile/Monaco/Features/Home/HomePendingVotesSlot.swift` and `apps/mobile/Monaco/Features/Groups/CabalSlots/CabalProposalsSlot.swift`. The test finds them by label. The dock buttons carry `proposal-vote-yes`, `proposal-vote-no` and `proposal-change-vote`.
- The paused-cabal caption and the trade that runs after the vote passes. `trading/execute` covers the trade.
- A vote refused because the proposal closed or the member is not a voter. The flow layer covers those outcomes (flow 10).
