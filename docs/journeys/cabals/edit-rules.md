---
id: cabals/edit-rules
title: Edit a cabal's rules and name
version: 8
milestone: M10
requires: [auth/sign-in]
actors: [A, B]
flows: []
xcuitest: [apps/mobile/MonacoUITests/Journeys/CabalsEditRulesJourney.swift, apps/mobile/MonacoUITests/Journeys/CabalsEditRulesJourneyUITests.swift]
---

# Edit a cabal's rules and name

The creator opens the cabal with `openCabal(named:)`, reads the six rows of the Rules tab and the footer, and renames the cabal from the "…" menu. Per founder decision Q4 every rule changes by proposal, so S2 turns "Show who voted what" on by proposal: A sends it, A and B hold Yes, and the Rules tab reads "on since". Only the name, picture and slippage stay direct edits. The rules are [Cabals](../../architecture/cabals.md#rules).

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actors A and B have each signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend` |
| P3 | `apps/mobile/qa/journeys/cabals/edit-rules.setup.sh` ran right before the scenario. It marks A and B as done with onboarding and sets their display names. A creates the cabal `QA rules {QA.run}` with "Every member" voting, "Majority" to pass and "1 day" open, and B joins it through the API. The setup writes B's user id to the hand-off file as `member-id` |

## Scenarios

### S1 Read the rules, rename

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | `openCabal(named:)` | the Cabals tab and the tile | `QA rules {QA.run}` | `cabal-header-name` reads `QA rules {QA.run}` within 15 s |
| S1.2 | A | `openCabalTab("Rules")` | `cabal-rules` | | Within 10 s: `cabal-rules-votesToPass` "2 of 2", `cabal-rules-voteWindow` "24 h", `cabal-rules-maxPerTrade` "No limit", `cabal-rules-whoCanPropose` "Voters", `cabal-rules-cashOut` "Anytime, your share", `cabal-rules-showSides` "After a vote closes · off"; `cabal-rules-footer` starts "Counts are live for every member." and contains "Rules change by proposal;" |
| S1.3 | A | tap, then tap | `cabal-more-button`, then `cabal-more-edit` | | The editor shows `edit-cabal-name` within 10 s |
| S1.4 | A | clear, type, then tap | `edit-cabal-name`, then `edit-cabal-save` | `QA renamed {QA.run}` | The toast "Cabal updated." shows within 10 s and `cabal-header-name` reads `QA renamed {QA.run}` |

### S2 Show who voted what, by proposal

Starts on the renamed cabal.

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | `openCabalTab("Rules")`, then tap | `cabal-rules-show-sides-toggle` | | `cabal-rule-proposal-words` reads "Show who voted what after a vote closes: on" within 10 s |
| S2.2 | A | tap | `cabal-rule-proposal-propose` | | The toast "Proposed. Your cabal votes on it." shows within 10 s |
| S2.3 | A | tap, then `pickAndHold("yes")` | the `cabal-open-proposal-<id>` row | | A holds Yes |
| S2.4 | B | `openCabal(named:)`, tap, then `pickAndHold("yes")` | the open proposal row | `QA renamed {QA.run}` | B holds Yes; two of two voters pass it |
| S2.5 | A | `openCabal(named:)`, `openCabalTab("Rules")` | `cabal-rules-showSides` | | The row reads "on since" within 20 s |

## Ground truth

`apps/mobile/qa/journeys/cabals/edit-rules.truth.sh` reads the cabal A created in this run. Its name is `QA renamed {QA.run}`, `show_sides` is true, and A and B can both vote.

## Known failures on staging

- Votes to pass, Vote window and Max per trade have no control on the Rules tab: the rows are read-only text, and only the Show who voted what switch starts a proposal (`CabalRulesTab.swift`). The `cabal-rule-proposal-threshold`, `-window` and `-max-trade` pickers are in the sheet but nothing on the page opens them. The ticket's "propose the change from the Rules tab" for those three is not drivable until HC (#6523 / #6498) adds a tap target; S2 covers the switch.

## Not covered

- The text in `edit-cabal-name` before and after typing. `MonacoTextField` sets `accessibilityValue` to an empty string, so the ground truth reads the renamed cabal.
- Changing the picture, which opens the system photo picker.
- The "Who can join" rule. `cabals/join` S2.1 changes it.
- A non-creator's Rules tab, where the switch is disabled.
