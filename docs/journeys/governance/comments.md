---
id: governance/comments
title: Comment on a proposal
version: 10
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
| P3 | `apps/mobile/qa/journeys/governance/comments.setup.sh` ran right before the scenario. S1 marks A as done with onboarding, sets A's display name, and creates the cabal `QA comments {QA.run}` as A. Each scenario then writes one open buy proposal by A in that cabal with the thesis `QA proposal {QA.run} <scenario>`, so it is the newest on the Feed. The rows are written the way `governance/vote` writes its proposal, because creating one through the API needs a live Jupiter quote. The feed consumer turns the proposal's event into the Feed card the scenarios open |

## Scenarios

### S1 Read the thread and post

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap, then tap | the Feed tab, then the first `feed-cell-<id>` | | The proposal page (`proposal-vote-yes`) or the post page (`feed-item-detail`) shows within 15 s (a proposal's Feed cell opens the proposal, which holds the thread and the composer; screens.md `ProposalDetailSlot`) |
| S1.2 | A | wait | `comment-thread` | | The section header "Replies" (no count) and the empty line "No replies yet" show in the thread within 10 s (F `04-proposal-dark`, #6541; old app: `CommentThreadView` read) |
| S1.3 | A | type, then tap | `comment-composer-field`, then `comment-composer-send` | `QA comment {QA.run}` | The field's placeholder is "Say something", the GIF button `comment-composer-gif` shows inside the field, and a `comment-body-<id>` reads `QA comment {QA.run}` within 10 s (F: the docked composer, "Say something"; old app: `CommentThreadView` post) |

### S2 Reply to a comment

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | tap, then tap | the Feed tab, then the first `feed-cell-<id>` | | `feed-item-detail` shows within 15 s |
| S2.2 | A | type, then tap | `comment-composer-field`, then `comment-composer-send` | `QA ask {QA.run}` | A `comment-body-<id>` reads `QA ask {QA.run}` within 10 s |
| S2.3 | A | tap | the `comment-reply-<id>` with the id of the `comment-body-<id>` reading `QA ask {QA.run}` | | `reply-sheet` shows within 5 s and its `reply-sheet-quote` reads "QA ask {QA.run}" (F `04-reply-composer`, #6548: the reply half sheet; old app: `CommentThreadView` reply) |
| S2.4 | A | type, then tap | `comment-composer-field`, then `comment-composer-send` | `QA answer {QA.run}` | A `comment-body-<id>` reads `QA answer {QA.run}` within 10 s, one indent in, and its row shows `comment-quote-<id>` naming the comment it answers (#6541) |

## Ground truth

`apps/mobile/qa/journeys/governance/comments.truth.sh` reads `feed_comments`. `QA comment {QA.run}` and `QA ask {QA.run}` are top-level comments by A, and `QA answer {QA.run}` has `QA ask {QA.run}` as its parent. A run that stopped before it wrote any of them passes with a note.

## Known failures on staging

None known.

## Not covered

- Comments on other feed items (trades, price moves). Every feed item has a thread; this journey covers the proposal, the old app's only thread.
- Deleting a comment and the 1000-character cap (`comment-composer-too-long`).
- A second member reading the comment live. That needs realtime (#623).
