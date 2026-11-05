---
id: profile/overview
title: Your profile
version: 16
rpc: fake-trade
milestone: M9
requires: [auth/sign-in]
actors: [A]
flows: [01]
xcuitest: [apps/mobile/MonacoUITests/Journeys/ProfileOverviewJourney.swift, apps/mobile/MonacoUITests/Journeys/ProfileOverviewJourneyUITests.swift]
---

# Your profile

Frames: `docs/design/v5/screens/10-profile-own.png`, `docs/design/v5/screens/11-share-A.png`.

A signed-in member opens the Profile tab, reads their profile card, month standing, cash row and Cabals tab, pulls to refresh, opens a cabal from the list, opens a block explorer, and signs out. The design is the [Profile tab](../../screens.md#profile-tab). The old app's version is `Profile/ProfileTabView.swift` at `c838bd24`.

The format of this doc is in [App journeys](../README.md). The Old app column names the old app's tap or element for each step, or says the step is new in the spec.

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Everything [auth/sign-in](../auth/sign-in.md) needs |
| P2 | Before each scenario, `apps/mobile/qa/journeys/profile/overview.setup.sh` sets A's `auth_state` to `ONBOARDING_COMPLETED`, so the onboarding nudge stays out of the way |
| P3 | Before each scenario, the setup script reads A's portfolio (`GET /v1/me/portfolio`), which lists every cabal A is a member of, and hands the test the cabal where A holds shares and the stake is worth the most (a cabal A holds no shares in reads a dash instead of a dollar share, so S7.2 would fail on it), with its id as `cabalID` and its name as `cabalName`. When A holds no shares anywhere, it seeds one with `qa_fake_holding A JRNYAx 25 "QA overview {QA.run}"` on the fakes: a cabal, a fund sweep from A's fake USDC, a buy of half, A votes yes, and the setup waits for the executed buy. It hands that cabal over |
| P4 | Before each scenario, the setup script reads A's profile (`GET /v1/users/{id}`), the call the screen makes, and hands the test `hasStanding`: `1` when the backend returns a `standing` for A, `0` when it does not. The backend drops it for an account with no resolved call in the current month, which is every account on the first days of a month |

## Scenarios

### S1 The tab shows every section

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S1.1 | tap | the Profile tab | | Within 15 s, `profile-card` shows with `profile-display-name` and `profile-handle` reading "@" and the handle, then "joined" with a month and year | `profile-root`, then `profile-card` |
| S1.2 | wait | `profile-stat-cabals` | | Within 15 s, the card shows "Positions" (`profile-positions`) and "This month" (`profile-pnl-1m`), with no profit-and-loss label on the card, and the counts line ends with "cabal" or "cabals" by count (`profile-stat-cabals`, 1 or more). When `hasStanding` is `1`, `profile-standing` reads the ranked line, matching `^\d+ of \d+ right this month · \d+(st|nd|rd|th) of \d+$` with no "%" and no "#", or the unranked line containing "resolved this month". When it is `0`, the step does not look for it | The count badge on "Your cabals" (`cabalRows.count`) |
| S1.3 | tap, then scroll to | the Cabals tab (`profile-tab-cabals`), then `profile-cabal-row-{cabalID}` | | The tab lists the row for `cabalName` with its ownership share ("%") and, when A is an admin there, "admin". The row never reads "voter" | `ProfileCabalsSection` rows |
| S1.4 | find | `profile-settings-row` | | The round button in the title bar reads "Settings" | None, new in spec ("Settings" row) |
| S1.5 | read | `profile-cash-row`, `profile-add-money-link` | | The cash row reads "Cash" and "USDC", with no APY text and no `profile-apy-info` button. `profile-add-money-link` reads "Deposit" above the tab bar | None, new in spec |

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
| S3.1 | tap, tap, then tap | the Profile tab, `profile-tab-cabals`, then `profile-cabal-row-{cabalID}` | | The cabal opens on Chat: `cabal-inline-title` reads `cabalName` and `chat-view` shows within 15 s | A `ProfileCabalsSection` row opens the cabal |

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
| S5.1 | tap | the Profile tab, then `profile-share-button` | | Within 15 s the share screen (`share-screen`) shows, pushed over Profile (screens.md: the share round in the title bar). Within 15 s `share-send` (“Share”) and `share-copy-image` (“Copy image”) both show and are enabled, side by side at the default text size (equal `frame.minY`, the second starting right of the first). Profile shares the portfolio card, so the toggles `share-eye-pnl`, `share-eye-trades` and `share-eye-code` read “Show $”, “Show trades” and “Show code” | New in the spec |
| S5.3 | tap | `share-copy-image` | | Within 10 s the toast reads "Image copied.", and no toast reads "Couldn’t make the image. Try again." The test waits for the toast to go before S5.2 | New in the spec |
| S5.2 | tap | the Back button | | The share screen closes and `profile-card` still shows within 5 s | New in the spec |

### S6 Sign out

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S6.1 | tap, tap, then scroll to and tap | the Profile tab, `profile-settings-row`, then `profileSignOutButton` in Settings | | Within 5 s the confirm panel shows "Sign out of Monaco?" and "You can sign back in with your existing account. This does not withdraw funds or leave your cabals." | `profile-sign-out`, with "Sign out of Monaco?" |
| S6.2 | tap | `profile-sign-out-cancel` ("Stay signed in") | | The confirm panel closes within 5 s, and the "Settings" screen still shows | "Cancel" |
| S6.3 | tap | `settings-sign-out-everywhere` ("Sign out of all devices", the row above Sign out) | | Within 5 s the confirm panel shows "Sign out of all devices?" (`profile-sign-out-everywhere-title`) and "This ends every session on your account, including a lost phone. You can sign back in with your existing account. This does not withdraw funds or leave your cabals.", with `profile-sign-out-everywhere-confirm` reading "Sign out of all devices" and `profile-sign-out-everywhere-cancel` reading "Stay signed in" | None, new in spec |
| S6.4 | tap | `profile-sign-out-everywhere-cancel` ("Stay signed in") | | The confirm panel closes within 5 s, and the "Settings" screen still shows | None, new in spec |
| S6.5 | tap, then tap | `profileSignOutButton`, then `profile-sign-out-confirm` | | Within 15 s the tab bar is gone. The invite gate (`invite-paste-screen`) shows first and is skipped with `invite-skip-button`, then the login form shows | `profile-sign-out-confirm` |

### S7 Totals and pot values

Starts signed in (auth/sign-in).

| Step | Action | Target | Input | Expect | Old app |
| --- | --- | --- | --- | --- | --- |
| S7.1 | tap | the Profile tab | | Within 15 s, `profile-positions` ("Positions") and `profile-pnl-1m` ("This month") read a figure, not "loading" or "unavailable": "Positions" in dollars | The old header's portfolio total |
| S7.2 | tap, then scroll to | `profile-tab-cabals`, then `profile-cabal-row-{cabalID}` | | The row reads the share in dollars ("$") and does not read "no share" | `ProfileCabalsSection` row value |

## Ground truth

Signing out ends the session the run opened, and nothing on A's account changes. `apps/mobile/qa/journeys/profile/overview.truth.sh` checks through `apps/mobile/qa/journeys/psql.sh` that A is still a member of the cabal named `QA overview {QA.run}` and that A's `auth_state` is still `ONBOARDING_COMPLETED`.

## Known failures on staging

- S1.2: the test checks "P&L", which the doc step does not quote. Blocked by #6198.
- S1.5: the test checks "3.5% APY", which the doc step does not quote. Blocked by #6198.
## Not covered

- The error state "Couldn't load your portfolio." with "Try again" (old app: `profile-error`). `ProfileScreen` owns one `PortfolioModel` that the identity card and `ProfileCabalsSlot` read, so a failed read draws "—" in the card, one error row under it and nothing in the cabals slot. The local backend cannot be made to fail one read on cue; `PortfolioModelTests` covers the failed state.
- A value that changes between two loads on pull to refresh. Steps run one at a time, so nothing writes to the backend while the scenario runs. S2 proves the refresh keeps every section on screen.
- "Your cabals" when empty ("No cabals yet" / "Start a cabal or join one from the Cabals tab."). P3 makes A a member, and A cannot leave every cabal without changing other journeys' starting state.
- The follow counts, the money strip, "Invite friends" and "Find friends". Their own journeys cover them.
- What Copy image put on the pasteboard. Reading `UIPasteboard` from the test runner can raise the system paste prompt, so `ShareScreenTests` covers the image and S5.3 reads the toast, which shows only after `ShareActions.copy(_:)` succeeds. The share card's look is a picture, covered by `ShareCardHarnessTests` and the parity sheet.
