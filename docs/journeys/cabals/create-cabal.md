---
id: cabals/create-cabal
title: Create a cabal
version: 11
milestone: M10
requires: [auth/sign-in]
actors: [A, B]
flows: [02, 03]
xcuitest: [apps/mobile/MonacoUITests/Journeys/CreateCabalJourney.swift, apps/mobile/MonacoUITests/Journeys/CreateCabalJourneyUITests.swift]
---

# Create a cabal

A signed-in member opens the Cabals tab, starts a cabal with a name, an optional cover and three rules, and lands on the new cabal's screen. A friend then finds a new cabal by name and asks to join, and the creator sees them on Members & voters. The rules are [Cabals](../../architecture/cabals.md#rules). The copy is [Start a cabal](../../screens.md#cabals-tab) in `screens.md`; B finds the cabal on Discover cabals, which the same section describes.

The old app (`c838bd24`) took the same path: the Cabals tab "+" (`Groups/CabalsTabView.swift`) opened the New cabal sheet, "Start a cabal" pushed `Groups/CreateGroupView.swift` with the name and the rules, and Create pushed `GroupDetailView` with the toast. On staging the "+" pushes the form directly (the sheet is gone) and the form is `CreateCabalRoute`, which draws the UI v3 `CreateGroupView`. Each Expect cell ends with the old-app tap it matches.

The format of this doc is in [App journeys](../README.md).

