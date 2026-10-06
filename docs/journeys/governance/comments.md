---
id: governance/comments
title: Comment on a proposal
version: 1
milestone: M18
requires: [auth/sign-in]
actors: [A]
flows: []
xcuitest: [apps/mobile/MonacoUITests/Journeys/GovernanceCommentsJourney.swift, apps/mobile/MonacoUITests/Journeys/GovernanceCommentsJourneyUITests.swift]
---

# Comment on a proposal

A member opens a proposal, reads its comments, posts one, and replies to a comment in the thread. Comments live on the proposal's feed item.

Old app (`c838bd24`): Proposal detail, then `Proposals/CommentThreadView.swift`: read the thread, reply, post. Spec: `ProposalCommentsSlot` in [screens.md](../../screens.md#proposal-screen-proposalroute).

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actor A has signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend` |
| P3 | `apps/mobile/qa/journeys/governance/comments.setup.sh` ran right before the scenario. It marks A as done with onboarding, sets A's display name, and creates the open cabal `QA comments {QA.run}` as A. It cannot seed the open proposal the scenarios start from: creating one needs a live Jupiter quote, and no flow or testkit scenario seeds one as the QA accounts. Once #612 or #711 lands a seed path, the setup seeds the proposal and its feed item there |

## Scenarios

### S1 Read the thread and post

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap, tap, then tap | the Feed tab, `feed-chip-Proposals`, then the first `feed-cell-<id>` | | `proposal-detail` shows within 15 s (old app: Proposal detail; screens.md `ProposalDetailSlot`) |
| S1.2 | A | wait | `comment-thread` | | The thread reads "Comments" and `comment-thread-empty` reads "No comments yet" within 10 s (screens.md `ProposalCommentsSlot`: ""Comments", the thread ... Empty: "No comments yet""; old app: `CommentThreadView` read) |
| S1.3 | A | type, then tap | `comment-composer-field`, then `comment-composer-send` | `QA comment {QA.run}` | The field's placeholder was "Add a comment", and a `comment-row-<id>` reads `QA comment {QA.run}` within 10 s (screens.md: "a composer pinned at the bottom, "Add a comment""; old app: `CommentThreadView` post) |

### S2 Reply to a comment

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | tap, tap, then tap | the Feed tab, `feed-chip-Proposals`, then the first `feed-cell-<id>` | | `proposal-detail` shows within 15 s |
| S2.2 | A | type, then tap | `comment-composer-field`, then `comment-composer-send` | `QA ask {QA.run}` | A `comment-row-<id>` reads `QA ask {QA.run}` within 10 s |
| S2.3 | A | tap | `comment-reply-<id>` on the row reading `QA ask {QA.run}` | | `comment-composer-cancel-reply` shows within 5 s (old app: `CommentThreadView` reply) |
| S2.4 | A | type, then tap | `comment-composer-field`, then `comment-composer-send` | `QA answer {QA.run}` | A `comment-row-<id>` reads `QA answer {QA.run}` within 10 s, below the comment it answers |

## Ground truth

`apps/mobile/qa/journeys/governance/comments.truth.sh` reads `feed_comments`. `QA comment {QA.run}` and `QA ask {QA.run}` are top-level comments by A, and `QA answer {QA.run}` has `QA ask {QA.run}` as its parent. A run that wrote none of them passes with a note, since every step is a known failure until #711.

## Known failures on staging

- S1.1 and S2.1: no proposal cell to open, because the setup cannot seed a proposal. Blocked by #612.
- S1.2, S1.3, S2.2, S2.3 and S2.4 read the live `ProposalCommentsSlot` (#711) and pass once S1.1 and S2.1 can open a proposal.

## Not covered

- Comments on other feed items (trades, price moves). #711 moves comments onto every feed item; this journey covers the proposal, the old app's only thread.
- Deleting a comment and the 1000-character cap (`comment-composer-too-long`).
- A second member reading the comment live. That needs realtime (#623).
