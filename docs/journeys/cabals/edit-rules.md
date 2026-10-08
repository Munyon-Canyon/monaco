---
id: cabals/edit-rules
title: Edit a cabal's rules and name
version: 4
milestone: M10
requires: [auth/sign-in]
actors: [A]
flows: []
xcuitest: [apps/mobile/MonacoUITests/Journeys/CabalsEditRulesJourney.swift, apps/mobile/MonacoUITests/Journeys/CabalsEditRulesJourneyUITests.swift]
---

# Edit a cabal's rules and name

The creator opens a cabal's details, opens Cabal settings from a Rules row, renames the cabal, changes what passes and how long votes stay open, and saves. In the same editor the creator picks who votes, and one Save sends it all. The details sheet reads the new rules back. The rules are [Cabals](../../architecture/cabals.md#rules).

Old app (`c838bd24`): Cabal -> i -> Rules -> the editor -> Save; the name and picture in the same editor (`GroupDetailsSheet.swift`, `CabalPictureEditor.swift`). Spec: `CabalRulesSlot` in [screens.md](../../screens.md#cabal-screen-cabalroute).

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actors A and B have each signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend` |
| P3 | `apps/mobile/qa/journeys/cabals/edit-rules.setup.sh` ran right before the scenario. It marks A and B as done with onboarding and sets their display names. A creates the cabal `QA rules {QA.run}` with "Every member" voting, "Majority" to pass and "1 day" open, and B joins it through the API. The setup writes B's user id to the hand-off file as `member-id` |

## Scenarios

### S1 Rename, change the rules, pick the voters

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap, type, then tap | the Cabals tab, `cabals-search-field`, then the `cabals-search-result-<id>` | `QA rules {QA.run}` | `cabal-header-name` reads `QA rules {QA.run}` within 15 s |
| S1.2 | A | tap, then scroll to | `cabal-details-button`, then `cabal-rules` | | `cabal-rules-name` reads "Name" `QA rules {QA.run}`, `cabal-rules-voters` "Who votes" "Every member", `cabal-rules-threshold` "To pass" "Majority" and `cabal-rules-expiry` "Votes stay open" "1 day" within 10 s |
| S1.3 | A | tap | `cabal-rules-threshold` | | The "Cabal settings" screen shows `edit-cabal-name` holding `QA rules {QA.run}` and `edit-cabal-rules-footer` reading "Rule changes apply to new proposals. Open votes keep their rules." within 10 s |
| S1.4 | A | clear, then type | `edit-cabal-name` | `QA renamed {QA.run}` | `edit-cabal-save` "Save" is enabled |
| S1.5 | A | tap, then tap | "Everyone agrees" in `edit-rule-threshold`, then "1 week" in `edit-rule-expiry` | | `edit-rule-threshold` reads "Passes only if every voter says yes." and `edit-rule-expiry` reads "A vote that hasn't passed closes after 1 week." |
| S1.6 | A | tap | "People I pick" in `edit-rule-voters` | | `edit-rule-voters` first showed "Everyone" selected, and now `edit-cabal-voters` lists the members within 10 s |
| S1.7 | A | tap | `voters-member-<member-id>` | | The row names B and is selected |
| S1.8 | A | tap | `edit-cabal-save` | | The toast "Cabal updated." shows within 10 s. One Save sends the name, threshold, expiry and voters in one PATCH |
| S1.9 | A | tap back | the navigation bar's back button | | The details sheet shows `cabal-rules-threshold` "Everyone agrees", `cabal-rules-expiry` "1 week" and `cabal-rules-voters` naming A and B within 10 s |
| S1.10 | A | tap | `cabal-details-done` | | `cabal-header-name` reads `QA renamed {QA.run}` within 10 s |

## Ground truth

`apps/mobile/qa/journeys/cabals/edit-rules.truth.sh` reads the cabal A created in this run. Its name is `QA renamed {QA.run}`, its threshold is `unanimous`, its proposal expiry is 604800 seconds, its voter mode is `list`, and A and B can both vote.

## Known failures on staging

None known.

## Not covered

- Changing the picture. `cabal-picture-picker` opens the system photo picker, which the journey does not drive (`apps/mobile/Monaco/Features/Groups/CabalPicturePicker.swift`). `CabalPictureEditorTests` covers the upload on the host.
- Joining. Every cabal is request-to-join, so there is no join rule to change, and `cabals/approve-request` covers requests.
- A non-creator's rules rows, which are read-only. `cabals/join` opens a cabal as a member but does not open its details.
