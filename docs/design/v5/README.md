# Monaco design v5 · screens

34 screens, dark only, at 2x (786 × 1704 for a phone; the kit board is wider). The system is N3's tokens and native iOS shapes, with N1's layouts where they were picked. Build from these; the rules below are the ones every screen follows.

Lanes build a PR's frame | before | after sheet with `python3 scripts/qa/parity.py sheet --design v5 <frame> --before <png> --after <png>`; `parity.py frames --design v5` lists the frames.

Round 13 added 33 screens in [r13/](r13/README.md); where one of them and a screen here disagree, the r13 screen wins for the part it changes. Use `--design r13` for those frames.

## Screens

| Frame | What it shows | Base |
|---|---|---|
| 00-kit | The kit board: the greens (today, A, B picked, C) on one strip of real UI, the vote card in its four uses, radius set, glass chrome, buttons, chips, cabal tabs, fields, grouped section, scene card, menu, toast, half sheet | M (kit) |
| 02-home-dark | Home: "Welcome, mrrobot" over the balance, the open vote card, the chart, cash with Deposit, your cabals | N1 |
| 02-home-empty | Home, first use: "There is no place like Monaco. Trade with your friends.", Start a cabal and Join with a link, Public cabals | N1 |
| 03-cabals-tab | Cabals tab: the open vote card, the grid of cabals with New cabal, the "Hold a cabal for quick actions." tip, your share in each | N1 |
| 03-cabals-tab-menu | Cabals tab, holding a cabal: the glass quick-actions menu | N3 |
| 03-cabals-tab-few | Cabals tab with two cabals: Up next, the grid with New cabal, the tip, Top cabals this month, your share in each | N1, with N3's Top cabals |
| 03-cabal-chat | A cabal opens on Chat, its first section: messages, reactions, a GIF, the open vote as a message, a fill as the cabal's own moment, the pinned capsule, the composer | new |
| 03-cabal-chat-few | A quiet three-member cabal's chat: its scene, "No vote open.", the pot, starters (Propose a trade, Poll, Share a stock) | new |
| 03-cabal-treasury | Cabal page, Treasury: header, one band for Treasury and Your share, Fund and Propose, the vote card, cash and positions | N1 |
| 03-cabal-members | Cabal page, Members: the same header, member rows with right calls and share | N1 |
| 03-cabal-rules | Cabal page, Rules: the same header, the plain rule rows | N3 |
| 04-proposal-dark | Proposal: who proposed, Buy NVDA $400 with "About 1.82 shares at $220.00" and "2% of treasury", the thesis, the vote, replies, the composer | N3 |
| 04b-hold-2-holding | Home while holding Yes: the capsule fills, "Keep holding · lift to cancel" | N3 |
| 04-reply-composer | Replying to a comment: the half-sheet composer over the proposal | N3 |
| 04-gif-sheet | The GIF sheet over the proposal (GIPHY) | N3 |
| 05-feed-A | Feed: the open vote card in a post, a passed buy, a sale, reactions and reply counts | N3 |
| 05-post | A post and its replies | N3 |
| 05-emoji-picker | The quick reaction row and the full emoji sheet | N3 |
| 06-markets-A | Markets: search, chips, watchlist with sparklines, Pre-IPO | N3 |
| 06-markets-search | Markets search with the keyboard | N3 |
| 07-stock-dark | Stock page: price, chart, Propose buy and Propose sell, in your cabals, market stats | N1 |
| 08-review-sheet | Propose buy sheet: the stock, the cabal, "Share your thesis", Send to vote | N3 |
| 09-fund-cabal | Fund a cabal: amount, keypad, Fund | N3 |
| 09-withdraw | Withdraw: address, network, amount, keypad | N3 |
| 10-profile-own | Your profile: card, positions and month, cash, cabals, Deposit | N3 |
| 10-profile-other | Someone else's profile: card, month and right calls, Follow, activity | N3 |
| 11-share-A | Share a position: the share card, toggles, formats, Share and Copy image | N3 |
| 11-invite-sheet | Invite to a cabal: the cabal, who invited you, the open vote, a note, Share and Copy link | N3 |
| 12-info-sheet | The Market stats sheet over the stock page with N1's Propose buy and Propose sell | mix: N1 buttons, N3 sheet |
| 12-toast | The one toast ("Voted Yes. Passed 5–1.") over Home | N3 |
| 19-permission-sheet | The notifications permission sheet over a passed proposal | N3 |
| 13-leaderboard-C | Leaderboard, Friends: you against the one above, the top three, around you | N3 |
| 15-first-fund | After a first fund: the cabal's scene, "You're in.", your first vote, members, Done | N1 |
| 16-catch-up | Catch up: waiting on you, replies and mentions, invites, results | N3 |

## Greens (option B: green fill acts, green text gains)

- **Gain green `#30F57A`** is for text and lines that went up: every + figure and the chart line. The chart line is the gain green, because a chart going up is a price going up.
- **Forest `#1E5C3A`** is only the fill of the screen's one go action (Deposit, Fund, Start a cabal, Send to vote, Propose buy) and a switch that is on. A second action on the same screen steps down to grey, so Deposit is grey on 02-home-empty, where Start a cabal is the go action.
- **White** is for links, See all, the selected tab and the caret. The old mid green (`#4DAE78`) is not used.
- **A selected chip** is a filter, not an action: white with a black label.
- Loss stays system red. No gradients, no glow. Every pair passes WCAG AA.

## The vote card's one line

One line under the title, nothing else: `AC Capital · 5 of 8 voted · 6 replies · closes in 4h`.

- "closes in 4h" only when the whole line fits; otherwise the bare "4h". On Home and the Cabals tab the cabal name leads, so it is `AC Capital · 5 of 8 voted · 6 replies · 4h`. On the cabal's own pages and in its chat the name drops: `5 of 8 voted · 6 replies · closes in 4h`.
- In the Feed the reply count stays on the post's 💬 chip, so the line is `AC Capital · 5 of 8 voted · 4h`.
- No proposer line, no "N more votes to go", no row of faces, no separate close-time row.
- The full card (logo, title, line, tally bar, capsule Yes and No with live counts) goes wherever you vote. Read-only rows (stock page, invite sheet, first fund, catch up) keep the same text as a row.

## Radius set

| Piece | Radius |
|---|---|
| Card, grouped section, menu | 26 |
| One-line box (stat row, cash row) | 20 |
| Inside a card corner | 14 |
| Half sheet, floating 8 in | 38 top, 47 bottom |
| Buttons, chips, fields, search, keypad press, toast, tab bar | capsule |
| Share card | 32 |
| Cabal crest, stock logo, app tile | 22.5% squircle |

Liquid Glass only on chrome (bar buttons, the tab bar, menus, the toast, the chat's pinned capsule), never on content.
