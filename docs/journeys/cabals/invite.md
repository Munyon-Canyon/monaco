---
id: cabals/invite
title: Invite a member
version: 3
milestone: M10
requires: [auth/sign-in]
actors: [A, B]
flows: [03]
xcuitest: [apps/mobile/MonacoUITests/Journeys/InviteJourney.swift, apps/mobile/MonacoUITests/Journeys/InviteJourneyUITests.swift]
---

# Invite a member

A member answers an invite from the Cabals tab, then invites someone else to the same cabal by handle. The invitee sees the invite on their Cabals tab and answers it. A member also copies and shares the cabal's invite code. The rules are [Cabals](../../architecture/cabals.md#invites).

The copy is the Cabals tab's `CabalsInvitesSlot` and the details sheet's `CabalInviteMemberSlot` in [screens.md](../../screens.md#cabal-screen-cabalroute).

The old app (`c838bd24`) had no invites. A member opened "i" on the cabal (`Groups/GroupDetailView.swift`), which showed `Groups/GroupDetailsSheet.swift` with the invite code, "Copy code" and "Share", and sent the code outside the app. That path is `cabals/join` S1.3 to S1.5. Invite by handle and the Cabals tab's invite rows are new in #696, so their Expect cells say "Old app: none".

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actors A and B have each signed in once (`auth/sign-in`), and their `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend`, which also builds `bin/monacoctl` |
| P3 | `apps/mobile/qa/journeys/cabals/invite.setup.sh` ran right before the scenario. `scripts/qa/journey.py run` runs it. It marks A and B as done with onboarding, gives them a display name and a handle (B is `@qa_b`), declines every invite waiting on A or B, and has a new dev user create the open cabal `QA pot <time>` and invite A. For S2 it instead has A create the open cabal `QA share {QA.run}` |

The setup runs before every scenario, because S1 uses up the invite it makes. It creates the cabal through the API, so this journey does not depend on `cabals/create-cabal`.

## Scenarios

### S1 Accept an invite, then invite someone

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap | the Cabals tab | | `cabals-invites` shows one `cabal-invite-row` within 10 s, naming `QA pot` and reading "invited you". screens.md: `CabalsInvitesSlot`, "Cabal invites". Old app: none, invites are new in #696 |
| S1.2 | A | tap | `cabal-invite-accept` | | The toast "You're in." shows within 10 s, and `cabal-details-button` shows within 15 s. Old app: none, invites are new in #696 |
| S1.3 | A | tap | `cabal-details-button` | | Once the toast goes and the push pre-prompt is answered "Not now" if it shows, the Cabal details sheet (`cabal-details-done`) shows within 10 s and `cabal-invite-member-row` shows within 10 s. screens.md: `CabalInviteMemberSlot`, "Invite someone". Old app: "i" opened `GroupDetailsSheet`, which had the invite code and no invite by handle |
| S1.4 | A | tap | `cabal-invite-member-row` | | `invite-member-handle-field` and `invite-member-pending-empty` show within 10 s. Old app: none, invites are new in #696 |
| S1.5 | A | type, then tap | `invite-member-handle-field`, then `invite-member-send-button` | `@nobody_zz` | The toast "No one on Monaco has that handle." shows within 10 s. Old app: none, invites are new in #696 |
| S1.6 | A | clear, type, then tap | `invite-member-handle-field`, then `invite-member-send-button` | `@QA_B` | The toast "Invite sent." shows within 10 s. One `invite-member-pending-row` reads "@qa_b" and "Expires in 7 days". Old app: none, invites are new in #696 |
| S1.7 | A | tap | `invite-member-revoke-button` | | The toast "Invite revoked." shows within 10 s, and `invite-member-pending-empty` shows. Old app: none, invites are new in #696 |
| S1.8 | A | type, then tap | `invite-member-handle-field`, then `invite-member-send-button` | `qa_b` | The toast "Invite sent." shows within 10 s, and one `invite-member-pending-row` reads "@qa_b". A refused second invite would toast "They already have a pending invite or request.", so this step also proves S1.7 revoked the first. Old app: none, invites are new in #696 |
| S1.9 | B | tap | the Cabals tab | | `cabals-invites` shows exactly one `cabal-invite-row` within 10 s, naming `QA pot`. screens.md: `CabalsInvitesSlot`. Old app: none, invites are new in #696 |
| S1.10 | B | tap | `cabal-invite-decline` | | The toast "Invite declined." shows within 10 s, and no `cabal-invite-row` is left. screens.md: `CabalsInvitesSlot`, Decline. Old app: none, invites are new in #696 |

### S2 Copy and share the invite code

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | tap, type, then tap | the Cabals tab, `cabals-search-field`, then the `cabals-search-result-<id>` | `QA share {QA.run}` | `cabal-header-name` reads `QA share {QA.run}` within 15 s. Old app: a search result in `CabalsTabView` pushed `GroupDetailView` |
| S2.2 | A | tap | `cabal-details-button` | | `cabal-invite-card` ("Invite code") shows within 10 s. `cabal-invite-code` is 10 characters, `cabal-invite-copy` reads "Copy code" and `cabal-invite-share` reads "Share". screens.md: `CabalInviteCodeSlot`. Old app: "i" opened `GroupDetailsSheet` with the same card |
| S2.3 | A | tap | `cabal-invite-copy` | | `cabal-invite-copy` reads "Copied" within 2 s. Old app: "Copy code" in `GroupDetailsSheet` |
| S2.4 | A | tap, then close | `cabal-invite-share`, then the share sheet's Close | | The system share sheet shows within 5 s. After Close, `cabal-invite-card` shows again within 5 s. Old app: the `ShareLink` "Share" in `GroupDetailsSheet` |

## Ground truth

After S1, A is a member of the seeded cabal, and B's invite to it is `denied` in `cabal_access_requests`. There is no truth script yet; the toasts are the server's answers.

## Known failures on staging

None known.

## Not covered

- The creator of a request cabal inviting, and a member of a request cabal not seeing "Invite someone". `CabalInviteTests` and `InviteMemberModelTests` cover both rules on the host.
- An invite that expires before it is answered. `CabalInvitesModelTests` covers `invite_expired`.
- Pull to refresh on the Cabals tab.
- What the share sheet sends and the copied code on the clipboard. The share sheet and the clipboard belong to iOS; S2.4 proves the sheet opens, and `cabals/join` S1.8 proves a pasted code joins.
