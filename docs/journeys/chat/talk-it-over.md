---
id: chat/talk-it-over
title: Talk it over
version: 2
milestone: M19
requires: [auth/sign-in]
actors: [A, B]
flows: [22]
xcuitest: [apps/mobile/MonacoUITests/Journeys/TalkItOverJourney.swift, apps/mobile/MonacoUITests/Journeys/TalkItOverJourneyUITests.swift]
---

# Talk it over

Two members of one cabal talk in its chat. A opens an empty chat and sends a message, and B sees it with A's name over it. B replies in a thread, A deletes a message, and B's unread badge shows on the Cabals tab until B opens the chat.

Old app (`c838bd24`): Cabal, then Chat, `Groups/GroupChatView.swift`. Spec: [Chat (`ChatRoute`)](../../screens.md#chat-chatroute-676). Backend flow 22.

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actors A and B have signed in once (`auth/sign-in`), and their `privy_user_id` values are in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend` |
| P3 | `apps/mobile/qa/journeys/chat/talk-it-over.setup.sh` ran right before S1. It marks A and B as done with onboarding, sets their display names, has A create the cabal `QA {QA.run}` with the default rules and B join it through the API. The cabal is new each run, so its chat starts empty |

## Scenarios

One simulator runs every actor, and each switch signs one member out and the next in, so the steps group each actor's work to keep switches few. B reads a message after switching in, and the chat loads it with no pull to refresh. B opening the chat is what puts "Seen by 1" under `gm {QA.run}` for A in S3.3.

### S1 An empty chat

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap, type, tap, then tap | the Cabals tab, `cabals-search-field`, the `cabals-search-result-<id>`, then `cabal-action-chat` | `QA {QA.run}` | `chat-title` reads `QA {QA.run}`. `chat-empty` reads "No messages yet. Say hi to your cabal or float a stock idea before someone proposes a buy." The composer's placeholder is "Message your cabal" and `chat-send` is disabled, all within 15 s (screens.md Chat) |

### S2 Send, and the other member sees it

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | tap, tap, type, then tap | the `cabals-list-card-<id>`, `cabal-action-chat`, `chat-composer`, then `chat-send` | `gm {QA.run}` | A `chat-message-<id>` reading `gm {QA.run}` shows within 5 s, on the right (screens.md Chat: "mine on the right in ink") |
| S2.2 | B | tap, tap, then wait | the `cabals-list-card-<id>`, `cabal-action-chat`, then `chat-message-<id>` | | A message reads `gm {QA.run}` within 15 s, on the left with A's name over it (`chat-author-<id>`), with no pull to refresh |

### S3 Reply in a thread

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S3.1 | B | long-press, then tap | the `chat-message-<id>` of `gm {QA.run}`, then "Reply" | | The menu offers "Reply" and no "Delete". `chat-thread-screen` shows with `chat-thread-parent-body` reading `gm {QA.run}` and `chat-thread-empty` reading "No replies yet. Start the thread." |
| S3.2 | B | type, then tap | `chat-composer` ("Reply in thread"), then `chat-send` | `gm back` | `chat-thread-also-in-channel` is off, and a message reads `gm back` under the parent within 5 s |
| S3.3 | A | tap, tap, then wait | the `cabals-list-card-<id>`, `cabal-action-chat`, then `chat-replies-<id>` and `chat-seen-label` | | `chat-replies-<id>` reads "1 reply · last reply" and `chat-seen-label` reads "Seen by 1" within 15 s, and no message in the channel reads `gm back` |

### S4 Delete your message

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S4.1 | A | tap, tap, type, tap, long-press, tap, then tap | the `cabals-list-card-<id>`, `cabal-action-chat`, `chat-composer`, `chat-send`, the `chat-message-<id>`, "Delete", then "Delete" in the dialog | `typo {QA.run}` | The menu offers "Reply" and "Delete". The dialog reads "Delete this message?" and "It's removed for everyone in the cabal.". After the second tap the toast `monaco-toast-banner` reads "Message deleted" and no message reads `typo {QA.run}` |

### S5 Unread clears after opening

B has not opened the chat since S3, so the two messages A sends here are all B has unread. B also checks here that A's deleted message is gone for B (S4's other half), because opening the chat clears the unread count.

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S5.1 | A | tap, tap, type, then tap, twice | the `cabals-list-card-<id>`, `cabal-action-chat`, `chat-composer`, then `chat-send` | `one {QA.run}`, then `two {QA.run}` | Both messages show within 5 s |
| S5.2 | B | tap, then wait | the Cabals tab, then the `cabals-list-card-<id>` of `QA {QA.run}` | | The card reads "2 unread messages" within 10 s |
| S5.3 | B | tap, then tap | the `cabals-list-card-<id>`, then `cabal-action-chat` | | `cabal-action-chat` reads "Chat, unread messages" (the dot) before the tap. The chat shows `two {QA.run}` as its newest message and `gm {QA.run}`, and no message reads `typo {QA.run}` |
| S5.4 | B | tap back, then tap back | `BackButton`, then `BackButton` | | `cabal-action-chat` reads "Chat" again within 10 s of the first back, and the card has no unread badge on the Cabals tab |

## Ground truth

`apps/mobile/qa/journeys/chat/talk-it-over.truth.sh` reads `cabal_messages` for `QA {QA.run}`. It has one live message `gm {QA.run}` by A with `reply_count` 1 and one reply `gm back` by B that is not in the channel. `typo {QA.run}` has `deleted_at` set. `one {QA.run}` and `two {QA.run}` are live.

## Known failures on staging

None.

## Not covered

- An empty chat as B, which S1.1 already covers for the same screen. "Not sent · Retry", which needs the network cut during a run, the "Seen by" sheet, @mentions (#2147), and the closed composer after leaving.
- "Also send to channel" turned on. #3238 adds the toggle's channel copy, so S3.2 only checks that a reply sent with it off stays out of the channel.
- Live delivery with both members on screen. One simulator holds one signed-in member, so S2.2 and S3.3 check that a message is there after a member opens the chat, which the backend and the chat realtime channel already serve.
