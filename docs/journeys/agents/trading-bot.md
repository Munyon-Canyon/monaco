---
id: agents/trading-bot
title: A cabal's trading bot
version: 2
milestone: M23
requires: [auth/sign-in]
actors: [A]
flows: []
xcuitest: [apps/mobile/MonacoUITests/Journeys/AgentsTradingBotJourney.swift, apps/mobile/MonacoUITests/Journeys/AgentsTradingBotJourneyUITests.swift]
---

# A cabal's trading bot

A member opens a cabal's trading bot, reveals its key and copies the connect instructions. A voter proposes adding a trading bot from the Propose chooser. The design is [Trading bot](../../screens.md#trading-bot-m23-691), `CabalAgentSlot` and the Propose chooser in [screens.md](../../screens.md). The old app's version at `c838bd24` is `AgentSectionView` -> `AgentDetailView` -> `AgentKeyRevealView`, and the Propose chooser -> `ProposeAddAgentView`.

The format of this doc is in [App journeys](../README.md). This is post-MVP: every step is a known failure until M23 (#691) lands.

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Everything [auth/sign-in](../auth/sign-in.md) needs |
| P2 | Before each scenario, `apps/mobile/qa/journeys/agents/trading-bot.setup.sh` marks A as done with onboarding, sets A's display name, and has A create the cabal `QA bot {QA.run}` through the API, so A is its creator and a voter |
| P3 | For S1, the cabal needs a trading bot with a $200.00 budget. No route or flow creates one yet (#691). Once #691 adds one, the setup seeds it with `qa_flow_seed` and this doc moves to version 2 |

## Scenarios

### S1 Open the trading bot and copy its connect instructions

Starts signed in (auth/sign-in).

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap, type, then tap | the Cabals tab, `cabals-search-field`, then the `cabals-search-result-<id>` | `QA bot {QA.run}` | `cabal-header-name` reads `QA bot {QA.run}` within 15 s (old app: the cabal screen) |
| S1.2 | A | scroll to | `cabal-agent-row` | | The row reads "Trading bot", the bot's name, "$200.00 budget" and "Active" within 10 s (screens.md `CabalAgentSlot`: "one row with the bot's name, "$200.00 budget" and its state ("Active", "Paused")"; old app: `AgentSectionView`) |
| S1.3 | A | tap | `cabal-agent-row` | | Within 15 s the screen is titled "Trading bot" and `agent-detail-budget` reads "Budget", "From the pot" and "$200.00" (screens.md Trading bot; old app: `AgentDetailView`) |
| S1.4 | A | tap | `agent-key-show` | | The "Bot key" card shows the key unmasked and "Paste this key into your bot. Anyone in the cabal can copy it here until the bot is removed." (screens.md Trading bot; old app: `AgentKeyRevealView`) |
| S1.5 | A | tap | `agent-copy-instructions` | | The button reads "Copy connect instructions" and the toast "Connect instructions copied" shows within 10 s (screens.md Trading bot; old app: `AgentKeyRevealView` copy) |

### S2 Propose adding a trading bot

Starts signed in (auth/sign-in).

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | tap, type, then tap | the Cabals tab, `cabals-search-field`, then the `cabals-search-result-<id>` | `QA bot {QA.run}` | `cabal-header-name` reads `QA bot {QA.run}` within 15 s (old app: the cabal screen) |
| S2.2 | A | tap | `cabal-action-propose` | | `propose-chooser` shows within 15 s, titled "Propose" (screens.md: "Propose chooser (`ProposeRoute(cabalID:)`) … Title "Propose""; old app: the Propose chooser) |
| S2.3 | A | tap | `propose-kind-add-agent` | | The row reads "Add a trading bot" and "Give a bot a budget from the pot", and `add-agent-name-field` shows within 10 s (screens.md: "in M23 "Add a trading bot" / "Give a bot a budget from the pot" (#691)"; old app: `ProposeAddAgentView`) |
| S2.4 | A | type, type, then tap | `add-agent-name-field`, `add-agent-allocation-field`, then `add-agent-submit` | `QA bot {QA.run}`, `5` | The proposal is sent and the chooser closes within 15 s (old app: `ProposeAddAgentView` submit) |

## Ground truth

`apps/mobile/qa/journeys/agents/trading-bot.truth.sh` reads `cabal_members` and `proposals` for `QA bot {QA.run}`. A is its only member, and it has at most the one proposal S2 sends: opening the bot, revealing its key and copying never write a proposal.

## Known failures on staging

- S1.2 to S1.5: Trading bot screen. `CabalAgentSlot` is not live and draws nothing, and nothing seeds a bot. Blocked by #691.
- S2.2 to S2.4: Propose add trading bot. `ProposeRoute` shows the not-migrated screen, so `propose-chooser` never opens. Blocked by #691 and #613.

## Not covered

- `cabal-agent-row` does not exist yet. #691 adds it in `apps/mobile/Monaco/Features/Groups/CabalSlots/CabalAgentSlot.swift`.
- `agent-key-show` and `agent-copy-instructions` do not exist yet. #691 adds them on the trading bot screen (`apps/mobile/Monaco/Features/Groups/AgentDetailView.swift` today). This ticket does not touch app code.
- Pausing, resuming and removing a bot (`propose-kind-pause-agent`, `propose-kind-resume-agent`, `propose-kind-revoke-agent`). They need a live bot; a later version of this journey adds them.
- "Spent", "Holdings" and "Trades" on the trading bot screen. They need an agent read route that M23 does not ticket yet.
