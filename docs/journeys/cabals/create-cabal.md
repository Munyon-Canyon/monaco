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

A signed-in member opens the Cabals tab, starts a cabal with a name and four rules, and lands on the new cabal's screen. The rules are [Cabals](../../architecture/cabals.md#rules).

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
| S1.1 | tap | the Cabals tab | | The toolbar "+" (`cabals-new-button`) shows within 15 s |
| S1.2 | tap | `cabals-new-button` | | The New cabal sheet shows `new-cabal-create-row` and `new-cabal-join-row` within 10 s |
| S1.3 | tap | `new-cabal-create-row` | | `create-group-name` shows within 10 s, with `create-rule-join`, `create-rule-voters`, `create-rule-threshold` and `create-rule-expiry` |

### S2 Create a cabal

Starts on the form (S1).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S2.1 | type | `create-group-name` | `QA pot <run>`, where `<run>` makes the name unique to the run | `create-group-submit` is enabled |
| S2.2 | tap | "I approve" in `create-rule-join`, "Just me" in `create-rule-voters`, "Everyone agrees" in `create-rule-threshold`, "1 hour" in `create-rule-expiry` | | Each choice is selected |
| S2.3 | tap twice | `create-group-submit` | | The cabal screen shows within 20 s: `cabal-header-name` is the typed name, and the form is gone |
| S2.4 | wait | the toast | | "Cabal created." shows within 5 s of S2.3 |
| S2.5 | wait | `cabal-member-count` | | Reads "1 member" |
| S2.6 | tap | Back | | The Cabals tab shows `cabals-list` within 10 s, exactly one card in it is named the typed name, and the dashed `cabals-list-new` card is there |

### S3 A blank name cannot be submitted

Starts on the form (S1).

| Step | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- |
| S3.1 | wait | `create-group-submit` | | Disabled while the name is empty |
| S3.2 | type | `create-group-name` | three spaces | `create-group-submit` stays disabled |

## Ground truth

After S2, `apps/mobile/qa/journeys/cabals/create-cabal.truth.sh` reads `GET /v1/me/cabals` and `GET /v1/cabals/{id}` with a dev token for actor A. The newest cabal named `QA pot <run>` appears once, with `join_mode: request`, `voter_mode: list`, `threshold: unanimous`, `proposal_expiry_seconds: 3600`, and one member, actor A, with `can_vote: true`.

## Not covered

- A server refusal (`invalid_input`, a rate limit). Those are flow 02's outcomes, checked by its flow tests.
- A lost response retried with the same `Idempotency-Key`. MonacoTests' `CreateCabalActionTests` covers it.
- The cabal picture, invite code and treasury, which other journeys own.
