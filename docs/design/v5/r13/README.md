# Monaco design v5 · round 13 screens

33 screens, dark only, at 2x (786 × 1706), drawn in the v5 system (`docs/design/v5/README.md` and `docs/design/v5/screens/`; every v5 rule holds). They answer an outside UI/UX review of the v5 app, with the options mrrobot picked on 2026-11-05. Build from these; where a screen here and a v5 screen disagree, this one wins for the part it changes.

PRs build a frame | before | after sheet with `python3 scripts/qa/parity.py sheet --design r13 <frame> --before <png> --after <png>`; `parity.py frames --design r13` lists the frames.

## What was picked

| Item | Pick |
|---|---|
| The open vote in Chat | The pin capsule at the top carries the open vote (not a dock over the composer) |
| Reply threads | A proposal's replies are one list: they show in the proposal's Replies, and in Chat they are a thread under the proposal message. Thread replies stay out of the main chat unless "Also send to chat" is on (off by default) |
| The thesis | An optional "Why NVDA?" box at the top of the review sheet (no extra step). Pre-IPO proposals ask too |
| A failed vote | Leaves a line in Chat ("Failed 2 to 4 · nothing bought"), as a passed vote does |
| Home | The total's two parts as rows under it: your share in cabals, and cash with Deposit |
| Onboarding | Four cards you swipe through, built on the world scenes, with motion; every step skippable; once per account, only for new accounts with no cabal (invited people only if new to the app) |
| Words | "Pre-IPO estimate" everywhere (not "Secondary estimate"); "Tokenized stock ⓘ" on the stock page with a plain explanation; "Ask" to join a cabal that approves members |
| Price | No second (Nasdaq) price; one line says Monaco's price can differ from Nasdaq's |
| Invite link | A deep link: opens the app on the cabal's invite page; without the app, the App Store, and the invite still opens after install |

## Screens

| Frame | What it shows |
|---|---|
| r13-2a-A1-pinned | Chat scrolled past the vote: the pin capsule carries the open vote (`Buy NVDA · $400`, `5 of 8 voted · 6 replies · 4h`, Vote, chevron). It switches to the vote only once the card has left the screen |
| r13-2a-A2-pins-sheet | The chevron opens Pinned at the medium detent: Open votes (each the shared vote card you can hold), then pinned Messages |
| r13-2a-A3-closed | A vote passed: the result lands once in the thread as the cabal's moment row; the capsule moves on to the next open vote |
| r13-2b-1-chat | A proposal in Chat is one message: the thesis as the proposer's bubble, the card, and a thread line (last three repliers' faces, reply count, time, ›) |
| r13-2b-2-thread | The pushed Thread screen: the proposer, the thesis and the card at the head (you can vote here), then the replies; the composer with "Also send to chat" |
| r13-2c-B1-sheet | The review sheet with an optional "Why NVDA?" box first, then the trade and the cabal, Send to vote |
| r13-2c-B2-typing | Typing the thesis: the sheet rides the keyboard, the trade folds to one line, Send to vote above the keyboard |
| r13-2c-feed | The Feed's "Mia proposed" post with the thesis between the header and the card (up to three lines) |
| r13-2c-proposal | The proposal page with the thesis under the facts line as the proposer's words |
| 3B-home | Home: under the total, two rows: "$11,582.00 · Your share in 3 cabals ›" and "$500.00 · Cash · USDC" with Deposit; the lone Cash row under the chart goes |
| 3B-home-scrolled | Your cabals' header shows the same $11,582.00, and its rows add up to it |
| 3-profile-own | Profile: "Positions" becomes "In cabals"; the cash row reads like Home's; rows say "Your 35% · admin" |
| 3-cabals-tab-share | Cabals tab: "Your share in each $11,582.00" |
| 5-home-empty | Empty Home: Start a cabal and Join with a link, public cabals you can join (Join, or Ask), Find your friends |
| 5-feed-empty | Empty Feed: one plain line, People to follow, Public cabals, From everyone |
| 5-cabals-empty | Empty Cabals tab: New cabal and Join with a link tiles, Public cabals, Find your friends |
| 5-public-cabals | All public cabals: search, sort chips, rows with Join or Ask |
| 5-find-people | The find-people half sheet: live results with Follow, and Invite a friend |
| 6-stock | Stock page: "Tokenized stock ⓘ" under the name opens About this price; Market stats has "About" |
| 6-price-sheet | About this price: what a tokenized stock is, why Monaco's price can differ from Nasdaq's, and the source line from the asset's own fields |
| 6-pre-ipo | Pre-IPO page: "Pre-IPO estimate" with its plain line, always visible |
| 6-propose-sheet | The propose sheet: "At Monaco's price ⓘ" opens the same sheet |
| 8-invite-landing | The invite page: the scene, the cabal, who invited you, the open vote (read-only), Join docked as the one filled button |
| 8-invite-landing-loading | Before the network: the scene draws from the link, the text shimmers, Join is docked but disabled |
| ob-A1-cabal | Onboarding 1: "A cabal is a few friends and one pot." on a world-scene postcard, the sample cabal's faces and pot |
| ob-A2-vote | Onboarding 2: "Anyone can put up a trade." "This cabal needs 4 of 6 to say yes." A practice vote card |
| ob-A2-vote-holding | Onboarding 2 while holding |
| ob-A2-vote-passed | Onboarding 2 after Yes: it passes |
| ob-A2-vote-open | Onboarding 2 after No: still open |
| ob-A3-share | Onboarding 3: "Your share moves with the pot. Cash it out any time." with a practice Cash out |
| ob-A3-cashout | Onboarding 3 after the practice Cash out: nothing moves |
| ob-A4-start | Onboarding 4: Start a cabal, Join with a link, Look at public cabals |
| ob-A4-invited | Onboarding 4 for someone who came from an invite link: Join the cabal, or Look around first |
