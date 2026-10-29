---
id: profile/overview
title: Your profile
version: 10
milestone: M9
requires: [auth/sign-in]
actors: [A]
flows: [01]
xcuitest: [apps/mobile/MonacoUITests/Journeys/ProfileOverviewJourney.swift, apps/mobile/MonacoUITests/Journeys/ProfileOverviewJourneyUITests.swift]
---

# Your profile

A signed-in member opens the Profile tab, reads their profile card, month standing, cash row and Cabals tab, pulls to refresh, opens a cabal from the list, opens a block explorer, and signs out. The design is the [Profile tab](../../screens.md#profile-tab). The old app's version is `Profile/ProfileTabView.swift` at `c838bd24`.

The format of this doc is in [App journeys](../README.md). The Old app column names the old app's tap or element for each step, or says the step is new in the spec.

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Everything [auth/sign-in](../auth/sign-in.md) needs |
| P2 | Before each scenario, `apps/mobile/qa/journeys/profile/overview.setup.sh` sets A's `auth_state` to `ONBOARDING_COMPLETED`, so the onboarding nudge stays out of the way |
| P3 | Before each scenario, the setup script reads A's portfolio (`GET /v1/me/portfolio`), because the Cabals tab lists only cabals A holds shares in, and hands the test its largest cabal's id as `cabalID` and its name as `cabalName`. When A holds none, it creates a cabal named `QA overview {QA.run}` and hands that over, and S1.3 fails because the app lists no unfunded cabal |

## Scenarios

### S1 The tab shows every section

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S1.1 | tap | the Profile tab | | Within 15 s, `profile-card` shows with `profile-display-name` and `profile-handle` reading "@" and the handle, then "joined" with a month and year | `profile-root`, then `profile-card` |
| S1.2 | wait | `profile-stat-cabals` | | Within 15 s, the card shows "Positions" (`profile-positions`) and "P&L · 1M" (`profile-pnl-1m`), and the counts line ends with "cabal" or "cabals" by count (`profile-stat-cabals`, 1 or more). `profile-standing` reads "% right calls this month" or the unranked line an account under five resolved votes gets | The count badge on "Your cabals" (`cabalRows.count`) |
| S1.3 | tap, then scroll to | the Cabals tab (`profile-tab-cabals`), then `profile-cabal-row-{cabalID}` | | The tab lists the row for `cabalName` with its ownership share and "admin" or "voter" | `ProfileCabalsSection` rows |
| S1.4 | find | `profile-settings-row` | | The round button in the title bar reads "Settings" | None, new in spec ("Settings" row) |
| S1.5 | read | `profile-cash-row`, `profile-add-money-link` | | The cash row reads "Cash", "USDC" and "3.5% APY". `profile-add-money-link` reads "Deposit" above the tab bar | None, new in spec |

### S2 Pull to refresh

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S2.1 | tap | the Profile tab | | `profile-card` shows within 15 s | `profile-root` |
| S2.2 | pull down | `profile-card` | | Within 15 s of the release, `profile-card`, `profile-stat-cabals` and the Cabals tab and `profile-cabal-row-{cabalID}` all still show. The screen never goes blank | `.refreshable` on `ProfileTabView` |

### S3 Open a cabal from the Cabals tab

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S3.1 | tap, tap, then tap | the Profile tab, `profile-tab-cabals`, then `profile-cabal-row-{cabalID}` | | The cabal screen shows within 15 s, with `cabal-header-name` and `cabalName` on screen | A `ProfileCabalsSection` row opens the cabal |

### S4 Open a block explorer

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S4.1 | tap, then tap | the Profile tab, then `profile-settings-row` | | The "Settings" screen shows within 10 s, with `settings-advanced` reading "Block explorers" | `profile-advanced-link` ("Advanced", "Block explorers") on the Profile tab |
| S4.2 | tap | `settings-advanced` | | The "Block explorers" screen shows within 5 s, with `settings-explorer-solscan` reading "Solscan explorer" | The Advanced screen's explorer list |
| S4.3 | tap | `settings-explorer-solscan` | | Safari is in the foreground within 15 s | The Solscan row opens Safari |

### S5 Share your return

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S5.1 | tap | the Profile tab, then `profile-share-button` | | Within 15 s the share screen (`share-screen`) shows, pushed over Profile (screens.md: the share round in the title bar) | New in the spec |
| S5.2 | tap | the Back button | | The share screen closes and `profile-card` still shows within 5 s | New in the spec |

### S6 Sign out

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S6.1 | tap, tap, then scroll to and tap | the Profile tab, `profile-settings-row`, then `profileSignOutButton` in Settings | | Within 5 s the confirm panel shows "Sign out of Monaco?" and "You can sign back in with your existing account. This does not withdraw funds or leave your cabals." | `profile-sign-out`, with "Sign out of Monaco?" |
| S6.2 | tap | `profile-sign-out-cancel` ("Stay signed in") | | The confirm panel closes within 5 s, and the "Settings" screen still shows | "Cancel" |
| S6.3 | tap, then tap | `profileSignOutButton`, then `profile-sign-out-confirm` | | Within 15 s the tab bar is gone. The invite gate (`invite-paste-screen`) shows first and is skipped with `invite-skip-button`, then the login form shows | `profile-sign-out-confirm` |

### S7 Totals and pot values

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S7.1 | tap | the Profile tab | | Within 15 s, `profile-positions` ("Positions") and `profile-pnl-1m` ("P&L · 1M") read a figure, not "loading" or "unavailable": "Positions" in dollars | The old header's portfolio total |
| S7.2 | tap, then scroll to | `profile-tab-cabals`, then `profile-cabal-row-{cabalID}` | | The row reads the share in dollars ("$") and does not read "no share" | `ProfileCabalsSection` row value |

## Ground truth

Signing out ends the session the run opened, and nothing on A's account changes. `apps/mobile/qa/journeys/profile/overview.truth.sh` checks through `apps/mobile/qa/journeys/psql.sh` that A is still a member of the cabal named `QA overview {QA.run}` and that A's `auth_state` is still `ONBOARDING_COMPLETED`.

## Known failures on staging

None known.

## Not covered

- The error state "Couldn't load your portfolio." with "Try again" (old app: `profile-error`). `ProfileScreen` owns one `PortfolioModel` that the identity card and `ProfileCabalsSlot` read, so a failed read draws "—" in the card, one error row under it and nothing in the cabals slot. The local backend cannot be made to fail one read on cue; `PortfolioModelTests` covers the failed state.
- A value that changes between two loads on pull to refresh. Steps run one at a time, so nothing writes to the backend while the scenario runs. S2 proves the refresh keeps every section on screen.
- "Your cabals" when empty ("No cabals yet" / "Start a cabal or join one from the Cabals tab."). P3 makes A a member, and A cannot leave every cabal without changing other journeys' starting state.
- The follow counts, the money strip, "Invite friends" and "Find friends". Their own journeys cover them.
