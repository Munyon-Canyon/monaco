---
id: governance/withdraw
title: Withdraw a proposal
version: 2
milestone: M13
requires: [auth/sign-in]
actors: [A, B]
flows: [13]
xcuitest: [apps/mobile/MonacoUITests/Journeys/GovernanceWithdrawJourney.swift, apps/mobile/MonacoUITests/Journeys/GovernanceWithdrawJourneyUITests.swift]
---

# Withdraw a proposal

The member who proposed a buy takes it back while the vote is open. The other member no longer has it to vote on. The rules are [Proposals](../../architecture/proposals.md).

The old app (`c838bd24`) opened `ProposalDetailView` from the cabal's "Needs your vote" card (`ProposalHistorySection`). It had no withdraw control: "Withdraw proposal", its confirm and the toast are new in [screens.md](../../screens.md) (`ProposalDetailSlot`), and every Expect cell quotes that copy.

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actors A and B have each signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend`, which also builds `bin/monacoctl` |
| P3 | `apps/mobile/qa/journeys/governance/withdraw.setup.sh` ran right before the scenario. It marks A and B as done with onboarding and sets their display names to the `name` column of `accounts.tsv`. It closes every open proposal A or B can vote on. A creates the cabal `QA withdraw {QA.run}`, B joins it, and A proposes a $5.00 buy with the reason `QA withdraw {QA.run}`. It hands the test `cabalName` and `proposalID` |

## Scenarios

### S1 Withdraw an open proposal

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap, type, then tap | the Cabals tab, `cabals-search-field`, then the `cabals-search-result-<id>` | `{cabalName}` | `cabal-header-name` reads `{cabalName}` within 15 s |
| S1.2 | A | tap | `proposal-card-<proposalID>` in the cabal's proposals | | The Proposal screen shows "Votes" and "Why buy" within 15 s |
| S1.3 | A | tap | "Withdraw proposal" | | The confirm "Withdraw this proposal?" with "Votes so far are dropped." shows within 5 s |
| S1.4 | A | tap | "Withdraw" in the confirm | | The toast "Proposal withdrawn." shows within 10 s, and the card shows the status chip "Withdrawn" within 10 s |
| S1.5 | B | tap | the Home tab | | `proposal-card-<proposalID>` is not under "Needs your vote" within 15 s |

## Ground truth

`apps/mobile/qa/journeys/governance/withdraw.truth.sh` reads the hand-off. After S1, the proposal's status is `withdrawn`.

## Known failures on staging

| Step | What fails | Blocked by |
| --- | --- | --- |
| S1.2 | The cabal's proposals show only proposals the viewer can vote on, so the proposer never sees their own card there, and a card does not open `ProposalRoute` | #612 |
| S1.3 | `ProposalDetailSlot` has no "Withdraw proposal" button, so the proposal cannot be withdrawn from the app. `DELETE /v1/proposals/{id}` (flow 13) is built | #612 |
| S1.4 | Follows S1.3 | #612 |

## Not covered

- Accessibility identifiers for "Withdraw proposal" and its confirm. They do not exist yet, so the test finds them by label. They belong to #612 in `apps/mobile/Monaco/Features/Proposals/ProposalDetailSlot.swift`.
- A withdraw refused because a vote already closed the proposal. The flow layer covers it (flow 13, `ProposalClosed`).
