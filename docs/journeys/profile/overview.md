---
id: profile/overview
title: Your profile
version: 2
milestone: M9
requires: [auth/sign-in]
actors: [A]
flows: [01]
xcuitest: [apps/mobile/MonacoUITests/Journeys/ProfileOverviewJourney.swift, apps/mobile/MonacoUITests/Journeys/ProfileOverviewJourneyUITests.swift]
---

# Your profile

A signed-in member opens the Profile tab, reads their stats band and their cabals, pulls to refresh, opens a cabal from the list, opens a block explorer, and signs out. The design is the [Profile tab](../../screens.md#profile-tab). The old app's version is `Profile/ProfileTabView.swift` at `c838bd24`.

The format of this doc is in [App journeys](../README.md). The Old app column names the old app's tap or element for each step, or says the step is new in the spec.

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Everything [auth/sign-in](../auth/sign-in.md) needs |
| P2 | Before each scenario, `apps/mobile/qa/journeys/profile/overview.setup.sh` sets A's `auth_state` to `ONBOARDING_COMPLETED`, so no nudge banner sits above the header |
| P3 | Before each scenario, the setup script makes A a member of a cabal named `QA overview {QA.run}` through the API, unless A already is one. It hands the test the cabal's id as `cabalID` and its name as `cabalName` |

## Scenarios

### S1 The tab shows every section

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S1.1 | tap | the Profile tab | | Within 15 s, `profile-header` shows with `profile-display-name`, `profile-handle` reading "@" and the handle, and `profile-member-since` reading "Member since" with a month and year | `profile-root`, then `profile-header` |
| S1.2 | wait | `profile-stat-cabals` | | Within 15 s, the band shows three columns, "In cabals", "All time" and "Cabals". `profile-stat-cabals` reads a whole number of 1 or more | The count badge on "Your cabals" (`cabalRows.count`) |
| S1.3 | scroll to | `cabal-row-{cabalID}` | | "Your cabals" shows, with the row for `cabalName` | `ProfileCabalsSection` rows |
| S1.4 | scroll to | `profile-settings-row` | | The row reads "Settings" | None, new in spec ("Settings" row) |

### S2 Pull to refresh

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S2.1 | tap | the Profile tab | | `profile-header` shows within 15 s | `profile-root` |
| S2.2 | pull down | `profile-header` | | Within 15 s of the release, `profile-header`, `profile-stat-cabals` and `cabal-row-{cabalID}` all still show. The screen never goes blank | `.refreshable` on `ProfileTabView` |

### S3 Open a cabal from Your cabals

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S3.1 | tap, then tap | the Profile tab, then `cabal-row-{cabalID}` | | The cabal screen shows within 15 s, with `cabal-details-button` and `cabalName` on screen | A `ProfileCabalsSection` row opens the cabal |

### S4 Open a block explorer

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S4.1 | tap, then tap | the Profile tab, then `profile-settings-row` | | The "Settings" screen shows within 10 s, with `settings-advanced` reading "Advanced" and "Block explorers" | `profile-advanced-link` ("Advanced", "Block explorers") on the Profile tab |
| S4.2 | tap | `settings-advanced` | | The "Advanced" screen shows within 5 s, with `settings-explorer-solscan` reading "Solscan explorer" | The Advanced screen's explorer list |
| S4.3 | tap | `settings-explorer-solscan` | | Safari is in the foreground within 15 s | The Solscan row opens Safari |

### S5 Sign out

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S5.1 | tap, then scroll to and tap | the Profile tab, then `profileSignOutButton` | | Within 5 s the confirm shows "Sign out of Monaco?" and "Your money stays where it is. You'll need a new code to sign back in." | `profile-sign-out`, with "Sign out of Monaco?" |
| S5.2 | tap | outside the dialog | | The confirm closes within 5 s, and `profile-header` still shows. On iOS 27 the confirm is a popover with no Cancel button | "Cancel" |
| S5.3 | tap, then tap | `profileSignOutButton`, then `profile-sign-out-confirm` | | The login form shows within 15 s, and the tab bar does not | `profile-sign-out-confirm` |

### S6 Totals and pot values

Starts signed in (auth/sign-in). Every step here fails on staging until the routes land (Known failures).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S6.1 | tap | the Profile tab | | Within 15 s, `profile-stat-in-cabals` and `profile-stat-all-time` read a figure, not "Not available yet", and `profile-stats-coming` does not show | The old header's portfolio total |
| S6.2 | scroll to | `cabal-row-{cabalID}` | | The row shows the cabal's value, and `profile-cabals-coming` ("Pot values show up here soon.") does not show | `ProfileCabalsSection` row value |

## Ground truth

Signing out ends the session the run opened, and nothing on A's account changes. `apps/mobile/qa/journeys/profile/overview.truth.sh` checks through `apps/mobile/qa/journeys/psql.sh` that A is still a member of the cabal named `QA overview {QA.run}` and that A's `auth_state` is still `ONBOARDING_COMPLETED`.

## Known failures on staging

| Step | What fails | Blocked by |
| --- | --- | --- |
| S6.1 | "In cabals" and "All time" show "—" with "Your totals show up here soon.", because no route serves the member's totals | #2140 |
| S6.2 | Your cabals rows show no value, with "Pot values show up here soon.", because no route serves the pot or the member's slice | #2140, #2136 |

## Not covered

- The error state "Couldn't load your stats." / "Couldn't load your cabals." with "Try again" (old app: `profile-error`). The local backend cannot be made to fail one read on cue. `ProfileStatsSlot` and `ProfileCabalsSlot` share `CabalsTabModel`, whose failed state `CabalsTabModelTests` covers.
- A value that changes between two loads on pull to refresh. Steps run one at a time, so nothing writes to the backend while the scenario runs. S2 proves the refresh keeps every section on screen.
- "Your cabals" when empty ("No cabals yet" / "Start a cabal or join one from the Cabals tab."). P3 makes A a member, and A cannot leave every cabal without changing other journeys' starting state.
- The follow counts, the balance row, "Invite friends" and "Find friends". Their own journeys cover them.
