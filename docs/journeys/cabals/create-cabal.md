---
id: cabals/create-cabal
title: Create a cabal
version: 1
milestone: M10
requires: [auth/sign-in]
actors: [A]
flows: [02]
xcuitest: [apps/mobile/MonacoUITests/Journeys/CreateCabalJourney.swift, apps/mobile/MonacoUITests/Journeys/CreateCabalJourneyUITests.swift]
---

# Create a cabal

A signed-in member opens the Cabals tab, starts a cabal with a name and four rules, and lands on the new cabal's screen. The rules are [Cabals](../../architecture/cabals.md#rules). The copy is [Start a cabal](../../screens.md#cabals-tab) in `screens.md`.

The old app (`c838bd24`) took the same path: the Cabals tab "+" (`Groups/CabalsTabView.swift`) opened the New cabal sheet, "Start a cabal" pushed `Groups/CreateGroupView.swift` with the name and the rules, and Create pushed `GroupDetailView` with the toast. On staging the sheet is `NewCabalSheet` and the form is `CreateCabalRoute`, which still draws `CreateGroupView`. Each Expect cell ends with the old-app tap it matches.

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actor A is signed in (`auth/sign-in`) and meets its P4, so the first-run gate opens the tab bar |
| P2 | The local backend is running (`just migrate db`, then `just run backend`) and answers `GET http://127.0.0.1:8080/healthz` |

## Scenarios

### S1 The form shows the four rules

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S1.1 | tap | the Cabals tab | | The toolbar "+" (`cabals-new-button`) shows within 15 s. Old app: the Cabals tab "+" in `CabalsTabView` |
| S1.2 | tap | `cabals-new-button` | | The New cabal sheet shows `new-cabal-create-row` ("Start a cabal" / "Name it and set the rules") and `new-cabal-join-row` ("Join with an invite code" / "Paste the code a friend sent you") within 10 s. Old app: the same two rows in `CabalsTabView`'s "New cabal" sheet |
| S1.3 | tap | `new-cabal-create-row` | | "Start a cabal" shows `create-group-name` within 10 s, with "The rules": `create-rule-join` ("Who can join"), `create-rule-voters` ("Who votes"), `create-rule-threshold` ("To pass") and `create-rule-expiry` ("Votes stay open"). Old app: `CreateGroupView` |

### S2 Create a cabal

Starts on the form (S1).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S2.1 | type | `create-group-name` | `QA pot <run>`, where `<run>` makes the name unique to the run | `create-group-submit` ("Create cabal") is enabled. Old app: the "Cabal name" field in `CreateGroupView` |
| S2.2 | tap | "I approve" in `create-rule-join`, "Just me" in `create-rule-voters`, "Everyone agrees" in `create-rule-threshold`, "1 hour" in `create-rule-expiry` | | Each choice is selected. Old app: the four segmented rules in `CreateGroupView` |
| S2.3 | tap twice | `create-group-submit` | | The cabal screen shows within 20 s: `cabal-header-name` is the typed name, and the form is gone. The second tap proves "Creating…" holds off a duplicate. Old app: Create pushed `GroupDetailView` |
| S2.4 | wait | the toast | | "Cabal created." shows within 5 s of S2.3. Old app: the same toast over `GroupDetailView` |
| S2.5 | wait | `cabal-member-count` | | Reads "1 member", the `CabalHeaderSlot` count. Old app: the member count on `GroupDetailView`'s header |
| S2.6 | tap | Back | | The Cabals tab shows `cabals-list` ("Your cabals") within 10 s, exactly one card in it is named the typed name, and the dashed `cabals-list-new` ("+ New cabal") card is there. Old app: Back to `CabalsTabView`'s cabals strip |

### S3 A blank name cannot be submitted

Starts on the form (S1).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S3.1 | wait | `create-group-submit` | | "Create cabal" is disabled while the name is empty. Old app: `CreateGroupView` disabled Create on an empty name |
| S3.2 | type | `create-group-name` | three spaces | `create-group-submit` stays disabled. Old app: `CreateGroupView` trimmed the name the same way |

## Ground truth

After S2, `apps/mobile/qa/journeys/cabals/create-cabal.truth.sh` reads `GET /v1/me/cabals` and `GET /v1/cabals/{id}` with a dev token for actor A. The newest cabal named `QA pot <run>` appears once, with `join_mode: request`, `voter_mode: list`, `threshold: unanimous`, `proposal_expiry_seconds: 3600`, and one member, actor A, with `can_vote: true`.

## Known failures on staging

None known. Every step's route, `POST /v1/cabals`, is live.

## Not covered

- A server refusal (`invalid_input`, a rate limit). Those are flow 02's outcomes, checked by its flow tests.
- A lost response retried with the same `Idempotency-Key`. MonacoTests' `CreateCabalActionTests` covers it.
- The one-line rule descriptions that change with each choice. No step reads them, because they have no accessibility identifier in `CreateGroupView`.
- The cabal picture, invite code and treasury, which other journeys own.
