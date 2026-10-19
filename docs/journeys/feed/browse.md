---
id: feed/browse
title: Browse the feed
version: 5
milestone: M18
requires: [auth/sign-in]
actors: [A]
flows: []
xcuitest: [apps/mobile/MonacoUITests/Journeys/FeedBrowseJourney.swift, apps/mobile/MonacoUITests/Journeys/FeedBrowseJourneyUITests.swift]
---

# Browse the feed

The Feed tab lists what happens across Monaco as event cards: cabals started, members joining, proposals, trades, price moves, replies and approvals. A member narrows it with the filter menu's kind toggles, any number at once, switches between everyone and the people they follow in the same menu, searches it, and taps a card to open what it is about.

Old app (`c838bd24`): the Feed tab, its single-choice chips, the "Everyone" / "Following" toggle, search, and cells that open the proposal. Spec: [Feed tab](../../screens.md#feed-tab).

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actors A and B have signed in once (`auth/sign-in`), and their `privy_user_id` values are in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend` |
| P3 | `apps/mobile/qa/journeys/feed/browse.setup.sh` ran right before the scenario. It seeds the shape of the testkit scenario `feed-two-cabals` through the API with the QA accounts: B creates the cabal `QA feed {QA.run}` and A joins it, and A creates the cabal `QA own {QA.run}`. A unfollows B, so A follows nobody the run seeded. The testkit file itself names fixed user ids, so the setup does not replay it |

## Scenarios

### S1 Kind toggles and search narrow the feed

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap, then tap | the Feed tab, then `feed-filter` | | `feed-root` shows within 15 s with `monaco-search-field` ("Search the feed"), and the open filter menu offers `feed-scope-Everyone`, `feed-scope-My cabals`, `feed-scope-Following`, `feed-chip-Proposals`, `feed-chip-Completed trades`, `feed-chip-Price moves`, `feed-chip-Replies & votes`, `feed-chip-Proposal outcomes`, `feed-chip-Cabal activity` and `feed-muted-link`, and no `feed-chip-All` (screens.md Feed tab: the filter menu; old app: Feed tab chips and toggle) |
| S1.2 | A | tap, tap, then tap outside the menu | `feed-filter`, `feed-chip-Cabal activity`, then the screen below the menu | | A card reads "started QA feed {QA.run}" within 15 s (screens.md: a cabal block reads the item's title; old app: Feed tab cells) |
| S1.3 | A | type | `monaco-search-field` | `QA feed {QA.run}` | A card reads "joined QA feed {QA.run}" within 10 s, and no card reads "started QA own {QA.run}" (screens.md: "search"; old app: Feed tab search) |
| S1.4 | A | replace text | `monaco-search-field` | `nomatch {QA.run}` | `feed-empty` reads "No matching updates" within 10 s (screens.md: "No matching updates" with a search or filter) |

### S2 Following shows only people A follows

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | tap, tap, tap, then tap outside the menu | the Feed tab, `feed-filter`, `feed-scope-Following`, then the screen below the menu | | No card reads "started QA feed {QA.run}" after 5 s (screens.md: the filter menu's "From" picks; old app: Feed tab "Following") |
| S2.2 | A | wait | `feed-empty` | | Reads "Follow people to see what they do." within 10 s (screens.md: "Following with no follows: "Follow people to see what they do." with "Find friends"") |

### S3 A cell opens what it is about

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S3.1 | A | tap, pick `feed-scope-Everyone` from `feed-filter` (closing the menu), then tap | the Feed tab, then the card reading "started QA feed {QA.run}" | | `cabal-header-name` reads `QA feed {QA.run}` within 15 s (old app: a feed cell opens its subject) |
| S3.2 | A | tap back, switch `feed-chip-Cabal activity` off and `feed-chip-Proposals` on in `feed-filter`, then tap | the first `feed-cell-<id>` | | `proposal-detail` shows within 15 s (old app: Feed cell -> proposal; screens.md Proposal screen `ProposalDetailSlot`) |
| S3.3 | A | tap back, switch `feed-chip-Proposals` off and `feed-chip-Cabal activity` on in `feed-filter`, then press and hold, then tap | the card reading "started QA feed {QA.run}", then "Hide this post" | | The card is gone within 5 s (screens.md: a long-press mutes or hides) |

## Ground truth

`apps/mobile/qa/journeys/feed/browse.truth.sh` reads `feed_objects` and `follows`. `QA feed {QA.run}` has one `cabal_created` and one `member_joined` row, and `QA own {QA.run}` one `cabal_created` row. Browsing never follows anyone: A has no live follow of B.

## Known failures on staging

- S2.2: the Following scope with no follows shows "Nothing here yet.", not "Follow people to see what they do." with "Find friends". Blocked by #671.
- S3.2: no proposal cell to tap. The setup cannot seed a proposal (creating one needs a live Jupiter quote), and the proposal routes move to the generated client in #612. Blocked by #612.

## Not covered

- The filter menu stays open after each pick (every pick applies at once), so each step taps the screen below the menu to close it before it goes on. The options are picked by their `feed-scope-<title>` and `feed-chip-<title>` identifiers. The kind options are toggles that stay on until tapped again, so S3 relies on S1.2 having left `feed-chip-Cabal activity` on and on S2 having left only the scope changed; an empty set shows everything.
- Feed cards have no stable per-run identifier: `feed-cell-<id>` carries the server's feed item id, so the steps find a card by its title text. A cabal card's block reads the item's title to VoiceOver.
- "Hide this post" and the mute items are context-menu items with no identifiers, found by label.
- "Find friends" on the empty Following scope (#671) and the author avatar opening `UserProfileRoute`, which `profile/overview` covers.
- The Completed trades, Price moves, Replies & votes and Proposal outcomes options: no trade, price move, reply or approval can be seeded without a swap, a price feed or a live proposal. `feed-chip-Replies & votes` sends `reply` alone, because votes have no feed kind.
