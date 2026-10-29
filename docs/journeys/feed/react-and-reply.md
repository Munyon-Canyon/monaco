---
id: feed/react-and-reply
title: React and reply from the Feed
version: 2
milestone: F launch (phase 1)
requires: [auth/sign-in]
actors: [A]
flows: [21, 21b]
xcuitest: [apps/mobile/MonacoUITests/Journeys/FeedReactAndReplyJourney.swift, apps/mobile/MonacoUITests/Journeys/FeedReactAndReplyJourneyUITests.swift]
---

# React and reply from the Feed

A member reacts to a post and replies to it from the Feed card, reads the post page, replies to a reply with a GIF, long-presses a reply for its actions, and reads a public post of a cabal they are not in. Every Feed post takes emoji reactions and replies (flow 21 is replies, where #6533 adds the GIF attachment rows; flow 21b is reactions, #6431 and #6507). There are no image uploads in chats at launch, so no step uses a Photo.

Frames: `05-feed-A`, `05-post-reply`, `05-post`, `05-post-scrolled`, `05-comment-actions`, `05-post-public` in `docs/design/f/screens/`.

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actor A has signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend` |
| P3 | The app under test is built with the GIPHY SDK key. `GIPHY_SDK_KEY` is encrypted in `.env.local`; `just build mobile` decrypts it with the shared `.env.keys` into the gitignored build config, as it does Privy's client ID (#6556), and the SDK asks GIPHY at rating pg-13. The backend needs no GIF key. A build from the `placeholder` config (CI, no key) draws #6556's unavailable state in the GIF sheet, so S2.3 runs only on a keyed build; the other steps need no key |
| P4 | `apps/mobile/qa/journeys/feed/react-and-reply.setup.sh` ran right before S1 (it seeds roles through `--pool`, so no run mints a user). The pool host `feed-host` makes the cabal `QA feed {QA.run}` with A as a member, a member's sale Feed post by the host in it, and one reply `QA root {QA.run}` by the host on that post. It also makes the cabal `QA public {QA.run}`, which A is not in, with an open proposal by the host and one reply `QA public reply {QA.run}` by the host. Rows with no API route (the sale's Feed item, the replies, the proposal) are written with SQL, the way `governance/vote` writes its proposal. It passes the two Feed item ids to the test through the hand-off file |

## Scenarios

### S1 React and reply from a Feed card

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap | the Feed tab | | The sale's card `feed-cell-<item id>` shows within 15 s with its reaction row's "+" (`reaction-add-feed-<item id>`, #6469) and the reply control `feed-reply-<item id>` reading 1 (#6551) |
| S1.2 | A | tap, search, tap, tap, tap | `reaction-add-feed-<item id>`, `emoji-search`, `emoji-cell-1F525` in `emoji-picker`, the new chip, then "+" and the picker again | `fire` | Within 10 s the card shows `reaction-chip-feed-<item id>-1F525` "🔥 1" outlined as A's (#6494, after #6469's split). Tapping the chip removes it. Adding it again brings it back |
| S1.3 | A | tap, type, tap | `feed-reply-<item id>`, `comment-composer-field`, `comment-composer-send` | `QA feed reply {QA.run}` | Over the Feed `reply-sheet` reads "Reply to QA host", `reply-sheet-quote` shows the host's name and the post's summary, and the field has focus with `comment-composer-gif` inside it (`05-post-reply`). Within 10 s the toast reads "Replied", the sheet closes and the reply control reads 2 |

### S2 The post page

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | tap | the card | | `feed-item-detail` shows within 15 s with the inline title "Post", the post with its reaction row, "Replies 2", and the rows `comment-row-<id>` of `QA root {QA.run}` and A's reply (`05-post`) |
| S2.2 | A | scroll to | `comment-composer-field` | | The last reply sits above the docked composer, whose field reads "Say something" with `comment-composer-gif` (`05-post-scrolled`) |
| S2.3 | A | tap, tap, type, tap | `comment-reply-<root id>` on `QA root {QA.run}`, `comment-composer-gif`, `gif-search`, the first `gif-cell-<id>` | `rocket` | `reply-sheet` shows `reply-sheet-quote` with `QA root {QA.run}`. `gif-sheet` shows `gif-attribution` and the grid (#6556). Within 15 s the GIF is sent (it sends on pick), the sheets close, and the new row shows `gif-bubble-<comment id>` (#6558) with its quote `comment-quote-<comment id>` naming the host |
| S2.4 | A | long-press, tap | `QA root {QA.run}`, then `reaction-tray-1F389` | | `comment-actions-<id>` shows the quick six and "+" above it, and `comment-action-reply`, `comment-action-copy` and `comment-action-report` below, with no `comment-action-delete` (`05-comment-actions`). Within 10 s the row shows `reaction-chip-comment-<id>-1F389` "🎉 1" as a per-emoji chip (#6541) |
| S2.5 | A | long-press, tap | `QA root {QA.run}`, then `comment-action-copy` | | The toast reads "Copied" |
| S2.6 | A | long-press, tap, tap | A's own `QA feed reply {QA.run}`, `comment-action-delete`, then Delete in the confirmation | | The menu shows `comment-action-delete` and no `comment-action-report`. Within 10 s the row is gone and the header reads "Replies 2" (the GIF reply and the root remain) |

### S3 A public post

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S3.1 | A | pick, tap | "Everyone" in `feed-filter` (#6455), then the open proposal's card of `QA public {QA.run}` | | The read-only card `proposal-vote-readonly` shows within 15 s and its label contains "<cabal> members vote", and no `proposal-vote-yes`, no `proposal-vote-no` and no `proposal-remind` (#6506's non-member state; `05-post-public`) |
| S3.2 | A | scroll to | `comment-members-only` | | It reads "Members of QA public {QA.run} reply here" (#6553). There is no `comment-composer-field` and no `comment-reply-<id>` |
| S3.3 | A | long-press, tap | the host's `QA public reply {QA.run}`, then `reaction-tray-1F44D` | | The tray, `comment-action-copy` and `comment-action-report` show, and no `comment-action-reply`. Within 10 s the row shows `reaction-chip-comment-<id>-1F44D` "👍 1" |

## Ground truth

`apps/mobile/qa/journeys/feed/react-and-reply.truth.sh` reads `feed_reactions`, `feed_comments`, `comment_attachments` and `comment_reactions`. It expects A's 🔥 reaction on the sale item; A's text reply `QA feed reply {QA.run}` deleted and A's GIF reply live with `parent_comment_id` equal to the root and a `gif` attachment; A's 🎉 on the root and 👍 on the public reply; and no reply by A on the public proposal. A run that stopped before it seeded anything passes with a note.

## Known failures on staging

None known.

## Not covered

- Polls, quoted theses and shared stocks from the ⊕ menu (#6563, #6574, #6568; no journey at launch).
- Who reacted (founder decision Q27, listed in the tracking issue #6410).
- Opening a reply from a push (#6582; needs springboard).
