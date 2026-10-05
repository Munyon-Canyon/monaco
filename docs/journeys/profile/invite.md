---
id: profile/invite
title: Invite friends
version: 1
milestone: M21
requires: [auth/sign-in]
actors: [A]
flows: []
xcuitest: [apps/mobile/MonacoUITests/Journeys/ProfileInviteJourney.swift, apps/mobile/MonacoUITests/Journeys/ProfileInviteJourneyUITests.swift]
---

# Invite friends

A signed-in member opens Profile, taps "Invite friends", copies their referral link and opens the share sheet. The design is `ProfileInviteSlot` in [screens.md](../../screens.md#profile-tab). The old app's version is the Profile "Invite friends" row with a `ShareLink` and a "Copy referral link" button at `c838bd24`.

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Everything [auth/sign-in](../auth/sign-in.md) needs |
| P2 | Before each scenario, `apps/mobile/qa/journeys/profile/invite.setup.sh` marks A as done with onboarding and sets A's display name through `scripts/qa/seed.sh`, so no nudge banner sits above the Profile header |

## Scenarios

### S1 Copy the invite link

Starts signed in (auth/sign-in).

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap | the Profile tab | | `profile-header` shows within 15 s (old app: the Profile tab) |
| S1.2 | A | scroll to, then tap | `profile-invite-row` | | The row reads "Invite friends" (screens.md `ProfileInviteSlot`: Row "Invite friends"; old app: Profile -> Invite friends) |
| S1.3 | A | wait | `invite-link` | | Within 15 s the screen is titled "Invite friends" and `invite-link` shows a link with "/r/" in it (screens.md: `Invite friends ──▶ InviteRoute`; old app: the referral link above Copy) |
| S1.4 | A | tap | `invite-copy` | | The toast "Link copied." shows within 10 s (old app: "Copy referral link" -> toast) |

### S2 Share the invite link

Starts signed in (auth/sign-in).

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | tap, scroll to, then tap | the Profile tab, then `profile-invite-row` | | `invite-link` shows within 15 s (screens.md `ProfileInviteSlot`: Row "Invite friends"; old app: Profile -> Invite friends) |
| S2.2 | A | tap | `invite-share` | | The button reads "Share", and the iOS share sheet opens within 5 s (old app: `ShareLink` on the invite screen) |
| S2.3 | A | close | the share sheet | | `invite-link` shows again within 5 s |

## Ground truth

`apps/mobile/qa/journeys/profile/invite.truth.sh` reads `referral_codes`. A has exactly one referral code: opening the invite screen, copying and sharing never mint a second one.

## Known failures on staging

None expected. A failing step becomes a wave 2 fix ticket.

## Not covered

- The pasteboard contents after Copy. XCUITest cannot read the simulator pasteboard without a paste prompt, so S1.4 checks the toast only.
- The handle link ("This link works too") and the deposit prompt that unlocks it. They need a funded account; a money journey covers them.
- A friend opening the link and following back. That is the referral flow, not this screen.
