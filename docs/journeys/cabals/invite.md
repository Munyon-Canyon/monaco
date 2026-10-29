---
id: cabals/invite
title: Invite a member
version: 9
milestone: M10
requires: [auth/sign-in]
actors: [A, B]
flows: [03]
xcuitest: [apps/mobile/MonacoUITests/Journeys/InviteJourney.swift, apps/mobile/MonacoUITests/Journeys/InviteJourneyUITests.swift]
---

# Invite a member

A member answers an invite from its "Invited" tile on the Cabals tab, finds the F invite sheet with Share and Copy link, and finds that a member is not offered Invite by handle. The creator of another cabal invites someone by handle from the sheet's gear, and the invitee sees the invite as a tile on their Cabals tab and answers it. The rules are [Cabals](../../architecture/cabals.md#invites).

The copy is the Cabals tab's grid and the Members tab's "Invite to <cabal>" row in [screens.md](../../screens.md#cabal-screen-cabalroute); the sheet is `CabalInviteSheet` (frame `11-invite-sheet`).

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
| S1.1 | A | tap | the Cabals tab | | Exactly one `cabals-invited-<id>` tile shows within 10 s, naming `QA pot` and reading "Invited". Old app: none, invites are new in #696 |
| S1.2 | A | tap, then tap | the Invited tile, then `cabal-invite-accept` in the invitation sheet | | The toast "You're in." shows within 10 s, and `cabal-header-name` shows within 15 s. Old app: none, invites are new in #696 |
| S1.3 | A | tap | the Members tab (`openCabalTab("members")`), then `cabal-members-invite` ("Invite to <cabal>") | | Once the toast goes and the push pre-prompt is answered "Not now" if it shows, the invite sheet shows within 10 s (`cabal-invite-title`). Old app: none |
| S1.4 | A | tap | `share-card-copy` | | `share-card-share` "Share" and `share-card-copy` "Copy link" show, there is no `cabal-invite-settings` gear and no `cabal-invite-handle-row`: a member who is not an admin is not offered Invite by handle. Copy link toasts "Link copied.". The gain row is gone from the sheet. Old app: none |

### S2 The creator invites by handle

Starts signed in (auth/sign-in).

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | tap | the Cabals tab and the `QA mine {QA.run}` tile (`app.openCabal(named:)`) | `QA mine {QA.run}` | `cabal-header-name` reads `QA mine {QA.run}` within 15 s |
| S2.2 | A | tap | the Members tab, then `cabal-members-invite` | | Once the toast goes and the push pre-prompt is answered "Not now" if it shows, the invite sheet shows within 10 s |
| S2.3 | A | tap, then tap | the gear `cabal-invite-settings`, then `cabal-invite-handle-row` ("Invite by handle", admins only) | | `invite-member-handle-field` and `invite-member-pending-empty` show within 10 s. Old app: none, invites are new in #696 |
| S2.4 | A | type, then tap | `invite-member-handle-field`, then `invite-member-send-button` | `@nobody_zz` | The toast "No one on Monaco has that handle." shows within 10 s. Old app: none, invites are new in #696 |
| S2.5 | A | clear, type, then tap | `invite-member-handle-field`, then `invite-member-send-button` | `@QA_B` | The toast "Invite sent." shows within 10 s. One `invite-member-pending-row` reads "@qa_b" and "Expires in 7 days". Old app: none, invites are new in #696 |
| S2.6 | A | tap | `invite-member-revoke-button` | | The toast "Invite revoked." shows within 10 s, and `invite-member-pending-empty` shows. Old app: none, invites are new in #696 |
| S2.7 | A | type, then tap | `invite-member-handle-field`, then `invite-member-send-button` | `qa_b` | The toast "Invite sent." shows within 10 s, and one `invite-member-pending-row` reads "@qa_b". A refused second invite would toast "They already have a pending invite or request.", so this step also proves S1.7 revoked the first. Old app: none, invites are new in #696 |
| S2.8 | B | tap | the Cabals tab | | Exactly one `cabals-invited-<id>` tile shows within 10 s, naming `QA mine {QA.run}`. Old app: none, invites are new in #696 |
| S2.9 | B | tap, then tap | the Invited tile, then `cabal-invite-decline` | | The toast "Invite declined." shows within 10 s, and no Invited tile is left. Old app: none, invites are new in #696 |

## Ground truth

After S1, A is a member of the seeded cabal. After S2, B's invite to `QA mine {QA.run}` is `denied` in `cabal_access_requests`. There is no truth script yet; the toasts are the server's answers.

## Known failures on staging

- Catch up's "Invites" row "Join" is `catch-up-join-<key>`, not `catch-up-invite-join-<id>` as #6561 names it, and `<key>` is not the cabal id. This journey does not drive it (owner: #6561).

## Not covered

- A member of an open cabal. S1 covers a member of a request cabal not seeing "Invite by handle"; `CabalInviteTests` and `InviteMemberModelTests` cover both rules on the host.
- An invite that expires before it is answered. `CabalInvitesModelTests` covers `invite_expired`.
- Joining from Catch up's "Invites" row (see Known failures).
- Pull to refresh on the Cabals tab.
