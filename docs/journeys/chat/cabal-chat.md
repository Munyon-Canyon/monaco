---
id: chat/cabal-chat
title: Cabal chat
version: 2
milestone: M19
requires: [auth/sign-in]
actors: [A, B]
flows: []
xcuitest: [apps/mobile/MonacoUITests/Journeys/ChatCabalChatJourney.swift, apps/mobile/MonacoUITests/Journeys/ChatCabalChatJourneyUITests.swift]
---

# Cabal chat

Members of a cabal talk in its chat. A opens the chat from the cabal screen, sees the empty state, and sends a message. B, another member, opens the same chat and sees it arrive.

Old app (`c838bd24`): Cabal, then Chat, `Groups/GroupChatView.swift`, send; the other member waits for the message. Spec: [Chat (`ChatRoute`)](../../screens.md#chat-chatroute-676).

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actors A and B have signed in once (`auth/sign-in`), and their `privy_user_id` values are in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend` |
| P3 | `apps/mobile/qa/journeys/chat/cabal-chat.setup.sh` ran right before the scenario. It marks A and B as done with onboarding, sets their display names, has A create the open cabal `QA chat {QA.run}` and B join it through the API |

## Scenarios

### S1 A sends, B receives

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap, type, tap, then tap | the Cabals tab, `cabals-search-field`, the `cabals-search-result-<id>`, then `cabal-action-chat` | `QA chat {QA.run}` | `chat-title` reads `QA chat {QA.run}` within 15 s (screens.md Chat: "Title is the cabal tile and name"; old app: Cabal -> Chat) |
| S1.2 | A | wait | `chat-empty` | | Reads "No messages yet. Say hi to your cabal or float a stock idea before someone proposes a buy." within 10 s (screens.md Chat: "Empty: the tile, the name, "No messages yet. ..."") |
| S1.3 | A | type, then tap | `chat-composer`, then `chat-send` | `QA hi {QA.run}` | The composer's placeholder was "Message your cabal", and a message reads `QA hi {QA.run}` within 10 s (screens.md Chat: "Composer "Message your cabal" with a send disc"; "mine on the right in ink"; old app: `GroupChatView` send) |
| S1.3b | A | type, tap, type, tap, type `@`, tap, then tap | `chat-composer`, `chat-send`, the first `chat-mention-<handle>` in `chat-mention-picker`, then `chat-send` | `QA two {QA.run}`, `QA three {QA.run}`, then `QA mention {QA.run} @` | Each of the three messages reads in the thread within 10 s, `QA hi {QA.run}` is still there, and the app neither crashes nor hangs (#3467) |
| S1.4 | B | tap, type, tap, then tap | the Cabals tab, `cabals-search-field`, the `cabals-search-result-<id>`, then `cabal-action-chat` | `QA chat {QA.run}` | A message reads `QA hi {QA.run}` within 15 s, with A's name over it (screens.md Chat: "Others' messages on the left with the author's avatar ... and name over the first of a run"; old app: B waits for the message) |

## Ground truth

`apps/mobile/qa/journeys/chat/cabal-chat.truth.sh` reads `cabal_members`: opening and using the chat never changes membership, so `QA chat {QA.run}` has exactly A and B. There is no chat message table on staging yet (#676), so the script cannot read back `QA hi {QA.run}`; once #676 adds one, the script checks one message by A.

## Known failures on staging

- S1.2: the empty chat shows the tile and the name but not "No messages yet. Say hi to your cabal ...". Blocked by #676.
- S1.3: the composer is disabled under "Chat opens soon.", and no route reads or sends a message. Blocked by #676 and #623.
- S1.4: B cannot see a message that was never sent, and no realtime channel delivers one. Blocked by #623 and #676.

## Not covered

- Chat messages have no identifier in `apps/mobile/Monaco/Features/Groups/CabalChatView.swift` (the legacy `GroupChatView` had `group-chat-message-<id>`). The steps find a message by its text; #676 should add `chat-message-<id>`, and this doc bumps to version 2 when it does. This ticket does not touch app code.
- A failed send ("Not sent · Retry"), day dividers, and the closed notice a removed member sees (`chat-closed`).
- Threads and delete (#703), "Seen by N" and unread badges (#704), @mentions (#2147).
