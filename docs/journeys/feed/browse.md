---
id: feed/browse
title: Browse the feed
version: 1
milestone: M18
requires: [auth/sign-in]
actors: [A]
flows: []
xcuitest: [apps/mobile/MonacoUITests/Journeys/FeedBrowseJourney.swift, apps/mobile/MonacoUITests/Journeys/FeedBrowseJourneyUITests.swift]
---

# Browse the feed

The Feed tab lists what happens across Monaco: cabals started, members joining, proposals, trades and price moves. A member narrows it with chips, switches between everyone and the people they follow, searches it, and taps a cell to open what it is about.

Old app (`c838bd24`): the Feed tab, its chips, the "Everyone" / "Following" toggle, search, and cells that open the proposal. Spec: [Feed tab](../../screens.md#feed-tab).

The format of this doc is in [App journeys](../README.md).

## Preconditions

| Id | What must be true |
| --- | --- |
| P1 | Actors A and B have signed in once (`auth/sign-in`), and their `privy_user_id` values are in `apps/mobile/qa/journeys/accounts.tsv` |
| P2 | The dev database is migrated (`just migrate db`). `scripts/qa/journey.py run` starts the backend with `just run backend` |
| P3 | `apps/mobile/qa/journeys/feed/browse.setup.sh` ran right before the scenario. It seeds the shape of the testkit scenario `feed-two-cabals` through the API with the QA accounts: B creates the open cabal `QA feed {QA.run}` and A joins it, and A creates the open cabal `QA own {QA.run}`. A unfollows B, so A follows nobody the run seeded. The testkit file itself names fixed user ids, so the setup does not replay it |

## Scenarios

### S1 Chips and search narrow the feed

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S1.1 | A | tap | the Feed tab | | `feed-root` shows within 15 s with `monaco-search-field` ("Search the feed"), `feed-scope` and the chips `feed-chip-All`, `feed-chip-Proposals`, `feed-chip-Trades`, `feed-chip-Price moves`, `feed-chip-Cabals` (screens.md Feed tab: "chips "All", "Proposals", "Trades", "Price moves", "Cabals"; an "Everyone" / "Following" toggle; search"; old app: Feed tab chips and toggle) |
| S1.2 | A | tap | `feed-chip-Cabals` | | A cell reads "started QA feed {QA.run}" within 15 s (screens.md: "cells with the actor's avatar (opens `UserProfileRoute`), title, detail, time and comment count"; old app: Feed tab cells) |
| S1.3 | A | type | `monaco-search-field` | `QA feed {QA.run}` | A cell reads "joined QA feed {QA.run}" within 10 s, and no cell reads "started QA own {QA.run}" (screens.md: "search"; old app: Feed tab search) |
| S1.4 | A | replace text | `monaco-search-field` | `nomatch {QA.run}` | `feed-empty` reads "Nothing matches" within 10 s (screens.md: "Empty: "Nothing here yet.""; the app quotes the query) |

### S2 Following shows only people A follows

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S2.1 | A | tap, then tap | the Feed tab, then "Following" in `feed-scope` | | No cell reads "started QA feed {QA.run}" after 5 s (screens.md: "an "Everyone" / "Following" toggle"; old app: Feed tab "Following") |
| S2.2 | A | wait | `feed-empty` | | Reads "Follow people to see what they do." within 10 s (screens.md: "Following with no follows: "Follow people to see what they do." with "Find friends"") |

### S3 A cell opens what it is about

| Step | Actor | Action | Target | Input | Expect |
| --- | --- | --- | --- | --- | --- |
| S3.1 | A | tap, tap, then tap | the Feed tab, `feed-chip-Cabals`, then the cell reading "started QA feed {QA.run}" | | `cabal-header-name` reads `QA feed {QA.run}` within 15 s (old app: a feed cell opens its subject) |
| S3.2 | A | tap back, tap, then tap | `feed-chip-Proposals`, then the first `feed-cell-<id>` | | `proposal-detail` shows within 15 s (old app: Feed cell -> proposal; screens.md Proposal screen `ProposalDetailSlot`) |
| S3.3 | A | press and hold, then tap | the cell reading "started QA feed {QA.run}", then "Hide" | | The cell is gone within 5 s (screens.md: "#702 adds hide and mute") |

## Ground truth

`apps/mobile/qa/journeys/feed/browse.truth.sh` reads `feed_objects` and `follows`. `QA feed {QA.run}` has one `cabal_created` and one `member_joined` row, and `QA own {QA.run}` one `cabal_created` row. Browsing never follows anyone: A has no live follow of B.

## Known failures on staging

- S2.2: the Following scope with no follows shows "Nothing here yet.", not "Follow people to see what they do." with "Find friends". Blocked by #671.
- S3.2: no proposal cell to tap. The setup cannot seed a proposal (creating one needs a live Jupiter quote), and the proposal routes move to the generated client in #612. Blocked by #612.
- S3.3: no hide or mute action on a cell. Blocked by #702.

## Not covered

- Feed cells have no stable per-run identifier: `feed-cell-<id>` carries the server's feed item id, so the steps find a cell by its title text.
- `Hide` and `Mute` have no identifiers yet. #702 adds them in `apps/mobile/Monaco/Features/Feed/FeedItemCell.swift`; this ticket does not touch app code.
- "Find friends" on the empty Following scope (#671) and the actor avatar opening `UserProfileRoute`, which `profile/overview` covers.
- The Trades and Price moves chips: no trade or price move can be seeded without a swap or a price feed.
