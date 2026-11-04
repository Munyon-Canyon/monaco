---
id: feed/react-and-reply
title: React and reply from the Feed
version: 4
milestone: F launch (phase 1)
requires: [auth/sign-in]
actors: [A]
flows: [20a, 21, 21b]
xcuitest: [apps/mobile/MonacoUITests/Journeys/FeedReactAndReplyJourney.swift, apps/mobile/MonacoUITests/Journeys/FeedReactAndReplyJourneyUITests.swift]
---

# React and reply from the Feed

A member reacts to a post and replies to it from the Feed card, votes on an open proposal from its Feed card, reads the post page, replies to a reply with a GIF, long-presses a reply for its actions, and reads a public post of a cabal they are not in, and reports a Feed post (App Store Guideline 1.2). Every Feed post takes emoji reactions and replies (flow 21 is replies, where #6533 adds the GIF attachment rows; flow 21b is reactions, #6431 and #6507). There are no image uploads in chats at launch, so no step uses a Photo.

Frames: `docs/design/v5/screens/05-feed-A.png`, `docs/design/v5/screens/05-post.png`, `docs/design/v5/screens/05-emoji-picker.png` and `docs/design/v5/screens/04-reply-composer.png`, plus `05-post-reply`, `05-post-scrolled`, `05-comment-actions` and `05-post-public` in `docs/design/f/screens/`, which v5 does not redraw.

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actor A has signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend` |
| P3 | The app under test is built with the GIPHY SDK key. `GIPHY_SDK_KEY` is encrypted in `.env.local`; `just build mobile` decrypts it with the shared `.env.keys` into the gitignored build config, as it does Privy's client ID (#6556), and the SDK asks GIPHY at rating pg-13. The backend needs no GIF key. A build from the `placeholder` config (CI, no key) draws #6556's unavailable state in the GIF sheet, so S2.3 runs only on a keyed build; the other steps need no key |
| P4 | `apps/mobile/qa/journeys/feed/react-and-reply.setup.sh` ran right before S1 (it seeds roles through `--pool`, so no run mints a user). The pool host `feed-host` makes the cabal `QA feed {QA.run}` with A as a member, a member's sale Feed post by the host in it, and one reply `QA root {QA.run}` by the host on that post. S5 adds one open buy proposal by the host in `QA feed {QA.run}`, whose voters are the host and A. It also makes the cabal `QA public {QA.run}`, which A is not in, with an open proposal by the host and one reply `QA public reply {QA.run}` by the host. Rows with no API route (the sale's Feed item, the replies, the proposal) are written with SQL, the way `governance/vote` writes its proposal. It passes the Feed item ids, the public proposal's id, and for S5 the vote item and its proposal id to the test through the hand-off file |

## Scenarios

### S1 React and reply from a Feed card

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap | the Feed tab | | The sale's card `feed-cell-<item id>` shows within 15 s with its foot row `reaction-row-feed-<item id>`, no "+" chip and no `reaction-chip-feed-<item id>-*` (nobody has reacted to the seeded sale yet), and the reply control `feed-reply-<item id>` reading 1, not the post page's "Replies" header (#6551) |
| S1.2 | A | long-press, tap, search, tap, tap, then the same again | `reaction-row-feed-<item id>` (hold 1 s; the tray opens at 0.4 s), `reaction-tray-more-feed-<item id>`, `emoji-search` in `emoji-picker`, `emoji-cell-1F525`, then the new chip | `fire` | The long press shows the quick tray (`reaction-tray-1F44D` and `reaction-tray-more-feed-<item id>`) within 5 s. The picker is the half sheet `emoji-picker`. Within 10 s the card shows `reaction-chip-feed-<item id>-1F525` reading 🔥 and 1 marked as A's. Tapping the chip removes it and the foot row shows no chip again. Adding it again the same way brings it back |
| S1.3 | A | tap, type, tap | `feed-reply-<item id>`, `comment-composer-field`, `comment-composer-send` | `QA feed reply {QA.run}` | Over the Feed `reply-sheet` reads "Reply to QA host", `reply-sheet-quote` shows the host's name and the post's summary, and the field has focus with `comment-composer-gif` inside it (`05-post-reply`). Within 10 s the toast reads "Replied", the sheet closes and the reply control reads 2 |

### S2 The post page

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | tap | the card | | `feed-item-detail` shows within 15 s with the inline title "Post", the post's foot row `reaction-row-feed-<item id>` with `reaction-chip-feed-<item id>-1F525` reading 🔥 and 1 (from S1.2), no "+" chip, "Replies 2", and the rows `comment-row-<id>` of `QA root {QA.run}` and A's reply (`05-post`) |
| S2.2 | A | scroll to | `comment-composer-field` | | The last reply sits above the docked composer, whose field reads "Say something" with `comment-composer-gif` (`05-post-scrolled`) |
| S2.3 | A | tap, tap, type, tap | `comment-reply-<root id>` on `QA root {QA.run}`, `comment-composer-gif`, `gif-search`, the first `gif-cell-<id>` | `rocket` | `reply-sheet` shows `reply-sheet-quote` with `QA root {QA.run}`. `gif-sheet` shows `gif-attribution` and the grid (#6556). Within 15 s the GIF is sent (it sends on pick), the sheets close, and the new row shows `gif-bubble-<comment id>` (#6558) with its quote `comment-quote-<comment id>` naming the host |
| S2.4 | A | long-press, tap | `QA root {QA.run}`, then `reaction-tray-1F389` | | `comment-actions-<id>` shows the quick six and "+" above it, and `comment-action-reply`, `comment-action-copy` and `comment-action-report` below, with no `comment-action-delete` (`05-comment-actions`). Within 10 s the row shows `reaction-chip-comment-<id>-1F389` "🎉 1" as a per-emoji chip (#6541) |
| S2.5 | A | long-press, tap | `QA root {QA.run}`, then `comment-action-copy` | | The toast reads "Copied" |
| S2.6 | A | long-press, tap, tap | A's own `QA feed reply {QA.run}`, `comment-action-delete`, then Delete in the confirmation | | The menu shows `comment-action-delete` and no `comment-action-report`. Within 10 s the row is gone and the header reads "Replies 2" (the GIF reply and the root remain) |

### S3 A public post

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S3.1 | A | pick, tap | "Everyone" in `feed-filter` (#6455), then the open proposal's card of `QA public {QA.run}` | | Before the tap, its Feed card shows no `feed-vote-yes-<proposal id>` and no `feed-vote-no-<proposal id>` (A is not in the cabal, so the Feed draws the read-only row). After it, the read-only card `proposal-vote-readonly` shows within 15 s and its label contains "<cabal> members vote", and no `proposal-vote-yes`, no `proposal-vote-no` and no `proposal-remind` (#6506's non-member state; `05-post-public`) |
| S3.2 | A | scroll to | `comment-members-only` | | It reads "Members of {cabal} reply here", naming the cabal `QA public {QA.run}` (#6553). There is no `comment-composer-field` and no `comment-reply-<id>` |
| S3.3 | A | long-press, tap | the host's `QA public reply {QA.run}`, then `reaction-tray-1F44D` | | The tray, `comment-action-copy` and `comment-action-report` show, and no `comment-action-reply`. Within 10 s the row shows `reaction-chip-comment-<id>-1F44D` "👍 1" |

### S4 Report a Feed post

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S4.1 | A | tap, long-press | the Feed tab, then the sale's card `feed-cell-<item id>` | | Within 10 s the card's menu shows `feed-report` and `feed-block` besides its mute rows. A is not offered either on a card of their own |
| S4.2 | A | tap, tap | `feed-report`, then `report-reason-spam` in `report-title` "Report" | | Within 10 s the toast reads "Thanks. We'll take a look." and the sheet closes. The menu's `feed-block` is not tapped, so the pooled host stays unblocked for the next run |

### S5 Vote from a Feed post

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S5.1 | A | tap, then scroll to | the Feed tab, then `feed-cell-<vote item id>` | | Within 15 s the card shows the shared vote card: `feed-vote-headline-<proposal id>` reads "Buy" with the symbol "TSLA" and "$5", `feed-vote-subhead-<proposal id>` names the cabal `QA feed {QA.run}`, reads "0 of 2 voted" and ends in a bare time, with no "replies" and no "closes in", and `feed-vote-yes-<proposal id>` and `feed-vote-no-<proposal id>` show (`05-feed-A`) |
| S5.2 | A | pick and hold | `feed-vote-yes-<proposal id>` (a tap, then a 0.9 s press) | | Within 10 s the toast reads "Voted Yes. 1st of 2." with `monaco-toast-action` (Undo), and `feed-vote-subhead-<proposal id>` reads "1 of 2 voted". After the toast, the push pre-prompt is answered "Not now" if it shows |
| S5.3 | A | pull down, then tap | `feed-root`, then `feed-vote-open-<proposal id>` | | Within 15 s of the refresh `feed-vote-subhead-<proposal id>` still reads "1 of 2 voted" (the voted state comes back from the server). The tap opens the proposal page, which shows `proposal-change-vote` within 15 s: A's ballot is on the server, and the tap on the card body opened the proposal instead of voting |

## Ground truth

`apps/mobile/qa/journeys/feed/react-and-reply.truth.sh` reads `feed_reactions`, `feed_comments`, `comment_attachments` and `comment_reactions`. It expects A's 🔥 reaction on the sale item; A's text reply `QA feed reply {QA.run}` deleted and A's GIF reply live with `parent_comment_id` equal to the root and a `gif` attachment; A's 🎉 on the root and 👍 on the public reply; and no reply by A on the public proposal; one `votes` row by A with choice `yes` on the S5 proposal (0 when the run stopped before S5, with a note), and at least one `reports` row by A against the host with kind `user` and reason `spam` (the sale card's actor). A run that stopped before it seeded anything passes with a note.

## Known failures on staging

None known.

## Not covered

- Undo from the Feed card: the toast's Undo is `governance/vote`'s, and this journey keeps A's vote for the truth check.
- Polls, quoted theses and shared stocks from the ⊕ menu (#6563, #6574, #6568; no journey at launch).
- Who reacted (founder decision Q27, listed in the tracking issue #6410).
- Opening a reply from a push (#6582; needs springboard).
