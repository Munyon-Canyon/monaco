---
id: governance/vote
title: Vote on a proposal
version: 5
milestone: F launch
requires: [auth/sign-in]
actors: [A, B]
flows: [10]
xcuitest: [apps/mobile/MonacoUITests/Journeys/GovernanceVoteJourney.swift, apps/mobile/MonacoUITests/Journeys/GovernanceVoteJourneyUITests.swift]
---

# Vote on a proposal

Two members of a cabal vote on a buy that a third member proposed. A picks and holds from Home, changes the ballot, and B takes a first ballot back with Undo before B's yes settles the vote at quorum. The rules are [Proposals](../../architecture/proposals.md).

In F a vote is a pick, then a 0.6 s hold, with no confirm dialog. The counts are live in every state and neither side is highlighted before a pick. An ordinary vote keeps Undo (a first ballot) and Change (an open ballot) until a vote settles the proposal. The vote that reaches the threshold marks the proposal Passed on the spot, and its toast has no Undo and its card has no Change. Nobody sees who voted which way on an open proposal; a cabal can turn on "Show who voted what after a vote closes" in Rules and the closed proposal then shows the sides row. The push permission sheet "Hear how votes end?" comes right after the first vote, once its toast has finished.

The vote buttons are `proposal-vote-yes` and `proposal-vote-no` on the Proposal screen and `home-vote-yes-<proposalID>` and `home-vote-no-<proposalID>` on Home's card. Their accessibility labels read "Yes, 0 votes" and, once committed, "Voted Yes, 1 vote". Copy in Expect quotes [screens.md](../../screens.md): `HomePendingVotesSlot`, `ProposalVoteSection` and `ProposalResultCard`.

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actors A and B have each signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend`, which also builds `bin/monacoctl` |
| P3 | `apps/mobile/qa/journeys/governance/vote.setup.sh` ran right before the scenario. It marks A and B as done with onboarding and sets their display names to the `name` column of `accounts.tsv`. It closes every open proposal A or B can vote on. A new dev user, "QA host", creates the cabal `QA vote {QA.run} <scenario>`, A and B join it, and QA host proposes a $5.00 buy with the reason `QA vote {QA.run}`. For S3 it also adds a closed proposal with the reason `QA closed {QA.run}`. For S4 it adds a second cabal `QA sides {QA.run}` whose "Show who voted what" rule is on, a closed proposal there that A and QA host voted Yes on and B voted No on, and the same closed proposal in the first cabal (rule off). It hands the test `cabalName`, `proposalID` and, for S3 and S4, `closedProposalID`, `sidesClosedProposalID` and `plainClosedProposalID` |

The setup seeds the proposal, so this journey does not depend on the propose screens. Each run reinstalls the app, so A's first vote in S1.4 is this install's first vote.

## Scenarios

### S1 Pick, hold, change, undo and settle

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap | the Home tab | | "Your vote" shows `home-vote-card-<proposalID>` within 15 s. `home-vote-yes-<proposalID>` reads "Yes, 0 votes" and `home-vote-no-<proposalID>` reads "No, 0 votes", neither selected |
| S1.2 | A | tap | `home-vote-open-<proposalID>` | | The Proposal screen shows `proposal-stat-row` with "Price", "Shares", "Of treasury" and "Closes", the title "Buy …", the thesis `proposal-reason` "QA vote {QA.run}", `proposal-vote-yes` "Yes, 0 votes", `proposal-vote-no` "No, 0 votes" and `proposal-remind` "Remind me at close", all within 15 s |
| S1.3 | A | tap | `proposal-vote-yes` | | Nothing is cast. After 2 s `proposal-vote-yes` still reads "Yes, 0 votes", `proposal-vote-no` reads "No, 0 votes" and no `monaco-toast-banner` shows |
| S1.4 | A | hold | `proposal-vote-yes` | 0.9 s | Within 10 s the toast reads "Voted Yes. 1st of 3." with `monaco-toast-action` "Undo", the card shows the vote (`proposal-vote-yes` reads "Voted Yes, 1 vote", or `proposal-voted` reads "You voted Yes"), `proposal-vote-status` reads "1 of 3 voted" first, and `proposal-remind` is gone. While `monaco-toast-banner` exists, `push-pre-prompt` does not. A leaves the toast alone |
| S1.5 | A | wait, then tap | `push-pre-prompt`, then `push-pre-prompt-not-now` | | Within 10 s of the toast closing, `push-pre-prompt` shows "Hear how votes end?". "Not now" closes it within 5 s. This step never taps Undo, because the sheet waits for the toast to end |
| S1.6 | A | tap, then hold | `proposal-change-vote`, then `proposal-vote-no` | 0.9 s | "You voted Yes" (`proposal-voted`) shows with `proposal-change-vote` "Change". After Change and the hold, the toast reads "Changed to No." within 10 s with no `monaco-toast-action`, and `proposal-voted` reads "You voted No" with "Change" again. A changes back to Yes the same way: the toast reads "Changed to Yes." with no Undo. `push-pre-prompt` does not show again |
| S1.7 | B | tap | the Home tab, then `home-vote-open-<proposalID>` | | The Proposal screen shows `proposal-vote-yes` reading "Yes, 1 vote" and `proposal-vote-no` reading "No, 0 votes" and `proposal-vote-status` reading "1 of 3 voted" within 15 s. There is no `proposal-votes-see-all`, and no ballot names an actor |
| S1.8 | B | hold, then tap | `proposal-vote-no`, then `monaco-toast-action` | 0.9 s | Within 10 s the toast reads "Voted No. 2nd of 3." with "Undo". B taps Undo while the toast shows. Within 10 s the toast reads "Vote undone.", `proposal-vote-yes` reads "Yes, 1 vote", `proposal-vote-no` reads "No, 0 votes", `proposal-change-vote` does not exist, and both buttons can be picked again. `push-pre-prompt` does not show |
| S1.9 | B | hold | `proposal-vote-yes` | 0.9 s | The settling vote. Within 10 s the toast reads "Voted Yes. Passed 2–0." with no `monaco-toast-action`, `proposal-vote-yes` reads "Voted Yes, 2 votes", the settled status line starts "Passed 2–0", `proposal-change-vote` does not exist, and the last cell of `proposal-stat-row` reads "Passed" with a time |
| S1.10 | A | relaunch, then tap | the Home tab | | `home-vote-card-<proposalID>` is not offered for a vote within 15 s |
| S1.11 | A | tap | "All proposals" in the "Your vote" header, `proposals-segment-closed`, then `proposal-row-<proposalID>` | | The Proposal screen shows `proposal-result` reading "Passed 2–0" within 15 s and no `proposal-change-vote`: Change ends at settlement |

### S2 Vote from the cabal page

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | tap | the Cabals tab, then the cabal's tile | `{cabalName}` | `cabal-header-name` reads `{cabalName}` within 15 s. The open proposal shows as `cabal-open-proposal-<proposalID>` above the tabs within 15 s |
| S2.2 | A | tap | `cabal-open-proposal-<proposalID>` | | The Proposal screen shows `proposal-vote-yes` and `proposal-reason` "QA vote {QA.run}" within 15 s |
| S2.3 | A | tap, then hold | `proposal-vote-yes` | 0.9 s | The toast reads "Voted Yes. 1st of 3." with "Undo" within 10 s, and `proposal-voted` reads "You voted Yes" with `proposal-change-vote` "Change" within 10 s |

### S3 All proposals

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S3.1 | A | tap | the Home tab, then "All proposals" in the "Your vote" header | | The "All proposals" screen shows within 15 s, `proposals-segment-open` is selected and the open `proposal-card-<proposalID>` shows under it |
| S3.2 | A | tap | `proposals-segment-closed` | | `proposal-row-<closedProposalID>` shows within 15 s and reads "Expired" or "Failed" |

### S4 Sides after close

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S4.1 | A | tap | the Home tab, "All proposals", `proposals-segment-closed`, then `proposal-row-<sidesClosedProposalID>` | | `proposal-result` shows within 15 s with `proposal-result-tally` reading "✓ 2" at one end and "1 ✕" at the other |
| S4.2 | A | read | `proposal-sides` | | `proposal-sides` shows within 15 s. Its "Yes:" line names QA host and A as "you", and its "No:" line names B |
| S4.3 | A | tap | the navigation bar's back button, then `proposal-row-<plainClosedProposalID>` | | `proposal-result` shows with its counts and no `proposal-sides` exists: that cabal's rule is off |

## Ground truth

`apps/mobile/qa/journeys/governance/vote.truth.sh` reads the hand-off. After S1, A's and B's ballots on S1's proposal are `yes`, the proposal is `passed` or `executed`, A's ballot is one row and B's undone No left no second row. After S4, the sides cabal's closed proposal still has its three ballots.

## Known failures on staging

None.

## Not covered

- Undo after the 15 s window closes and Undo on a changed ballot. The flow layer covers them (flow 10, `ProposalVoteCopy` and `RetractVote` tests).
- The Proposal screen refreshing while it is open when B votes. The test signs A out while B acts and signs A in again after, so A never has the screen open while B votes. S1.10 checks the result after a relaunch.
- Home's card pick, hold and "You voted Yes" (`04b-hold-1-pick` to `04b-hold-3-committed`). The Home card shares `VotePair` with the Proposal screen; S1.1 only checks its idle counts.
- The paused-cabal caption and the trade that runs after the vote passes. `trading/execute` covers the trade.
- A vote refused because the proposal closed or the member is not a voter. The flow layer covers those outcomes (flow 10).
