---
id: chat/cabal-chat
title: Cabal chat
version: 8
milestone: M19
requires: [auth/sign-in]
actors: [A, B]
flows: []
xcuitest: [apps/mobile/MonacoUITests/Journeys/ChatCabalChatJourney.swift, apps/mobile/MonacoUITests/Journeys/ChatCabalChatJourneyUITests.swift]
---

# Cabal chat

Members of a cabal talk in its chat, the tab a cabal opens on. A opens the cabal, sees the quiet chat, and sends a message. B, another member, opens the same chat and sees it arrive, reacts to it, and sees A pin it. A vote for a buy lands in the thread, and B casts it there.

Old app (`c838bd24`): Cabal, then Chat, `Groups/GroupChatView.swift`, send; the other member waits for the message. Spec: [Chat (`ChatRoute`)](../../screens.md#chat-chatroute-676), and the v5 frames `docs/design/v5/screens/03-cabal-chat.png` and `03-cabal-chat-few.png`.

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actors A and B have signed in once (`auth/sign-in`), and their `privy_user_id` values are in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend` |
| P3 | `apps/mobile/qa/journeys/chat/cabal-chat.setup.sh` ran right before the scenario. It marks A and B as done with onboarding, sets their display names, has A create the cabal `QA chat {QA.run}` and B join it through the API. Again, right before S1.7, it seeds an open buy proposal by A in that cabal, which the backend writes into the chat as a vote card: a proposal needs cash in the pot, and this journey moves no money |

## Scenarios

### S1 A sends, B receives

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | run | `openCabal(named:tab:)` (the Cabals tab, then the cabal's tile) | `QA chat {QA.run}`, tab `chat` | `cabal-inline-title` reads `QA chat {QA.run}` and `chat-view` shows within 15 s: a cabal opens on Chat, with no "…" menu in the way (#7918; `03-cabal-chat`) |
| S1.2 | A | wait | `chat-quiet-title`, `chat-quiet-line`, `chat-quiet-propose`, `chat-quiet-poll`, `chat-quiet-stock` | | `chat-quiet-title` reads "No vote open." and `chat-quiet-line` reads "2 members" and "$0.00 pot" (two members, no money) within 10 s, and the three things to do show: "Propose a trade", "Poll" and "Share a stock" (the quiet chat of a brand-new cabal; `03-cabal-chat-few`) |
| S1.3 | A | type, then tap | `chat-composer`, then `chat-send` | `QA hi {QA.run}` | The composer's placeholder began "Message" and named the cabal, then a `chat-message-<id>` reads `QA hi {QA.run}` within 10 s, and the placeholder reads "Message" alone (screens.md Chat: “mine on the right in ink”; old app: `GroupChatView` send) |
| S1.3b | A | type, tap, type, tap, type `@`, tap, then tap | `chat-composer`, `chat-send`, the first `chat-mention-<handle>` in `chat-mention-picker`, then `chat-send` | `QA two {QA.run}`, `QA three {QA.run}`, then `QA mention {QA.run} @` | Each of the three messages reads in the thread within 10 s, `QA hi {QA.run}` is still there, and the app neither crashes nor hangs (#3467) |
| S1.4 | B | run | `openCabal(named:tab:)` (the Cabals tab, then the cabal's tile) | `QA chat {QA.run}`, tab `chat` | A `chat-message-<id>` reads `QA hi {QA.run}` within 15 s, and its label starts with A's name, which is also drawn over the first bubble of a run (`chat-author-<id>` is hidden from VoiceOver, so the label carries it; screens.md Chat: "Others' messages on the left with the author's avatar ... and name over the first of a run"; old app: B waits for the message) |
| S1.5 | B | long-press, then tap | the `chat-message-<id>` of `QA hi {QA.run}`, then `reaction-tray-1F44D` in `reaction-tray-chat-<id>` | | The quick tray shows within 5 s, and after the tap `chat-tapback-<id>` reads "1 reaction" within 10 s (#7906; `03-cabal-chat`) |
| S1.5b | A | run, then wait | `openCabal(named:tab:)`, then `chat-tapback-<id>` | `QA chat {QA.run}`, tab `chat` | A's message shows `chat-tapback-<id>` reading "1 reaction" with 👍 within 10 s |
| S1.6 | A | long-press, then tap | the `chat-message-<id>` of `QA hi {QA.run}`, then `chat-action-pin` | | The menu offers "Pin" because A is the admin, and after the tap `chat-pin-capsule` shows within 10 s (#7908; `03-cabal-chat`) |
| S1.6b | B | run, then wait | `openCabal(named:tab:)`, then `chat-pin-capsule` and `chat-pin-next` | `QA chat {QA.run}`, tab `chat` | `chat-pin-next` reads A's first name and `QA hi {QA.run}` ("Pinned by …") within 10 s |
| S1.7 | B | wait, hold | the `chat-proposal-<id>` card, then `chat-proposal-yes-<id>` | | The seeded buy shows as a vote card in the thread within 15 s, with `chat-proposal-yes-<id>` and `chat-proposal-no-<id>`. After a pick and a hold, `chat-proposal-yes-<id>` reads "Voted Yes" within 10 s (#7915; `03-cabal-chat`) |

## Ground truth

`apps/mobile/qa/journeys/chat/cabal-chat.truth.sh` reads `cabal_members`: opening and using the chat never changes membership, so `QA chat {QA.run}` has exactly A and B. It also reads `votes`: when the seeded proposal exists, B's Yes from S1.7 is the only ballot on it, or there is none when the run stopped before S1.7. The script does not read the messages back: `QA hi {QA.run}` is checked on screen by S1.4.

## Known failures on staging

None.

## Not covered

- A failed send ("Not sent" with "Retry"), day dividers, the offline notice (`chat-offline-notice`), and the closed notice a removed member sees (`chat-closed`).
- Proposing a buy from `chat-quiet-propose`: a proposal needs cash in the pot, and `governance/propose-buy` funds one with real USDC, so S1.7 seeds the proposal and covers the vote. Threads and delete (`chat/talk-it-over`), "Seen by N" and unread badges.
