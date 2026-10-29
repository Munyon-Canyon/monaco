---
id: feed/browse
title: Browse the feed
version: 8
milestone: M18
requires: [auth/sign-in]
actors: [A]
flows: []
xcuitest: [apps/mobile/MonacoUITests/Journeys/FeedBrowseJourney.swift, apps/mobile/MonacoUITests/Journeys/FeedBrowseJourneyUITests.swift]
---

# Browse the feed

The Feed tab lists what happens across Monaco as event cards: cabals started, members joining, proposals, trades, price moves, replies and approvals. A member narrows it with the filter menu, which switches between everyone and the people they follow in the same menu, and taps a card to open what it is about.

Old app (`c838bd24`): the Feed tab, its single-choice chips, the "Everyone" / "Following" toggle, search, and cells that open the proposal. Spec: [Feed tab](../../screens.md#feed-tab).

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actors A and B have signed in once (`auth/sign-in`), and their `privy_user_id` values are in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend` |
| P3 | `apps/mobile/qa/journeys/feed/browse.setup.sh` ran right before the scenario. S1 seeds the shape of the testkit scenario `feed-two-cabals` through the API with the QA accounts: B creates the cabal `QA feed {QA.run}` and A joins it, and A creates the cabal `QA own {QA.run}`. S1 and S2 take back every follow of A's, so A follows nobody at all. S3 writes one open buy proposal by B in `QA feed {QA.run}` with the thesis `QA proposal {QA.run}`, the way `governance/vote` seeds one, because creating one through the API needs a live Jupiter quote. The testkit file itself names fixed user ids, so the setup does not replay it |

## Scenarios

### S1 The filter menu scopes the feed

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap, then tap | the Feed tab, then `feed-filter` | | `feed-root` shows within 15 s, and the open filter menu offers `feed-scope-Your cabals and friends` (checked), `feed-scope-Your cabals`, `feed-scope-Friends`, `feed-scope-Everyone` and `feed-muted-link` (screens.md Feed tab: the filter menu) |
| S1.2 | A | tap outside the menu | the left margin beside the menu | | A card reads "started QA feed {QA.run}" within 15 s (screens.md: a cabal block reads the item's title) |
| S1.3 | A | look | `monaco-search-field`, `feed-bell` | | Neither exists: the Feed has no search and no bell (Q40) |
| S1.4 | A | tap, tap, then tap outside the menu | `feed-filter`, `feed-scope-Your cabals`, then the left margin beside the menu | | A card reads "started QA own {QA.run}" within 15 s |

### S2 Friends shows only people A follows

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | tap, tap, tap, then tap outside the menu | the Feed tab, `feed-filter`, `feed-scope-Friends`, then the left margin beside the menu | | No card reads "started QA feed {QA.run}" after 5 s (screens.md: the filter menu's "From" picks; old app: Feed tab "Following") |
| S2.2 | A | wait | the text "Follow people to see what they do.", then `feed-find-friends` | | The message shows within 10 s with `feed-find-friends` reading "Find friends" under it (screens.md: "Following with no follows: "Follow people to see what they do." with "Find friends"") |

### S3 A cell opens what it is about

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S3.1 | A | tap, pick `feed-scope-Your cabals and friends` from `feed-filter` (closing the menu), then tap | the Feed tab, then the card reading "started QA feed {QA.run}" | | `feed-item-detail` shows within 15 s, the post page, not the cabal page (screens.md Post screen: the Feed card opens its post; the cabal opens from the card's context menu, "Open <cabal>") |
| S3.2 | A | tap back, then tap | the first `feed-cell-<id>` | | `proposal-vote-yes` shows within 15 s, and the screen reads `QA proposal {QA.run}` within 10 s (screens.md Proposal screen) |
| S3.3 | A | tap back, then press and hold, then tap | the card reading "started QA feed {QA.run}", then "Hide this post" | | The card is gone within 5 s (Q40: a long-press hides) |

## Ground truth

`apps/mobile/qa/journeys/feed/browse.truth.sh` reads `feed_objects` and `follows`. `QA feed {QA.run}` has one `cabal_created` and one `member_joined` row, and `QA own {QA.run}` one `cabal_created` row. Browsing never follows anyone: A has no live follow of B.

## Known failures on staging

None.

## Not covered

- Feed search: F draws none (Q40), so there is no "Search the feed" field and no "No matching updates" step. S1.3 asserts it is absent.
- The filter menu stays open after each pick, so each step taps the left margin beside the menu to close it. Scope options are picked by `feed-scope-<title>`.
- Feed cards have no stable per-run identifier: `feed-cell-<id>` carries the server's feed item id, so the steps find a card by its title text. A cabal card's block reads the item's title to VoiceOver.
- "Hide this post" and the mute items are context-menu items with no identifiers, found by label.
- "Find friends" on the empty Following scope (#671) and the author avatar opening `UserProfileRoute`, which `profile/overview` covers.
- Kind options (Proposals, Completed trades, Price moves, Replies & votes, Proposal outcomes, Cabal activity): not in the filter menu yet, and no trade, price move, reply or approval can be seeded without a swap, a price feed or a live proposal.
