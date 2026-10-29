---
id: chat/cabal-chat
title: Cabal chat
version: 6
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
| P3 | `apps/mobile/qa/journeys/chat/cabal-chat.setup.sh` ran right before the scenario. It marks A and B as done with onboarding, sets their display names, has A create the cabal `QA chat {QA.run}` and B join it through the API |

## Scenarios

### S1 A sends, B receives

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap, type, tap, then tap | `openCabal(named:)` (the Cabals tab, then the cabal's tile), `cabal-more-button`, then `cabal-more-chat` | `QA chat {QA.run}` | `chat-title` reads `QA chat {QA.run}` within 15 s (screens.md Chat: "Title is the cabal tile and name"; old app: Cabal -> Chat) |
| S1.2 | A | wait | `chat-empty` | | Reads "Start the conversation" over "Messages here stay with your cabal. Proposal-specific discussion stays attached to the proposal." within 10 s (screens.md Chat: "Empty: a chat tile, "Start the conversation" over …") |
| S1.3 | A | type, then tap | `chat-composer`, then `chat-send` | `QA hi {QA.run}` | The composer's placeholder was "Message your cabal", and a `chat-message-<id>` reads `QA hi {QA.run}` within 10 s (screens.md Chat: "Composer "Message your cabal" with a send disc"; "mine on the right in ink"; old app: `GroupChatView` send) |
| S1.3b | A | type, tap, type, tap, type `@`, tap, then tap | `chat-composer`, `chat-send`, the first `chat-mention-<handle>` in `chat-mention-picker`, then `chat-send` | `QA two {QA.run}`, `QA three {QA.run}`, then `QA mention {QA.run} @` | Each of the three messages reads in the thread within 10 s, `QA hi {QA.run}` is still there, and the app neither crashes nor hangs (#3467) |
| S1.4 | B | tap, type, tap, then tap | `openCabal(named:)` (the Cabals tab, then the cabal's tile), `cabal-more-button`, then `cabal-more-chat` | `QA chat {QA.run}` | A `chat-message-<id>` reads `QA hi {QA.run}` within 15 s, and its label starts with A's name, which is also drawn over the first bubble of a run (`chat-author-<id>` is hidden from VoiceOver, so the label carries it; screens.md Chat: "Others' messages on the left with the author's avatar ... and name over the first of a run"; old app: B waits for the message) |

## Ground truth

`apps/mobile/qa/journeys/chat/cabal-chat.truth.sh` reads `cabal_members`: opening and using the chat never changes membership, so `QA chat {QA.run}` has exactly A and B. The script does not read the messages back: `QA hi {QA.run}` is checked on screen by S1.4.

## Known failures on staging

None. The chat entry is the cabal page's "…" menu (`cabal-more-button`, `cabal-more-chat`), not a tab (founder decision Q53, #6497).

## Not covered

- A failed send ("Not sent" with "Retry"), day dividers, the offline notice (`chat-offline-notice`), and the closed notice a removed member sees (`chat-closed`).
- The proposal card in the timeline, threads and delete (#703), "Seen by N" and unread badges (#704).
