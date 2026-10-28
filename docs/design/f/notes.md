# F design notes (round 11, the final set)

What an engineer needs from the designer's notes. The frames are in `screens/`, the kit in [kit.md](kit.md), the gallery in `index.html`. The designer's fix logs, check sources and offsets are left out. Frames named below that are in the designer's "Alternatives not chosen" section (`05-feed-B`, `06-markets-A2`, `13-leaderboard-A`, the Cabals tab iterations and the rest) are not in this package.

## Decisions

### The 10-26 decisions, and what changed

| # | Decision | What changed in F | Frames |
|---|---|---|---|
| 1 | Leaderboard at launch ranks right calls over a plain period, no season | The board's header is "This month · Oct 1 – 31" with **Week · Month · All time** as plain labels (Month on), in the kit's `.lb-date` with a new `.periods`. The Season 3 menu at the top right is gone (an i for how it is ranked takes its place). No "Season" on any launch frame. | `13-leaderboard-C-people`, `-C-cabals`, `-C`, `-C-scrolled`, `-C-unranked`, `15-leaderboard-moment`, Home's leaderboard row |
| 2 | The score is the right-call rate with its record, one figure per row; five resolved votes to rank; turnout is the sub-line | Every ranked row trails **"91%" over "10 of 11 right"** (the kit's value-over-detail trail, one fact); the sub-line is turnout ("Voted 12 of 12"). Below five resolved votes a row has no rank and no rate and says "2 more · to rank" (`.rank-row.unranked`). The same figures run through the Members tab ("Admin · 7 of 9 right · voted 11 of 11"), Christelle's profile ("Right calls · 1M 12 of 13") and Home ("Christelle leads · 92%", your 78%). | the boards, `03-cabal-members*`, `10-profile-other*`, `02-home-*` |
| 3 | Season and streak pieces leave the launch screens | Removed from launch: Home's streak line under the vote card, the cabal page's streak row, the season row on the Cabals tab and both profiles, the Follow control. The season sheet, season close, streak moment and season story move whole to Later. Section 3 below lists every redraw. | see §3 |
| 4 | Home's figure at 36, with a 32 / 36 / 40 set to compare; **the founder chose 32** (10-26 ~11:55 PM) | Kit token `--t-home-figure` is **32 / 36 semibold** on `.home-hd.brand .amt` (fix 1; it was 36); `.f36` and `.f40` draw the two sizes not chosen on the same element. Every Home frame uses 32; the keypad keeps the 44 money hero. `02-home-figure-32` went (it was Home itself); `-36` and `-40` are in Alternatives not chosen. | `02-home-*`, `04b-hold-*`, `12-toast`, `14-hold-sheet`, `14-wrap-sheet`, `01-tokens-type`, `02-home-figure-36/40` |
| 5 | Reactions are any emoji; GIF replies | The smiley-plus chip opens a quick row of six (👍 ❤️ 😂 🎉 👀 💪) and +; + opens the full picker (search, Frequently used, Smileys and people, the category bar) as a half sheet. Chips show whatever emoji was used (Feed A now carries a 🫡). The composer gets a **GIF** button inside its field; it opens a half-height GIF sheet (Trending GIFs, search, "Powered by GIPHY" in the header); a GIF reply is a bubble with the GIF in the reply's place. | `05-reactions`, `05-emoji-picker`, `05-feed-A*`, `04-gif-sheet`, `04-gif-reply`, every composer |
| 6 | Who voted which way: private by default, a Rules switch shows the sides after close | Rules gains "Show who voted what · After a vote closes", **off** by default; the footer says counts are live for every member and that the switch changes by proposal like every rule. With it on (`03-cabal-rules-sides`), the closed proposal shows D's reveal row under the settled tally: ✓ Mia, Christelle, Logan, Sam, you · ✕ Kai · – Jordan and Priya did not vote (`04-proposal-sides`; fix 1: the vote settled on Andres's fifth yes, so Jordan never voted). `04-proposal-passed` stays the default (counts only). | `03-cabal-rules*`, `04-proposal-sides`, `01-tokens-controls` |
| 7 | Markets A in both orders, each with its scrolled frame; **the founder chose F's order** (10-26 ~11:55 PM) | F's order (Watchlist · Pre-IPO · Moving today) is Launch: `06-markets-A` and `-A-scrolled`. The director's (Watchlist · Moving today · Hot in your cabals · Pre-IPO as dense rows with valuation and round) is in Alternatives not chosen with its end frame. | `06-markets-A`, `-A-scrolled` (Launch), `-A2`, `-A2-scrolled` (Alternatives) |
| 8 | Three iterations of the Cabals tab grid, each with its menu; **the founder chose the existing tab** ("i like this one better i think is the old design") | `03-cabals-tab` and its menu stay in Launch as they were. v1 (the icons carry their numbers), v2 (the grid is the launcher, the list is what happened) and v3 (tiles like small widgets), each with its long-press menu, are in Alternatives not chosen. | `03-cabals-tab`, `-menu` (Launch), `03-cabals-tab-v1` … `-v3-menu` (Alternatives) |
| 9 | Weekly ritual: F's push, A's Sunday Ledger as the sheet, the Story share; no streak or season | The push is Sunday's "The Ledger · Week 42" (launch Lock Screen and Notification Center); it opens A's Ledger as a half sheet (+1.4% on your stake, Voted · Right calls · Sharpest, the best call, Full Ledger, Share the Ledger); Share opens the Ledger's Story card (cover band, your picture, the week's figure and line, Best call · Sharpest · Voted, the code). | `14-wrap-sheet`, `14-ledger-story`, `14-lock-screen-launch`, `14-lock-screen` |
| 10 | The proposal's facts: D's stat row under the hero, above the thesis | Open: **Price (i) · Shares ≈ · Of treasury · Closes** ($220.00 · ≈ 1.82 · 2% · 1:53 PM). Closed: the result's four (Fill · Shares · Now · Since). Pre-IPO: Estimate (i) · Shares ≈ 2.70 · 2% · Fri 5:41 AM. Failed: Price · 2.2 · 4% · Closed Oct 18. The hero's sub-line keeps the company name; its price and share of treasury moved into the row (one place for each fact). | every `04-*`, `19-permission-sheet`, `15-passed` (behind) |
| 11 | Home keeps six blocks; the markets variant is not chosen | Home unchanged in structure; `02-home-variant-markets` is in section 3, Alternatives not chosen. | `02-home-*` |
| 12 | Versions: Feed A, Markets A, Leaderboard C on every tab; Share A for a position, Share B for a passed proposal | Leaderboard C drawn for People and Cabals (Friends existed). Share B redrawn: Christelle's picture with Degens' crest and "Christelle called it" as the header, two markers (the seal where the vote passed at $709.80, B where it filled at $716.52). Feed B and C, Markets B and C, Leaderboard A and B (with A's light twin) and the Home markets variant are in section 4 as round 10 drew them. | `11-share-B`, `13-leaderboard-C-*`, section 4 |
| 13 | Round-9 director defaults are the founder's (18, 20, 22, 24, 25, 28, 30) | Already drawn; checked: status as words on share cards, flat podium (on the not-chosen A), short cabal name everywhere but the page title (applied in the build), share screen docked, comments run on under the composer, chips as capsules, no greeting. | all |
| 14 | Yes / No on notifications at launch (open the hold sheet); no rivalry; settle at quorum | `14-notification`, `14-faceid-alt`, `15-last-vote` and `14-hold-sheet` are launch frames. Nothing in F draws rivalry, challenges or trophies. **Fix 1 carries settle-at-quorum through the NVDA story**: Andres's Yes is the fifth of eight, so the vote passes 5–1 on it at 9:41 AM and fills the same minute; every committed, toast, Catch up, permission and result frame says so (see Fixed after check 1). | `14-notification`, `14-hold-sheet`, `04-vote-committed`, `04b-hold-3-committed`, `12-toast`, `04-proposal-passed`, `-sides`, `15-passed` |

### Founder answers, 10-27

- **Undo:** an ordinary vote's toast has Undo ("Voted Yes. 6th of 8. Undo"); the vote that settles the proposal has none ("Voted Yes. Passed 5–1.").
- **Feed comments:** every Feed post can be commented on from the Feed, with text, emoji reactions and GIF replies. A proposal's or a result's thread is its cabal's discussion: members reply, anyone else who can see it reads and reacts. Every other post takes replies from anyone who can see it. A Feed post that quotes a reply from a proposal's discussion opens the proposal at that reply. A reply to your post, or to your reply, reaches you in Catch up and as a push grouped with that cabal or person.
- **10-27 evening:** round 11 stays the design. No image uploads in chats at launch: replies take text, emoji reactions, GIFs (the GIPHY iOS SDK, rating pg-13), Poll, Quote the thesis and Share a stock; the composer's plus menu has no Photo. Profile and cabal pictures are not chats and stay.

## Feed sizes (10-27 evening, final)

The founder's notes: the reaction buttons are too big, and the logo and ticker could be bigger. The final numbers, in phone points of the 393 × 852 frames:

- **Reaction chip on a post:** 24 visible, 44 × 44 to tap (its own 44 width, 10 above and below the 24). At least 44 wide, padding 8, 4 between chips. Emoji, count, + and the reply glyph are Footnote semibold 13 in the frame; in the app they are 15, under the 15 pt floor (founder decision Q1, listed in the tracking issue).
- **A reply's chip** stays 22 visible, 44 × 44 to tap.
- **Feed post logo:** 36, radius 15 % of its size (5.4).
- **Ticker and amount line:** Headline 17 semibold (`.font(.headline)`) when a logo leads the line. The cabal and the fact keep Footnote 13. Lines with no logo (Nora joined, "Tech Titans") keep 15.
- **No post gains a line and nothing truncates.** The longest line, "OpenAI $200 Quant Kids Pre-IPO", fits the 377 pt margin.
- **`05-post-reply`:** the quoted post takes two lines inside the quote: "Logan sold", then "AAPL $480 · Tech Titans · +6.7%".
- **`04-composer-menu`:** lists Poll, Quote the thesis and Share a stock, with no separator and no Photo.
- **No image uploads in chats.** The plus menu has no Photo and no reply takes a picture. What remains is not a chat and stays: profile and cabal pictures, the GIF stills (GIPHY, not uploads), and "Copy image" on the share sheets (it exports Monaco's card).

## Feed comments (10-27)

The founder asked on 10-27: "in the feed can we comment or add gifs or only reactions? we should be able". A Feed card had reactions and a reply count only; replies and GIF replies lived on the proposal page, and a sale, a join, a right call or a quote could not be commented on. Now every Feed post takes text, any-emoji reactions and GIF replies from the Feed itself, under the rules in "Founder answers, 10-27".

**Rules the frames draw**
- A reply's reactions are emoji chips, as a post's are: one small chip per emoji used (the emoji and its count), then Reply. An emoji with no count shows nothing, so a reply nobody reacted to shows Reply alone. A chip toggles your reaction, a double tap adds ❤️, and a long press opens the quick six and + (`05-comment-actions`).
- GIF sits inside the field of both reply sheets (`04-reply-composer`, `05-post-reply`): one component, `Comments/CommentComposer`.
- Every chip, quick-row emoji, GIF button and text Reply is 44 × 44 to tap in both dimensions; the visible label does not change.
- A public proposal's replies keep their chips and take no Reply.
- Each post's reaction row ends with the reply control (SF `bubble.left` and the count), 44 to tap. It shows no count when there are none. The count appears once on a card, on the control; the status line keeps turnout and deadline.
- Home's From your cabals rows keep two lines and end the sub-line in the reply glyph and count. The whole row opens the thread; there is no trailing chip. Catch up stays as drawn; Sam's reply arrives as a push in `19-notifications`.

**Frames**

| Frame | What it shows |
|---|---|
| `05-post` (new) | The page for any post that is not a proposal or a result: an inline "Post" bar with the back chevron and share, then Logan's AAPL sale as Feed A draws it (36 logo, Headline ticker line, 24 chips, reply control 5), then Replies 5, rooted as the proposal draws them: Nora (4h, "Why sell here?") with View 1 reply (Logan's answer, folded), your reply, Christelle's GIF reply whose quote jumps to Logan's answer, and Sam's reply to you. The composer is docked with ⊕ and GIF, and replies run on under it |
| `05-post-scrolled` (new) | The same page at its real end: Replies 5 at the bar, Sam's reply 40 above the composer. The list's end inset is `--composer-clear` + 40, the same rule as the proposal thread |
| `05-post-reply` (new) | Replying from the Feed. Over Feed A the composer opens as a half sheet over the keyboard: "Reply to Logan", the post quoted on two lines, the text with GIF inside the field, and Send |
| `05-comment-actions` (new) | Long-press on a reply. The page blurs and Nora's reply lifts. Above it the quick six and +; below it Reply, Copy, Report (Report in the destructive colour). On your own reply, Delete replaces Report. The frame draws someone else's reply |
| `05-post-public` (new) | A proposal from a cabal you are not in (Maya's Buy AMZN $250 in Pilates): the proposal's crest line, hero, stat row and thesis, and the live tally ✓3 ✕0 with "3 of 7 voted · 2 replies · ends in 23h". No vote buttons and no Remind; the tally's middle says "Pilates members vote". Replies are readable and reactions open. In the composer's place one line: Pilates' crest and "Members of Pilates reply here" |
| `05-feed-A`, `05-feed-A-scrolled`, `05-feed-light`, `05-reactions`, `05-emoji-picker` | Every post's reaction row ends with the reply control. Degens' "5 replies" text becomes the control. Mia's status line drops "6 replies" and keeps "5 of 8 voted · ends in 4h". On `05-reactions` the control follows the chips while + is open as the quick row |
| `05-feed-empty` | The public posts gain a reaction row with + and the control (Maya 2, Rosa 4, Ivan and Sam none), and their status lines drop the count |
| `02-home-*` | The From your cabals rows end the sub-line in "· 💬 n" (NVDA 6, the AAPL sale 5, Christelle called it 5, Nora joined 3, Sell TSLA failed 4). `02-home-empty`'s From everyone rows do the same |
| `01-tokens-controls` | The reactions specimen ends with the reply control |
| `19-notifications` | A new push at the top: "Sam replied to you · now · “He called the top again.” On Logan's AAPL sale." It is grouped with Sam, a group of one with no header, and opens `05-post` at his reply |
| `16-catch-up` | Unchanged. "Mia replied to you" is already the reply-to-you pattern, and the bell stays at 6 |
| `04-proposal-*`, `04-vote-*`, `04-reply-composer`, `04-gif-*` | Each reply's heart becomes its emoji chips; Christelle's GIF reply on Logan's sale carries 😂 3 and ❤️ 1. `04-reply-composer` carries GIF in its field |

**Routes**
- The reply control on a proposal or a result (Mia's Buy NVDA, Degens' passed META, Tech Titans' failed TSLA) opens `ProposalScreen` scrolled to Replies with the composer focused. The docked composer grows into the half sheet as `04-reply-composer` draws it, with no quote, so it replies to the proposal itself.
- Logan's quote on Sell TSLA is a reply from that vote's thread. Its control opens the same proposal page at his reply, with the sheet quoting him (`04-reply-composer`'s "Reply to Logan").
- Every other post opens `05-post`. Its control opens the page with the composer focused, and from the Feed it opens the half sheet in place (`05-post-reply`).

Nothing is removed: the proposal page's own status line keeps its "6 replies", because it is the vote card on the page that holds the replies.

## Launch versus later

**Phase 2 frames (section 2 since fix 1, kept whole and polished, built with `LATER` on):** `01-tokens-later` (new: Follow and Following, the streak row's three states, the season row, Home's streak footer, the rows that left `01-tokens-controls`), `04-proposal-follow` (new: the proposal with Remind and Follow, the control the launch proposal leaves out), `14-live-activity-compact`, `-expanded`, `-lock`, `14-dynamic-island`, `14-lock-screen`, `14-widgets`, `14-standby`, `14-spotlight`, `10-season-sheet`, `15-season-close`, `15-streak`, `17-season-story-1`, `17-season-story-card`.

**Launch frames redrawn without a phase-2 piece** (the phase split is the reason for each removal; where something takes the space, its own reason follows):

| Frame | Removed | In its place, with reason |
|---|---|---|
| `02-home-dark`, `-light`, `-scrolled`, `02-home-figure-*`, `04b-hold-1/2/3`, `12-toast`, `14-hold-sheet` (behind), `14-wrap-sheet` (behind) | The streak line under Home's vote card ("11 votes in a row · this one makes 12") | Nothing: the card ends at its tally and the chart follows it |
| `02-home-*` leaderboard row | "You are #8 · 1 call behind Kai", the season score 82 | The month's figures ("Christelle leads · 92%", "This month · you are #8 of 24", 78%): decisions 1 and 2 |
| `03-cabals-tab`, `-menu` | The season row at the end | Nothing |
| `03-cabal-treasury*`, `-rules*`, `-members*` | The streak row under the header ("You 11 · Cabal 4 · 1 grace left") | Nothing; the open proposal sits 16 under the buttons. Each tab's real maximum scroll was re-measured: Treasury gains a **Treasury history** row (the history behind the figure's chevron gets a row where the treasury's facts are; it also lets the tab end on a boundary at both edges), Members a fourth footer line (what a right call and turnout mean, now that the record is right calls), Rules a three-line footer (it names the new switch) |
| `04-proposal-dark`, `-light`, `04-vote-*`, `04-proposal-preipo`, `04-reply-composer`, `04-gif-*`, `04-composer-menu`, `19-permission-sheet` | Follow (it starts a Live Activity) | Remind me at close stays alone, as theme 8 drew it; after you vote it leaves too (the result push goes to voters, so there is nothing left to remind you of) |
| `10-profile-own`, `-own-scrolled`, `11-invite-sheet` (behind) | The season row under the followers line | Nothing; the cash row moves up. The scrolled frame's offset is re-measured (32: the card whole under the bar, the third cabal ends on the Deposit shelf; fix 1: 90; fix 2: 96, the picture and its pencil ring under the bar, the bio 8 under it, the invitation 6 above the shelf) |
| `10-profile-other`, `-other-cabals` | The season row ("Season 3 · 14 of 14 votes · 14") | Nothing; "Right calls" becomes "Right calls · 1M 12 of 13" (decision 2) and "Took the crown · Season 2 close" becomes "Topped the board · September" (the month board's #1 has the crown) |
| `13-leaderboard-C`, `-C-scrolled` | "Season 3 · ends Dec 31", the Season 3 menu, the season score | The period header and the rate (decisions 1 and 2) |
| `15-leaderboard-moment` | Leaderboard A's podium with the season header | Leaderboard C's version of the moment (decision 12 makes C the board) |
| `18-invite-landing` | "Season 3" in the identity line and the "Season 3 +4.1%" stat | "since Aug 2026" and the treasury's 1M (−4.1%, the figure the cabal page shows) |
| `14-wrap-sheet` | The streak stat ("Streak 10 votes") | A's Ledger (decision 9) |
| `01-tokens-controls` | Remind · Follow, the streak row, the season row | Remind alone; the phase-2 rows moved whole to `01-tokens-later`; the launch board gains the period labels, the sides row, the proposal stat row, the ranked row, reactions and the composer |
| `14-lock-screen` (both) | Accessory widgets and the Live Activity | `14-lock-screen-launch` is the launch version, notifications only (AC Capital's vote reminder and Sunday's Ledger); `14-lock-screen` stays whole in Later |

## Fixture table

Sample data only. The app shows real data; these values are not product rules.

Clock: **Thursday, October 22, 2026, 9:41 AM** (the status bar's 9:41 is the fixture time). Oct 12 Mon, Oct 17 Sat, Oct 18 Sun, Oct 19 Mon. Buy NVDA settles at quorum on Andres's fifth yes at **9:41 AM** and fills the same minute (fix 1); the closed NVDA frames (`04-proposal-passed`, `04-proposal-sides`, `15-passed`, `14-dynamic-island`) show it later that day (Now $220.00). This month = Oct 1 – 31; this week = Oct 19 – 25; Week 42 = Oct 12 – 18. Phase-2 frames keep Season 3 (Oct 1 – Dec 31, 70 days left) and Season 2 (closed Sep 30).

| Fact | Value |
|---|---|
| **Andres** | Net worth **$12,082.00** = $7,000.00 + $2,710.00 + $1,872.00 stakes + $500.00 cash; Positions $11,582.00; 1M P&L +$241.10 (2.0%); 12 followers, 8 following, 3 cabals + Quant Kids invitation (counted as "3 · 1 invited"). Home figure at **32** (founder's choice; the not-chosen 36 and 40 in Alternatives). Profile's standing line: 78% right calls this month · 7 of 9 · #8 of 24. Bell badge **6** (the Catch up count); none on the first-use account. |
| **AC Capital** (AC Capital Management only as its page title) | 8 members, all eight vote, 5 of 8 to pass, 6 h window, max 20% of cash per trade, since Aug 2026, admins Andres and Christelle. Treasury **$20,000.00**, −4.1% · 1M = Cash $5,100.00 (3.5% APY) + NVIDIA $8,360.00 + Apple $4,320.00 + SpaceX $2,220.00 (at estimate). Shares: Andres 35% ($7,000), Christelle 20%, Logan 12%, Mia 10%, Jordan 8%, Priya 6%, Sam 5%, Kai 4%. Turnout 94% (month). Rules switch "Show who voted what": **off** by default; on in `03-cabal-rules-sides` / `04-proposal-sides` ("on since Oct 20"). Pending: Leo, Jordan's link, 2h. |
| **Tech Titans** | 6 members, $22,580 treasury, Andres voter 12% ($2,710.00, −1.7%). Open: Logan's Buy SpaceX $500 (Andres has voted Yes), proposed 4h ago, ✓2 ✕0, 2 of 6 voted, 3 replies, ends in 20h (Fri 5:41 AM), ≈ 2.70 sh at the estimate, 2% of treasury. Closed: Logan's Sell TSLA $900 (all 2.2 sh), failed 2–2 Oct 18 6:00 PM (4 of 5 voted, needed 3, Andres No, Priya did not vote), 4d. Logan sold AAPL $480 +6.7%, 5h. Nora joined, 3d. |
| **Degens** (Friday Degens) | 6 members, $23,400 treasury, Andres voter 8% ($1,872.00, +11.2%); all six vote. META: Christelle's Buy META $300, passed 4–1 (5 of 6 voted) about 5h ago, passed at $709.80, filled 0.42 sh at **$716.52** at **4:41 AM**, fee $0.30, now $724.40 = +1.1% since the fill; Christelle called it. Open: Christelle's Buy AMZN $250 (Andres has voted Yes), proposed 1h ago, ✓3 ✕0, 3 of 6 voted, 2 replies, ends in 23h. Priya sold AMZN $250 −2.1% on **Sat Oct 17** (5d; Andres voted Yes) at $192.60; AMZN was $184.51 by Sunday, 4.2% under the sale: the Ledger's best call. |
| **Quant Kids** | 5 members, invited by Sam; Sam bought OpenAI $200 (Pre-IPO), 4d. |
| **Garage Fund** (empty cabal) | Started today by Andres with Zoe and Tomas, 3 members, $600.00 treasury, your share $200 (33%), 0 open votes, bot off. |
| **Open NVDA vote** | Mia's Buy NVDA $400, proposed 2h ago (about 7:53 AM), would close **1:53 PM** ("ends in 4h"); Price $220.00, Shares ≈ 1.82, 2% of treasury; ✓4 (Mia, Christelle, Logan, Sam) ✕1 (Kai), 5 of 8 voted, Jordan and Priya still out (Christelle's "Still out: @Andres, Jordan?"), 6 replies. Andres's vote is the 6th and his Yes the **5th**: at 5 of 8 it passes on the spot (decision 14), so after the hold the card reads Passed 5–1 · 6 of 8 voted · buying 1.82 sh, the stat row's Closes becomes Passed 9:41 AM, and nothing in AC Capital is open ("8 members · 35% yours"). Before his vote it is the only vote that waits on him (Home's Your vote 1, Catch up's Waiting on you 1, the Cabals tab badge 1). |
| **Same vote, closed** | **Passed 5–1** at 9:41 AM on Andres's fifth yes (6 of 8 voted; Jordan and Priya did not), filled 1.82 sh at $219.40 at **9:41 AM**, fee $0.40, now $220.00 = +0.3%; treasury 38 → 39.82 sh. Sides (when on): ✓ Mia, Christelle, Logan, Sam, Andres · ✕ Kai · – Jordan and Priya did not vote. The result push: "NVDA passed 5–1 · Bought 1.82 sh at $219.40." Catch up's cleared row: "Buy NVDA passed 5–1 · now" (fix 2). |
| **Positions** | NVDA 38 sh × $220.00 = $8,360.00, avg $192.95, +14.0% (+$1,027.90), trades: B Jun 3 $181.20, B (July) $188.40, S Aug 20 $214.10, B Sep 30 $196.75; AAPL 24 sh, $4,320.00, avg $183.30, −1.8%; SpaceX 12 sh $2,220.00 at the estimate; META 4 sh $2,897.60 +1.1%; AMZN 5 sh $922.50 +4.8%; TSLA 2.2 sh $907.61 +40.0%. Held total $19,627.71 across 3 cabals. |
| **Quotes** | NVIDIA leads the Watchlist (its star is filled on the stock page, fix 2). NVDA $220.00 +$8.82 / +4.18%; AAPL $180.00 −0.80%; TSLA $412.55 −2.40%; META $724.40 +1.10%; AMZN $184.50 −2.10%; MSFT $512.30 +0.60%. Pre-IPO (secondary estimate, Oct 20; never a percent): SpaceX $185.00 (last round Jun ’26 $165.00, valuation $350B), OpenAI $310.00 ($500B, Mar ’26), Anthropic $96.40 ($183B, Sep ’25, Oct 18 estimate, research only), Stripe $27.50 ($91B, Feb ’26, Oct 14), Databricks $71.80 ($100B, Dec ’25, Oct 9, research only). |
| **Month board (People, Oct 1–31)** | rate · right of resolved · voted: 1 Christelle 92% · 12 of 13 · 14 of 14; 2 Logan 91% · 10 of 11 · 12 of 12; 3 Mia 89% · 8 of 9 · 10 of 11; 4 Jordan 88% · 7 of 8 · 10 of 11; 5 Priya 86% · 6 of 7 · 9 of 11; 6 Sam 83% · 5 of 6 · 8 of 9; 7 Kai 79% · 11 of 14 · 14 of 15; **8 Andres 78% · 7 of 9 · 11 of 11** (one right call puts him past Kai: 8 of 10 = 80%); 9 Rosa 75% · 6 of 8; 10 Nora 71% · 5 of 7; 11 Diego 67% · 4 of 6; 12 Maya 60% · 3 of 5. 24 people. Five resolved votes to rank. |
| **Friends** | the same figures re-ranked among 10 friends: Christelle, Logan, Mia, Jordan, Kai (#5), Andres (#6), Rosa, Nora, Diego, Maya. |
| **Cabals board** | 1 Pilates 93% · 13 of 14 (turnout 96%); 2 Tech Titans 89% · 8 of 9 (92%); 3 AC Capital 86% · 6 of 7 (94%); 4 Degens 83% · 10 of 12 (88%); 5 Quant Kids 80% · 4 of 5 (85%); 6 Night Shift 75% · 6 of 8 (81%); 7 Big Ballers 71% · 5 of 7 (78%); 8 Family 67% · 4 of 6 (75%). |
| **Week board (Oct 19–25)** | Christelle 100% · 6 of 6; Logan 86% · 6 of 7; Mia 83% · 5 of 6; Kai 80% · 4 of 5; Jordan 80% · 4 of 5; not ranked: Andres 3 resolved (2 more), Rosa 2 resolved (3 more). |
| **The Ledger, Week 42** | Oct 12 – 18, delivered Sunday: your stake +1.4% across 3 cabals; voted 3 of 3; right calls 2 of 3; sharpest Christelle; **best call Degens' Sell AMZN** (Sat Oct 17, you voted Yes): AMZN fell 4.2% after it, so the call caught **+4.2%**; the miss Tech Titans' Sell TSLA −2.4% (in the Full Ledger). No buy filled in your cabals that week, so the Story chart has the seal and no B (fix 1). |
| **Cabal rows** | AC Capital +4.7% (+$312.40 on $6,687.60 funded), Tech Titans −1.7%, Degens +11.2%: your stake's return since you funded; the treasury's −4.1% is the cabal's last month. |
| **Cash** | $500.00. Fund $250 → share 35% → 36%, then cash $250.00; Fund invalid $650 = $150.00 over (no fee). Withdraw $120, fee $0.50 on top, ≈ 120.00 USDC on Solana, then cash $379.50; so the most you can send is **$499.50** (Max fills it), and an invalid $650 is **$150.50 over** (fix 1). |
| **Christelle** | #1 this month (92% · 12 of 13), P&L 1M +12.4%, 48 followers, 21 following, 3 cabals (AC Capital admin 20%, Degens voter 15%, Big Ballers voter 8%), topped September's board (11 of 12 right). |
| **First fund** | a new member's day one, drawn from her side: Zoe puts $250 into AC Capital = 1.23% of a $20,250 treasury, the ninth member; her first vote reads 5 of 9 voted. |
| **Garage Fund** | see above; the empty cabal. |
| **Reply counts** (Feed comments, 10-27) | One per post, shown by the reply control: Mia's Buy NVDA 6, Degens' Buy META 5, Logan sold AAPL 5, Nora joined 3, Tech Titans' Sell TSLA failed 4, Priya sold AMZN 1; Logan's quote on Sell TSLA none (a reply in that vote's thread, unanswered), Sam bought OpenAI none. From everyone: Maya's Buy AMZN 2, Rosa's Buy Stripe 4, Ivan sold AAPL none, Sam bought OpenAI none. |
| **Logan's AAPL thread** (`05-post`) | Logan sold AAPL $480 · Tech Titans · +6.7%, 5h (💰 4, 🙌 2). Replies 5, oldest first: Nora 4h "Why sell here?" (❤️ 1) → Logan 4h "Took the gain while it was there." (folded, View 1 reply); Andres 3h "Good timing on this one." (❤️ 2); Christelle 2h, a GIF reply quoting Logan's answer (😂 3, ❤️ 1: her 4 reactions split by emoji, fix 5); Sam now "He called the top again." quoting Andres (no reactions yet). Anyone who can see the post replies. |
| **Sam's push** (`19-notifications`) | "Sam replied to you · now · “He called the top again.” On Logan's AAPL sale." Grouped with Sam (a group of one, no header). It lands at 9:41, so Catch up (6) and the bell are unchanged (coordinator, 10-27). |
| **Pilates** (public, `05-post-public`) | 7 members, treasury $12,400; Andres is not a member. Maya's Buy AMZN $250, proposed 1h ago (8:41 AM), closes Fri 8:41 (ends in 23h); Price $184.50 (AMZN quote), Shares ≈ 1.36, Of treasury 2%; ✓3 ✕0, 3 of 7 voted, 2 replies: Zoe 40m "In. The cloud carries this one." (❤️ 3), Maya 30m "Starter now, more after earnings." quoting Zoe (❤️ 1). |

## Build map

One row per launch frame (fix 1: the founder has picked; the compare rows are gone). "Today" names where the app has it now (`apps/mobile/Monaco/Features/…`, read only); "new" means no screen exists for it yet. **Ticket** names the F launch (phase 1) issues that build the row (the tracking issue, #6410, lists them all, backend and QA included).

| Frame | App screen · state | Today | Kind | Ticket |
|---|---|---|---|---|
| `01-tokens`, `01-tokens-type`, `01-chart-variants` | Design tokens: colour, radii, spacing, type (Home figure 32, the founder's choice), the flat chart fill | `Design/` | change (tokens) | #6432 (colour), #6440 (type, spacing, radii), #6450 (chart fill) |
| `01-tokens-controls` | Shared controls: underline tabs, ranges and period labels, chips, buttons, the tally and hold, the sides row, the proposal stat row, Remind, the ranked row, reactions, the composer with GIF, the crest, the toast | `Components/`, `Design/` | change + new parts | #6449, #6458, #6460, #6461, #6469, #6548, #6551 |
| `02-home-dark`, `02-home-light` | Home · ready (dark, light): wordmark, bell with the Catch up count, figure 32, six blocks | `Home/HomeScreen` and its slots (`HomePendingVotesSlot`, `HomePortfolioSlot`, `HomeBalanceSlot`, `HomeCabalsSlot`, `HomeLeaderboardSlot`, `HomeMarketUpdatesSlot`) | change | #6480, #6496, #6543 |
| `02-home-scrolled` | Home · scrolled (inline wordmark) | `Home/HomeScreen` | change | #6480, #6496, #6543 |
| `02-home-empty` | Home · first use, no cabals (no bell badge) | `Home/HomeScreen` empty state, `HomeNudgeSlot` | change | #6564, #6543 |
| `04b-hold-1-pick`, `-2-holding`, `-3-committed` | Home's vote card · pick, holding, committed (collapses to "You voted Yes") | `Home/HomePendingVotesSlot`, `Proposals/ProposalVoteCard` | change | #6508, #6460 |
| `12-toast` | Toast after a vote, 2.5 s: no Undo when your vote settles the proposal (the trade starts); an ordinary vote's toast has Undo (the kit board). The founder kept this as drawn on 10-27 | `Components/` | change | #6458, #6430 (Undo), #6506 |
| `03-cabals-tab`, `03-cabals-tab-menu` | Cabals tab · ready, long-press menu (the founder's choice, 10-26) | `Groups/CabalsTabScreen`, `CabalsListSlot`, `CabalsInvitesSlot` | change | #6522 (tab), #6560 (long-press menu) |
| `03-cabal-treasury`, `-light`, `-treasury-scrolled` | Cabal page · Treasury tab, at rest and at its end (with Treasury history) | `Groups/CabalScreen`, `CabalHoldingView`, `CabalActivityListRoute` | change | #6481 (header), #6497 (tabs), #6511 (Treasury) |
| `03-cabal-rules`, `-rules-scrolled` | Cabal page · Rules tab, with "Show who voted what" (off) | `Groups/CabalRulesSection` | change + new rule | #6523 |
| `03-cabal-rules-sides` | Cabal page · Rules, the switch on (a rule change by proposal) | `Groups/CabalRulesSection` | new state | #6523, #6498, #6510 |
| `03-cabal-members`, `-members-scrolled` | Cabal page · Members tab (month record and turnout) | `Groups/MembersView` | change | #6534 |
| `03-cabal-members-all` | Members · all, with the pending request and Approve first | `Groups/MembersView`, `InviteMemberView` | change + new row | #6534 |
| `03-cabal-empty` | Cabal page · empty (no proposals yet), "Cabals like yours start with" | `Groups/CabalScreen` empty state | new state | #6524 |
| `15-first-fund` | After a first fund · "You're in." | `Deposit/FundCabalView` outcome (`MoneyOutcome`) | change | #6509 |
| `04-proposal-dark`, `-light`, `04-proposal-scrolled` | Proposal · open (and at the end of its replies, the list's end inset clearing the composer), with the stat row, Remind, rooted replies, composer with ⊕ and GIF | `Proposals/ProposalScreen`, `ProposalHero`, `ProposalVoteCard`, `Comments/CommentThreadView` | change | #6491, #6541, #6548 |
| `04-vote-idle`, `-picked`, `-holding`, `-committed` | Proposal · the vote's four states; committed is also the settle-at-quorum state when your vote reaches the threshold (Passed · buying, no Change) | `Proposals/ProposalVoteCard`, `governance/app/cast_vote.go` (marks Passed on the threshold vote) | change | #6506, #6460, #6520 |
| `19-permission-sheet` | Push permission, right after the first vote | `Notifications/PushPrePromptSheet` | change (copy, placement) | #6554 |
| `04-vote-closed`, `04-proposal-passed`, `04-proposal-failed` | Proposal · closed (passed, failed), the result's stat row | `Proposals/ProposalScreen`, `ProposalStatusSection` | change | #6532, #6521 |
| `04-proposal-sides` | Proposal · closed with the sides (Rules switch on) | `Proposals/ProposalVotersView` | new state | #6539 |
| `15-passed` | Proposal · the passed moment sheet | `Proposals/` | change | #6545 |
| `04-proposal-preipo` | Proposal · Pre-IPO (estimate, ledger, tender) | `Proposals/ProposalScreen` | change | #6550 |
| `04-reply-composer` | Reply · the composer as a half sheet with the quote | `Comments/CommentComposer` | change | #6548 |
| `04-composer-menu` | Composer · ⊕ menu (Poll, Quote the thesis, Share a stock; no Photo: chats take no image uploads at launch, founder 10-27 evening) | `Comments/CommentComposer` | new | #6563 (Poll #6574; Quote the thesis and Share a stock #6568) |
| `04-gif-sheet` | Composer · GIF sheet (trending, search, attribution) | — | new | #6556, #6420 (GIPHY SDK), #6558 |
| `04-gif-reply` | Discussion · a GIF reply bubble | `Comments/CommentThreadView` | new cell | #6556, #6558 |
| `14-hold-sheet` | The hold sheet (from a notification or a link) | `Proposals/` | change | #6557, #6565, #6559 |
| `05-feed-A`, `-A-scrolled`, `-light` | Feed · timeline (Feed A): each post's asset line is the 36 logo and the ticker and amount at Headline 17 (`.font(.headline)`), the cabal and change at Footnote; the reaction chips 24 tall, emoji and count at Footnote semibold 13, 4 apart, each at least 44 wide and 44 × 44 to tap (founder 10-27 evening and late) | `Feed/FeedView`, `FeedItemCell`, `FeedEventParts` | change | #6486, #6500, #6535, #6551, #6469 |
| `05-reactions` | Feed · the quick reaction row (six and +) under the 24 chips, the list scrolled to Mia's post at the bar | `Feed/FeedItemCell` | new | #6469, #6551 |
| `05-emoji-picker` | Feed · the full emoji picker (half sheet) | — | new | #6494 |
| `05-feed-empty` | Feed · a quiet week (its posts with the 36 logo, the Headline ticker line and the 24 chips) | `Feed/FeedView` empty state | change | #6542, #6551 |
| reply control in `05-feed-A`, `-A-scrolled`, `-light`, `05-reactions`, `05-feed-empty`; the glyph on Home's From your cabals rows | Feed post · the reply control (bubble and count after the reactions and +; tapping it or the post opens the thread) | `Feed/FeedItemCell`; Home's preview row is new (Home has no feed slot today; `HomeMarketUpdatesSlot` is markets) | new control | #6551 (Home's rows: #6543) |
| `05-post`, `05-post-scrolled` | Feed post page · any post that is not a proposal or a result: the post as the Feed draws it, then its rooted replies and the docked composer (end inset `--composer-clear` + 40) | new (the post cell from `Feed/FeedItemCell`, the thread from `Comments/CommentThreadView`, `Comments/CommentComposer`) | new | #6555 (thread: #6541) |
| `05-post-reply` | Reply from the Feed · the composer as the half sheet with the post quoted (36 logo; who and verb, then the ticker line at Headline) and GIF in the field | `Comments/CommentComposer` | change | #6548 (GIF in the field: #6556) |
| `05-comment-actions` | Reply · long-press: quick six and +, Reply, Copy, Report (Delete on your own) | `Comments/CommentThreadView` | new | #6552 |
| `05-post-public` | Proposal · seen by a non-member: the live tally without vote buttons, replies read only, reactions open, "Members of <cabal> reply here" in the composer's place | `Proposals/ProposalScreen`, `Comments/CommentThreadView`, `Comments/CommentComposer` | new state | #6553 with #6506 (non-member vote card) |
| `06-markets-A`, `-A-scrolled` | Markets · ready and its end, F's order (the founder's choice): Watchlist · Pre-IPO · Moving today as a sideways strip | `Assets/Stocks`, `MarketsSections`, `MoverTile`, `AssetStockRow` | change | #6487, #6529 |
| `06-markets-search` | Markets · search typing | `Assets/Stocks` search | change | #6501 |
| `06-markets-held` | Markets · Held chip | `Assets/MarketsSortFilter` | change | #6516 |
| `07-stock-dark`, `-light` | Stock detail · NVIDIA | `Assets/AssetDetailClientView`, `AssetDetailHeader`, `AssetDetailChartSection`, `AssetDetailActions` | change | #6528, #6536 |
| `07-stock-preipo`, `-preipo-scrolled` | Stock detail · Pre-IPO (SpaceX) and its end | `Assets/AssetDetailClientView` | change | #6544 |
| `08-review-sheet`, `-preipo` | Propose buy · review sheet (public, Pre-IPO) | `ProposeReviewScreen` | change | #6567 |
| `12-info-sheet` | i-sheet (Market stats) | `Components/` | change | #6562 |
| `09-fund-cabal-empty`, `09-fund-cabal`, `-invalid`, `-settled` | Fund a cabal · empty, typed, invalid, settled (Phantom keypad) | `Deposit/FundCabalView`, `FundRoute`, `ProposeAmountScreen` keypad | change | #6490, #6459, #6475 |
| `09-withdraw-empty`, `09-withdraw`, `-invalid`, `-settled` | Withdraw · empty, typed, invalid, settled | `Settings/WithdrawView`, `WithdrawConfirmView` | change | #6504, #6459, #6476, #6517 |
| `10-profile-own`, `-own-scrolled` | Profile · yours (the founder's card, the month's standing line, docked Deposit) | `Profile/ProfileScreen` and slots | change | #6488, #6505, #6518 |
| `10-profile-other`, `-other-cabals` | Someone else's profile · Activity, Cabals | `Profile/UserProfileScreen`, `UserProfileSharedCabalsSlot` | change | #6530 |
| `11-invite-sheet` | Invite sheet (the cabal is the invite) | `Groups/CabalInviteSheet` | change | #6489 |
| `18-invite-landing` | Invitee landing, before an account | `Groups/PublicCabalView`, `Referrals/ReferralDeepLink` | change | #6502, #6428 |
| `11-share-A`, `-B`, `-C`, `-card-vote`, `-sticker`, `-sticker-on` | Share screen · position, passed proposal, your share, open vote, sticker | `ShareCard/`, `Design/ShareCard` | change | #6519, #6503, #6531, #6537, #6572 |
| `13-leaderboard-C-people`, `-C-cabals`, `-C`, `-C-scrolled` | Leaderboard · People, Cabals, Friends (version C), month, the top three as the flat podium with records | `Groups/LeaderboardView`, `LeaderboardRoute`, `LeaderboardFilterSheet` (becomes the period labels) | change | #6484, #6514, #6499 |
| `13-leaderboard-C-unranked` | Leaderboard · Week, under five resolved votes | `Groups/LeaderboardView` | new state | #6514 |
| `15-leaderboard-moment` | Leaderboard · when the lead changes (crown card, the podium, turnout rows, docked row with the climb bar and its gap line) | `Groups/LeaderboardView` | new state | #6526 |
| `14-lock-screen-launch`, `19-notifications` | Push notifications: the reminder, the proposal, the result, the Ledger, grouped per cabal | `Notifications/`, server push copy | change | #6425, #6444, #6547, #6525 |
| `14-notification`, `14-faceid-alt`, `15-last-vote` | Expanded push with Yes / No (opens the hold sheet), Face ID alternative, the deciding-vote push | `Notifications/` (actionable category) | change | #6565, #6557 |
| `14-wrap-sheet` | The Ledger · half sheet from Sunday's push | — | new | #6570 |
| `14-ledger-story` | The Ledger · Story share | `ShareCard/` | new | #6581 |
| `16-catch-up`, `-open`, `-open-scrolled`, `-empty`, `-empty-scrolled` | Catch up (behind the bell) · list, one by one, caught up | `Notifications/ActivityView`, `ActivityFeedRoute` | change | #6561, #6571, #6566 |

Phase 2 (section 2) maps later to new targets: Live Activities and the Dynamic Island (an ActivityKit extension), Home Screen widgets and StandBy (a WidgetKit extension), Spotlight and Siri (App Intents), seasons and streaks (server and `Groups/LeaderboardView`, `Profile/`).

## Every other content change, with its reason

- **Proposal hero: the price and "2% of treasury" leave the hero's sub-line** for the stat row under it (decision 10): one place for each fact, the price keeps its i there.
- **Home's leaderboard row reads the month** ("This month · you are #8 of 24") rather than the gap to the next rank: the gap ("one right call from #7") is on the board itself, where it can be acted on; on Home the row's job is to say where you stand.
- **Andres is a voter in Degens**, not a member (`10-profile-own*`, `11-invite-sheet`, `10-season-sheet`): he voted Yes on META there (`04-vote-closed`, Catch up), so "8% · member" contradicted the fixture.
- **Spotlight's second intent** reads "Buy SpaceX · you voted Yes" (phase 2): only Buy NVDA waits on Andres (Home "Your vote 1", Catch up "Waiting on you 1"), so offering a SpaceX vote contradicted the fixture.
- **META's fill time is 4:41 AM**, not 2:10 PM (`04-vote-closed`): it passed about 5h before the 9:41 frames, and tokenized stocks trade 24/7.
- **The Ledger sheet shows the best call; the miss moves into Full Ledger** ("Every call, the miss and the top reply"): the sheet is the week's headline at the half detent, and Right calls 2 of 3 already says there was a miss; the miss is one tap away.
- **The Ledger's push copy** is "The Ledger · Week 42 / You voted 3 of 3. Best call: Sell AMZN, +4.2%." (no streak; fix 1 changed the call from Buy NVDA, which had no Week 42 trade), on both Lock Screens.
- **Markets A2's watchlist preview holds four rows** (See all 7): the director's order brings the movers into the first view, and with five rows the Hot section's title rested under the tab bar (a kit audit failure). F's order keeps five.
- **The emoji picker has no title row**: like the system's picker it is grabber, search, grid and categories, which keeps three rows of smileys inside the half detent.
- **The empty cabal's cover is an identity cell with its symbol** (Garage Fund has no picture yet); Monaco green is never the default.
- **The Cabals tab menu's lifted icon starts at 363**, on the grid icon's own position (round 10 drew it at 369, 6 below, after the grid moved).
- **The keyboard's action key is the system blue** (round-6 deferred polish: the keyboard is a system surface; F's kit now draws it so).
