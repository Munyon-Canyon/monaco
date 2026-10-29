---
id: governance/vote
title: Vote on a proposal
version: 6
milestone: M13
requires: [auth/sign-in]
actors: [A, B]
flows: [10]
xcuitest: [apps/mobile/MonacoUITests/Journeys/GovernanceVoteJourney.swift, apps/mobile/MonacoUITests/Journeys/GovernanceVoteJourneyUITests.swift]
---

# Vote on a proposal

Two members of a cabal vote on a buy that a third member proposed. A picks and holds from Home, changes the ballot, and B takes a first ballot back with Undo before the vote that settles it. The rules are [Proposals](../../architecture/proposals.md).

A vote is a pick, then a 0.6 s hold: a short tap casts nothing. The counts are live in every state and neither side is highlighted before a pick. The vote that reaches the threshold marks the proposal Passed on the spot, with no Undo and no Change. Before settlement an ordinary first ballot toasts with Undo and the open card reads "You voted Yes" with Change; a change toasts "Changed to No." with no Undo. Nobody sees who voted which way on an open proposal; a cabal can turn on "Show who voted what after a vote closes" and then its closed proposals show the sides row. The push permission sheet "Hear how votes end?" follows the first vote's toast once per install.

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actors A and B have each signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend`, which also builds `bin/monacoctl` |
| P3 | `apps/mobile/qa/journeys/governance/vote.setup.sh` ran right before the scenario. It marks A and B as done with onboarding and sets their display names to the `name` column of `accounts.tsv`. It closes every open proposal A or B can vote on. A new dev user, "QA host", creates the cabal `QA vote {QA.run} <scenario>`, A and B join it, and QA host proposes a $5.00 buy with the reason `QA vote {QA.run}`, so 2 yes of 3 voters pass it ("Majority"). For S3 it also adds a closed proposal with the reason `QA closed {QA.run}`. For S4 it makes two cabals, one with "Show who voted what" on and one with it off, each with a passed proposal that QA host and A voted Yes on and B voted No on. It hands the test `cabalName`, `proposalID` and, for S3, `closedProposalID`, and for S4 `closedProposalID` (rule on) and `offProposalID` (rule off) |
| P4 | Each run reinstalls the app, so A's first vote in S1.4 is this install's first vote |

The setup seeds the proposal, so this journey does not depend on the propose screens.

## Scenarios

### S1 Vote from Home and the proposal screen

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap | the Home tab | | "Your vote" shows the proposal with `home-vote-yes-<proposalID>` and `home-vote-no-<proposalID>` within 15 s. Both show the live count "0" and neither is highlighted |
| S1.2 | A | tap | the card's row (`home-vote-open-<proposalID>`) | | The Proposal screen shows `proposal-stat-row` with "Price", "Shares", "Of treasury" and "Closes", the thesis "QA vote {QA.run}", `proposal-vote-yes` reading "Yes, 0 votes", `proposal-vote-no` reading "No, 0 votes" and `proposal-remind` "Remind me at close" within 15 s |
| S1.3 | A | tap | `proposal-vote-yes`, and let go before 0.6 s | | Nothing is cast: the counts still read "Yes, 0 votes" and "No, 0 votes" after 2 s, and no toast shows |
| S1.4 | A | tap, then hold | `proposal-vote-yes` | | Within 10 s `proposal-vote-yes` reads "Voted Yes, 1 vote", the status line reads "1 of 3 voted", the toast reads "Voted Yes. 1st of 3." with "Undo", and `proposal-remind` is gone. While the toast shows, `push-pre-prompt` does not. A leaves the toast alone. Within 10 s of the toast closing `push-pre-prompt` shows "Hear how votes end?", and "Not now" dismisses it. This step never taps Undo |
| S1.5 | A | tap, then pick and hold | `proposal-change-vote` "Change", then `proposal-vote-no`, then Change and `proposal-vote-yes` | | The card reads "You voted Yes" with `proposal-change-vote`. After the first change the tally reads "Yes, 0 votes" / "No, 1 vote" within 10 s, the toast reads "Changed to No." with no `monaco-toast-action`, and "Change" shows again. After the second it reads "Yes, 1 vote" / "No, 0 votes" and "Changed to Yes." with no Undo. The permission sheet does not show again |
| S1.6 | B | tap, then tap | the Home tab, then `home-vote-open-<proposalID>` | | The tally reads "Yes, 1 vote" / "No, 0 votes" and the status line "1 of 3 voted" within 15 s. There is no votes list with names: `proposal-votes-see-all` does not exist |
| S1.7 | B | pick and hold, then tap | `proposal-vote-no`, then the toast's "Undo" while it shows | | Within 10 s `proposal-vote-no` reads "Voted No, 1 vote" and the toast reads "Voted No. 2nd of 3." with "Undo". After Undo, within 10 s the tally reads "Yes, 1 vote" / "No, 0 votes", the toast reads "Vote undone.", both buttons can be picked and `proposal-change-vote` does not exist. No permission sheet shows |
| S1.8 | B | pick and hold | `proposal-vote-yes` | | Within 10 s the toast reads "Voted Yes. Passed 2–0." with no `monaco-toast-action`, `proposal-vote-yes` reads "Voted Yes, 2 votes", the status line starts "Passed 2–0", `proposal-change-vote` does not exist, and the stat row's last cell reads "Passed" |
| S1.9 | A | relaunch, then tap | the Home tab | | Within 15 s "Your vote" no longer offers the proposal for a vote (`home-vote-yes-<proposalID>` does not exist). If Home still shows it, opening it shows no `proposal-change-vote` |

### S2 Vote from the cabal page

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | open the cabal | the Cabals tab, then the cabal named `{cabalName}` | | `cabal-header-name` reads `{cabalName}` and `cabal-open-proposal-<proposalID>` shows above the tabs within 15 s |
| S2.2 | A | tap | `cabal-open-proposal-<proposalID>` | | The Proposal screen shows `proposal-stat-row` within 15 s |
| S2.3 | A | pick and hold | `proposal-vote-yes` | | The toast reads "Voted Yes." with "Undo" within 10 s, and `proposal-change-vote` shows |

### S3 All proposals

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S3.1 | A | tap, then tap | the Home tab, then "All proposals" in the "Your vote" header | | "All proposals" opens within 15 s |
| S3.2 | A | tap | the Closed segment (`proposals-segment-closed`) | | The open proposal shows under Open, and `proposal-row-<closedProposalID>` shows under Closed with "Expired" or "Failed", both within 15 s |

### S4 Sides after close

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S4.1 | A | open | the rule-on cabal's closed proposal from All proposals, Closed (`proposal-row-<closedProposalID>`) | | Within 15 s the closed tally shows "✓ 2" at one end and "1 ✕" at the other, and `proposal-sides` names "you" and QA host on the ✓ line and B on the ✕ line |
| S4.2 | A | open | the rule-off cabal's closed proposal (`proposal-row-<offProposalID>`) | | The closed result shows the counts and `proposal-sides` does not exist |

## Ground truth

`apps/mobile/qa/journeys/governance/vote.truth.sh` reads the hand-off. After S1, A's and B's ballots on `proposalID` are `yes`, the proposal has two ballot rows (A's change moved one ballot and B's undone No left none) and is no longer `open`. After S4, nothing changed.

## Known failures on staging

- S4.1 and S4.2 depend on #6433's `show_sides` rule and #6506's closed result card, which the setup seeds through `cabals.show_sides`.
- S2.1 finds a member's cabal by the `cabals-tile-<id>` icon and its label, not the `cabals-grid-<id>` identifier the ticket names, which the app does not set; `openCabal(named:)` falls back to the cabal search when no tile matches.

## Not covered

- The Proposal screen refreshing while it is open when B votes. The test signs A out while B acts and signs A in again after, so A never has the screen open while B votes. S1.9 checks the result after a relaunch. The gap is listed under Known failures with #612.
- The paused-cabal caption and the trade that runs after the vote passes. `trading/execute` covers the trade.
- A vote refused because the proposal closed or the member is not a voter. The flow layer covers those outcomes (flow 10).
- Undo after the window closes, and Undo on a changed ballot (flow layer; Q32).
