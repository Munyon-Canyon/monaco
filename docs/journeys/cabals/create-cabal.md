---
id: cabals/create-cabal
title: Create a cabal
version: 2
milestone: M10
requires: [auth/sign-in]
actors: [A, B]
flows: [02, 03]
xcuitest: [apps/mobile/MonacoUITests/Journeys/CreateCabalJourney.swift, apps/mobile/MonacoUITests/Journeys/CreateCabalJourneyUITests.swift]
---

# Create a cabal

A signed-in member opens the Cabals tab, starts a cabal with a name and four rules, and lands on the new cabal's screen. A friend then joins a new cabal with its invite code, and the creator sees them on the member board. The rules are [Cabals](../../architecture/cabals.md#rules). The copy is [Start a cabal](../../screens.md#cabals-tab) in `screens.md`.

The old app (`c838bd24`) took the same path: the Cabals tab "+" (`Groups/CabalsTabView.swift`) opened the New cabal sheet, "Start a cabal" pushed `Groups/CreateGroupView.swift` with the name and the rules, and Create pushed `GroupDetailView` with the toast. On staging the sheet is `NewCabalSheet` and the form is `CreateCabalRoute`, which still draws `CreateGroupView`. Each Expect cell ends with the old-app tap it matches.

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actor A is signed in (`auth/sign-in`) and meets its P4, so the first-run gate opens the tab bar |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend`, which also builds `bin/monacoctl` |
| P3 | For S4, actor B has signed in once (`auth/sign-in`), and `apps/mobile/qa/journeys/cabals/create-cabal.setup.sh S4` ran right before the scenario. `scripts/qa/journey.py run` runs it. It marks A and B as done with onboarding and sets their display names to the `name` column of `accounts.tsv` (B is "Bartholomez"). For S1 to S3 it does nothing |

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

### S4 A friend joins with the invite code

A creates an open cabal, B joins it with the code, and A finds B on the member board.

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S4.1 | A | tap | the Cabals tab, `cabals-new-button`, then `new-cabal-create-row` | | "Start a cabal" shows `create-group-name` within 10 s. Old app: Cabals "+", then "Start a cabal" pushed `CreateGroupView` |
| S4.2 | A | type, tap, then tap | `create-group-name`, "Anyone" in `create-rule-join`, then `create-group-submit` | `QA duo {QA.run}` | "Anyone" is selected. `cabal-header-name` reads `QA duo {QA.run}` within 20 s, and the toast "Cabal created." shows within 10 s. Old app: Create in `CreateGroupView` pushed `GroupDetailView` |
| S4.3 | A | tap | `cabal-details-button` | | `cabal-invite-card` ("Invite code") shows within 10 s, and `cabal-invite-code` is 10 characters. The test hands the code to B. Old app: "i" opened `GroupDetailsSheet` with the code |
| S4.4 | B | tap | the Cabals tab, `cabals-new-button`, then `new-cabal-join-row` | | "Join a cabal" shows `join-group-id` within 10 s. Old app: Cabals "+", then "Join with an invite code" pushed `JoinGroupView` |
| S4.5 | B | tap | `join-group-paste` | the code from S4.3, on B's clipboard | `join-group-name` reads `QA duo {QA.run}` and `join-group-submit` reads "Join cabal" within 10 s. Old app: Paste in `JoinGroupView`'s code field |
| S4.6 | B | tap | `join-group-submit` | | The toast "You're in." shows within 10 s, and `cabal-member-count` reads "2 members" within 15 s. Old app: Join in `JoinGroupView` pushed `GroupDetailView` with the same toast |
| S4.7 | A | tap, type, then tap | the Cabals tab, `cabals-search-field`, then the `cabals-search-result-<id>` | `QA duo {QA.run}` | `cabal-member-count` reads "2 members" within 15 s, and a `cabal-member-<id>` row on the "Leaderboard" (`CabalMemberBoardSlot`) names B within 15 s. Old app: `GroupDetailView`'s `MemberBoardSection` |

## Ground truth

After S2, `apps/mobile/qa/journeys/cabals/create-cabal.truth.sh` reads `GET /v1/me/cabals` and `GET /v1/cabals/{id}` with a dev token for actor A. The cabal S2 typed appears once, with `join_mode: request`, `voter_mode: list`, `threshold: unanimous`, `proposal_expiry_seconds: 3600`, and one member, actor A, with `can_vote: true`. Each check runs only when its scenario ran. After S4, the newest cabal named `QA duo <run>` has `join_mode: open` and two members.

## Known failures on staging

None known. Every step's route, `POST /v1/cabals`, is live.

## Not covered

- A seeing B arrive on a screen already open. Each phase relaunches the app on its actor's simulator, so S4.7 loads the cabal fresh, as `cabals/join` S1.10 does.
- Copy and Share on the invite card. `cabals/join` S1.3 to S1.5 cover them.
- A server refusal (`invalid_input`, a rate limit). Those are flow 02's outcomes, checked by its flow tests.
- A lost response retried with the same `Idempotency-Key`. MonacoTests' `CreateCabalActionTests` covers it.
- The one-line rule descriptions that change with each choice. No step reads them, because they have no accessibility identifier in `CreateGroupView`.
- The cabal picture, invite code and treasury, which other journeys own.
