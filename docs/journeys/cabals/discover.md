---
id: cabals/discover
title: Discover a public cabal
version: 1
milestone: UI v3
requires: [auth/sign-in]
actors: [A]
flows: [03]
xcuitest: [apps/mobile/MonacoUITests/Journeys/CabalsDiscoverJourney.swift, apps/mobile/MonacoUITests/Journeys/CabalsDiscoverJourneyUITests.swift]
---

# Discover a public cabal

A member opens Discover cabals from the Cabals tab, reads what it shows with nothing to search for, finds a public cabal by name, opens its public screen, asks to join from there and sees "Request pending" on the screen and on the Discover row. [cabals/browse](browse.md) asks from the Discover row and [cabals/join](join.md) walks the whole approval, so this journey is the Discover screen and the public cabal screen themselves. The spec is [Discover cabals](../../screens.md#cabals-tab) and [Public cabal](../../screens.md#public-cabal-cabalroute-non-member). The rules are [Cabals](../../architecture/cabals.md#join-and-access-requests).

The format of this doc is in [App journeys](../README.md).

Discover cabals is reached the way [cabals/browse](browse.md) reaches it: type a name A has no cabal called in `cabals-filter-field`, then tap `cabals-search-public` ("Search public cabals"). The Discover search field then holds that text.

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actor A has signed in once (`auth/sign-in`), and its `privy_user_id` is in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py` starts the backend with `just run backend` |
| P3 | `apps/mobile/qa/journeys/cabals/discover.setup.sh` ran right before the scenario. It marks A as done with onboarding. For S2, a pooled dev user "QA host" (`--pool discover-host`) creates the cabal `QA discover {QA.run}` with "Approval required" |

## Scenarios

### S1 Nothing typed yet

Starts signed in (auth/sign-in).

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap, type, then tap | the Cabals tab, `cabals-filter-field`, then `cabals-search-public` | `zzqq nobody` | Within 10 s, `cabals-search-field` holds the typed text and `cabals-search-empty` reads "No matching cabals". screens.md: "Search public cabals" pushes Discover cabals with the typed text; no match is "No matching cabals" / "Try another name." |
| S1.2 | A | clear | `cabals-search-field` | | Within 10 s, `cabals-search-empty` is gone and `cabals-browse-coming` reads "Browsing every public cabal opens soon. Search by name to find more.". screens.md: an empty query shows the public cabal board's rows and that caption, because the API has no list of every public cabal |

### S2 Open a public cabal and ask

Starts on Discover cabals with an empty field (S1).

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | type | `cabals-search-field` | `QA discover {QA.run}` | Within 10 s, one `cabals-search-result-<id>` names `QA discover {QA.run}` with "1 member", and its `cabals-search-enter-<id>` reads "Request" and is labelled "Request to join QA discover {QA.run}". screens.md: a row's right side is "Request" for an approval cabal (screens.md also says the row names "Approval required", which the app does not draw: the policy shows on the public cabal screen, S2.2) |
| S2.2 | A | tap | the `cabals-search-result-<id>` | | Within 15 s, `public-cabal` shows under the title `QA discover {QA.run}`, with `public-cabal-joining` reading "Approval required", `public-cabal-return` labelled "Return unavailable", and `cabal-join-button` reading "Request to join". There is no `cabal-action-fund`. screens.md: a non-member sees the public cabal, a "Membership" card and one call to action |
| S2.3 | A | tap | `cabal-join-button` | | The toast "Request sent. You'll be in once the creator says yes." shows within 10 s, and `cabal-join-requested` reads "Request pending" next to `cabal-join-cancel` within 15 s |
| S2.4 | A | tap, then tap | `push-pre-prompt-not-now` when the push pre-prompt shows, then the navigation bar's back button | | Within 10 s, pulling the list down when the row has not changed yet (the realtime `cabal_access` hint that refreshes it does not reach a local backend), the row for `QA discover {QA.run}` shows `cabals-search-requested` "Request pending" in place of `cabals-search-enter-<id>`, and `cabals-search-pending-notice` reads "Your request to join QA discover {QA.run} is pending." screens.md: while a request is pending, a warn notice sits above the list |

## Ground truth

`apps/mobile/qa/journeys/cabals/discover.truth.sh` reads the run's cabals. After S2, A has one pending request on `QA discover {QA.run}` and is not a member of it, so looking at Discover and the public screen joined nothing.

## Known failures on staging

None known.

## Not covered

- Joining an open cabal from the row or the public screen, and a declined request. [cabals/join](join.md) covers them.
- The private cabal state ("This cabal is private"). The API reports no private cabal, so the state is unreachable. `GroupDetailSampleHarness` covers it.
- The Discover rows for the public cabal board with ranked cabals. The board depends on funded cabals, and S1.2 asserts only the caption.
- The loading skeleton and the full-screen load failure. They need the backend to fail mid-run. `-MonacoDiscoverSample` (`loading`, `error`) covers them.
