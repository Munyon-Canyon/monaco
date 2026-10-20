---
id: cabals/invite
title: Invite a member
version: 7
milestone: M10
requires: [auth/sign-in]
actors: [A, B]
flows: [03]
xcuitest: [apps/mobile/MonacoUITests/Journeys/InviteJourney.swift, apps/mobile/MonacoUITests/Journeys/InviteJourneyUITests.swift]
---

# Invite a member

A member answers an invite from the Cabals tab and finds that a member is not offered Invite by handle. The creator of another cabal invites someone by handle, and the invitee sees the invite on their Cabals tab and answers it. The rules are [Cabals](../../architecture/cabals.md#invites).

The copy is the Cabals tab's `CabalsInvitesSlot` and the information screen's `CabalInfoLinksSlot` in [screens.md](../../screens.md#cabal-screen-cabalroute).

The old app (`c838bd24`) had no invites by handle, so their Expect cells say "Old app: none".

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actors A and B have each signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend`, which also builds `bin/monacoctl` |
| P3 | `apps/mobile/qa/journeys/cabals/invite.setup.sh` ran right before the scenario. `scripts/qa/journey.py run` runs it. It marks A and B as done with onboarding, gives them a display name and a handle (B is `@qa_b`), declines every invite waiting on A or B. For S1 a pooled dev user creates the cabal `QA pot <time>` and invites A. For S2 A creates the cabal `QA mine {QA.run}`. |

The setup runs before every scenario, because S1 uses up the invite it makes and S2 needs A to be the creator of a new cabal. It creates the cabal through the API, so this journey does not depend on `cabals/create-cabal`.

## Scenarios

### S1 Accept an invite

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap | the Cabals tab | | `cabals-invites` shows one `cabal-invite-row` within 10 s, naming `QA pot` and reading "invited you". screens.md: `CabalsInvitesSlot`, "Cabal invites". Old app: none, invites are new in #696 |
| S1.2 | A | tap | `cabal-invite-accept` | | The toast "You're in." shows within 10 s, and `cabal-details-button` shows within 15 s. Old app: none, invites are new in #696 |
| S1.3 | A | tap | `cabal-details-button` | | Once the toast goes and the push pre-prompt is answered "Not now" if it shows, the Cabal information screen shows within 10 s (`cabal-members-row`) and `cabal-invite-member-row` shows within 10 s. screens.md: `CabalInfoLinksSlot`, "Invite to this cabal". Old app: none |
| S1.4 | A | tap, then scroll | `cabal-invite-member-row`, then the "Share cabal" sheet | | `share-card-share` shows within 10 s. After a swipe up `cabal-invite-gain-row` "Share your gain as an image" shows, and there is no `cabal-invite-handle-row`: a member who is not the creator is not offered Invite by handle (cabals.md: only the creator invites). Old app: none |

### S2 The creator invites by handle

Starts signed in (auth/sign-in).

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | tap, type, then tap | the Cabals tab and Discover cabals, `cabals-search-field`, then the `cabals-search-result-<id>` | `QA mine {QA.run}` | `cabal-header-name` reads `QA mine {QA.run}` and `cabal-details-button` shows within 15 s |
| S2.2 | A | tap | `cabal-details-button` | | Once the toast goes and the push pre-prompt is answered "Not now" if it shows, the Cabal information screen shows within 10 s (`cabal-members-row`) and `cabal-invite-member-row` shows within 10 s |
| S2.3 | A | tap, then tap | `cabal-invite-member-row`, then `cabal-invite-handle-row` in the "Share cabal" sheet (swipe up the sheet when the row is below the fold) | | `invite-member-handle-field` and `invite-member-pending-empty` show within 10 s. Old app: none, invites are new in #696 |
| S2.4 | A | type, then tap | `invite-member-handle-field`, then `invite-member-send-button` | `@nobody_zz` | The toast "No one on Monaco has that handle." shows within 10 s. Old app: none, invites are new in #696 |
| S2.5 | A | clear, type, then tap | `invite-member-handle-field`, then `invite-member-send-button` | `@QA_B` | The toast "Invite sent." shows within 10 s. One `invite-member-pending-row` reads "@qa_b" and "Expires in 7 days". Old app: none, invites are new in #696 |
| S2.6 | A | tap | `invite-member-revoke-button` | | The toast "Invite revoked." shows within 10 s, and `invite-member-pending-empty` shows. Old app: none, invites are new in #696 |
| S2.7 | A | type, then tap | `invite-member-handle-field`, then `invite-member-send-button` | `qa_b` | The toast "Invite sent." shows within 10 s, and one `invite-member-pending-row` reads "@qa_b". A refused second invite would toast "They already have a pending invite or request.", so this step also proves S1.7 revoked the first. Old app: none, invites are new in #696 |
| S2.8 | B | tap | the Cabals tab | | `cabals-invites` shows exactly one `cabal-invite-row` within 10 s, naming `QA mine {QA.run}`. screens.md: `CabalsInvitesSlot`. Old app: none, invites are new in #696 |
| S2.9 | B | tap | `cabal-invite-decline` | | The toast "Invite declined." shows within 10 s, and no `cabal-invite-row` is left. screens.md: `CabalsInvitesSlot`, Decline. Old app: none, invites are new in #696 |

## Ground truth

After S1, A is a member of the seeded cabal. After S2, B's invite to `QA mine {QA.run}` is `denied` in `cabal_access_requests`. There is no truth script yet; the toasts are the server's answers.

## Known failures on staging

None known.

## Not covered

- A member of an open cabal. S1 covers a member of a request cabal not seeing "Invite by handle"; `CabalInviteTests` and `InviteMemberModelTests` cover both rules on the host.
- An invite that expires before it is answered. `CabalInvitesModelTests` covers `invite_expired`.
- Pull to refresh on the Cabals tab.