Discover cabals is where `cabals-search-field` lives. A step that says "Discover cabals" reaches it from the Cabals tab by typing a name the actor has no cabal called, such as `zzqq`, in `cabals-filter-field` ("Find your cabals"), then tapping `cabals-search-public` ("Search public cabals"), which pushes Discover cabals with that text in `cabals-search-field`; an actor with no cabals taps "Explore cabals" instead. The step then replaces the text in `cabals-search-field`.

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actor A is signed in (`auth/sign-in`) and meets its P4, so the first-run gate opens the tab bar |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend`, which also builds `bin/monacoctl` |
| P3 | For S4, actor B has signed in once (`auth/sign-in`), and `apps/mobile/qa/journeys/cabals/create-cabal.setup.sh S4` ran right before the scenario. `scripts/qa/journey.py run` runs it. It marks A and B as done with onboarding and sets their display names to the `name` column of `accounts.tsv` (B is "Bartholomez"). For S1 to S3 it does nothing |

## Scenarios

### S1 The form shows the rules

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S1.1 | tap | the Cabals tab | | The toolbar "+" (`cabals-new-button`) shows within 15 s. Old app: the Cabals tab "+" in `CabalsTabView` |
| S1.2 | tap | `cabals-new-button` | | "Start a cabal" shows `create-group-name` within 10 s, with no sheet in between. Old app: the "New cabal" sheet's two rows in `CabalsTabView` came first |
| S1.3 | wait | the form | | The form shows "Cabal name", the "Cover image" picker (`create-cover-picker`) and three rule blocks: `create-rule-join` ("Who can join"), `create-rule-voters` ("Who votes") and `create-rule-expiry` ("Votes stay open"), with no "To pass" block and an info notice with the passing rule. `create-rule-expiry` starts on "1 week". Old app: `CreateGroupView` |

### S2 Create a cabal

Starts on the form (S1).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S2.1 | type | `create-group-name` | `QA pot <run>`, where `<run>` makes the name unique to the run | `create-group-submit` ("Create cabal") is enabled. Old app: the "Cabal name" field in `CreateGroupView` |
| S2.2 | tap | "Approval required" in `create-rule-join`, "People I pick" in `create-rule-voters`, "1 hour" in `create-rule-expiry` | | Each choice is selected. The journey picks the join rule itself, so it does not depend on which choice the form starts on. Old app: the four segmented rules in `CreateGroupView` |
| S2.3 | tap twice | `create-group-submit` | | The cabal screen shows within 20 s: `cabal-header-name` is the typed name, and the form is gone. The second tap proves "Creating…" holds off a duplicate. Old app: Create pushed `GroupDetailView` |
| S2.4 | wait | the toast | | "Cabal created." shows within 20 s of the S2.3 tap, read while the cabal screen loads because the toast stays up 2.5 s. Old app: the same toast over `GroupDetailView` |
| S2.5 | wait | `cabal-member-count` | | Reads "1 member", the `CabalHeaderSlot` count. Old app: the member count on `GroupDetailView`'s header |
| S2.6 | tap | Back | | Once the toast goes, the push pre-prompt (`push-pre-prompt`) is answered "Not now" if it shows.  The Cabals tab shows `cabals-list` ("Your cabals") within 10 s, and exactly one `cabal-cover-card-<id>` button in it is named the typed name. Old app: Back to `CabalsTabView`'s cabals strip |

### S3 A blank name cannot be submitted

Starts on the form (S1).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S3.1 | wait | `create-group-submit` | | "Create cabal" is disabled while the name is empty. Old app: `CreateGroupView` disabled Create on an empty name |
| S3.2 | type | `create-group-name` | three spaces | `create-group-submit` stays disabled. Old app: `CreateGroupView` trimmed the name the same way |

### S4 A friend asks to join

A creates a cabal, B finds it by name and asks to join, A approves, and A finds B on Members & voters.

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S4.1 | A | tap | the Cabals tab, then `cabals-new-button` | | "Start a cabal" shows `create-group-name` within 10 s. Old app: Cabals "+", then "Start a cabal" pushed `CreateGroupView` |
| S4.2 | A | type, tap "Approval required" in `create-rule-join`, then tap | `create-group-name`, then `create-group-submit` | `QA duo {QA.run}` | `cabal-header-name` reads `QA duo {QA.run}` within 20 s, and the toast "Cabal created." shows within 20 s of the tap, read first because it stays up 2.5 s. Old app: Create in `CreateGroupView` pushed `GroupDetailView` |
| S4.3 | B | tap, type, then tap | the Cabals tab and Discover cabals, `cabals-search-field`, then the `cabals-search-result-<id>` | `QA duo {QA.run}` | `cabal-header-name` reads `QA duo {QA.run}` and `cabal-join-button` shows within 15 s. screens.md: `CabalJoinSlot`, "Request to join" |
| S4.4 | B | tap | `cabal-join-button` | | The toast "Request sent. You'll be in once the creator says yes." shows within 10 s, and `cabal-join-requested` reads "Request pending" within 15 s. B is not a member yet. |
| S4.5 | A | tap, type, tap, then tap | the Cabals tab and Discover cabals, `cabals-search-field`, the `cabals-search-result-<id>`, `cabal-join-requests-heading`, `cabal-join-review`, then `cabal-join-approve` in the dialog | `QA duo {QA.run}` | The toast "Approved." shows and `members-requests-empty` shows. Then tapping `members-tab-members` lists a `members-list` row that names B within 15 s, and after the back button `cabal-member-count` reads "2 members" within 15 s. The cabal screen's "Member standings" (`CabalMemberBoardSlot`) is not used: it stays hidden while the pot has no stake. Old app: `GroupDetailView`'s `MemberBoardSection` |

## Ground truth

After S2, `apps/mobile/qa/journeys/cabals/create-cabal.truth.sh` reads `GET /v1/me/cabals` and `GET /v1/cabals/{id}` with a dev token for actor A. The cabal S2 typed appears once, with `join_mode: request`, `voter_mode: list`, `threshold: majority`, `proposal_expiry_seconds: 3600`, and one member, actor A, with `can_vote: true`. Each check runs only when its scenario ran. After S4, the newest cabal named `QA duo <run>` has `join_mode: request` and two members.

## Known failures on staging

Every step's route, `POST /v1/cabals`, is live. The setup script removes actor A from every earlier `QA pot` and `QA duo` cabal before S2 and S4, so the Cabals list stays short. One step still depends on services outside the repo:

- S2.3: creating a cabal depends on Privy treasury creation and public mainnet RPC, so the cabal screen can take longer than 20 s. On failure the journey attaches `app.debugDescription` and the last 50 lines of the api log (when `journey.py` started the backend) to the result.

## Not covered

- A seeing B arrive on a screen already open. The test signs each actor in again when the actor changes, so S4.5 loads the cabal fresh, as `cabals/join` S1.7 does.
- A server refusal (`invalid_input`, a rate limit). Those are flow 02's outcomes, checked by its flow tests.
- A lost response retried with the same `Idempotency-Key`. MonacoTests' `CreateCabalActionTests` covers it.
- The fixed helper line under each rule, the cover upload after create (a picked photo) and its failure toast, and the error notice after a failed create. No step picks a photo or forces a failure.
- The cabal picture and treasury, which other journeys own.
