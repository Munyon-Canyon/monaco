# Screen map

This page says what every screen in the iOS app shows, top to bottom, which button goes where, and which ticket builds each part. It is the layout source for the mobile tickets in M9 to M23. A ticket that builds a slot or a route follows its section here. When a ticket and this page disagree, fix whichever is wrong in the same PR.

The demo film (`docs/demo/storyboard.md`) and the last pre-rewrite build (`015447d8`, read old views with `git show 015447d8:<path>`) are the visual reference. Copy in quotes is the exact string. Words follow [product.md](product.md#words): "cabal", "pot", "your slice", "deposit", "fund", "cash out" (sell your slice back to the cabal) and "withdraw" (send USDC out of Monaco). Look and feel follow [design.md](design.md) and `.claude/skills/ios-screen/SKILL.md`. Wiring follows [mobile-navigation.md](how-to/mobile-navigation.md).

## Rules every screen follows

**States.** Every slot and pushed screen has four states, and its ticket names all four:

| State | Shows |
| --- | --- |
| Loading, first load | A skeleton shaped like the rows it stands in for (`BoardRowSkeleton`, `AmountEntrySkeleton`). Never a bare spinner on a blank screen |
| Empty | `EmptyState(title:message:actionTitle:)` with the copy this page gives. A slot marked "hidden when empty" renders nothing instead |
| Error, nothing on screen yet | An inline row: "Couldn't load <thing>." with a secondary "Try again" button |
| Error, content already on screen | Keep the content and show a toast from `ToastCopy.message(for:)` |

**No blank screens.** A tab or pushed screen never renders an empty page. Until a screen's slots are live it shows `NotMigratedView`. Once one slot is live, every always-visible slot on that screen must also be live, or the screen keeps a slot that explains what is coming. #713 deletes `NotMigratedView` only after every slot below is live.

**Shared patterns.**

- **Ink hero.** At most one dark `heroInk` band per screen: Home's money hero and the cabal hero. Hero slots stacked one after another each paint the same full-bleed `heroInk` background, so they read as one band.
- **Section.** Header in `Typo.section`, an optional count badge, an optional "See all" link on the right, then ruled rows (`MonacoSectionHeader`, `MonacoGroupedList`). Sections are ledgers on paper, not cards.
- **Range chips.** A row of capsule chips under a chart or board. Selected is ink with white text. Boards and value curves use ranking's ranges: "1H", "1D", "1W", "1M", "All". Stock charts use market's ranges: "1D", "1W", "1M", "3M", "1Y", "ALL".
- **Amount entry** (fund, propose, add a bot, cash out, withdraw). A large centred "$0" that turns ink when non-zero. Quick-pick chips under it. A grey helper line ("$1,000.00 available"). A short grey explainer. A full-width primary button pinned above the tab bar whose label echoes the amount ("Add $500 to the pot"). The button is disabled at $0 and over the limit, and the helper turns red with the limit message.
- **Bottom CTA.** Primary actions sit in a pill pinned above the tab bar, with a spinner and a "…ing" label while submitting.
- **Toasts** confirm every action. A toast is the dark rounded panel above the tab bar. No alerts or banners for confirmations.
- **People and cabals.** A person is an avatar (photo, or the animal their id hashes to) and a display name. Tapping a person anywhere (boards, members, feed, chat, comments, votes) opens `UserProfileRoute`. A cabal is its picture or tinted initials tile and its name. Tapping a cabal opens `CabalRoute`.
- **Assets** show `AssetDisplayName` and the ticker, never a mint or "xStock". Addresses show through `MonacoWalletAddressText`, never hyphenated.

## Navigation

Five tabs, left to right: "Home" (`house`), "Feed" (`newspaper`), "Cabals" (`person.3`), "Stocks" (`chart.line.uptrend.xyaxis`), "Profile" (`person.crop.circle`). The tab bar stays visible on pushed screens. The demo film shows four tabs because it predates the feed (M18).

Before the tabs, the session gate runs sign-in, then the first-run steps: handle (#693), phone (#694), X (#694), and later find friends (#663). Each step has a working primary button that calls its route and moves the gate forward. No step may leave the user on a screen whose button does nothing.

```text
Home ──┬─ avatar button ───────────────▶ Profile tab
       ├─ Add money ───────────────────▶ DepositRoute
       ├─ Withdraw ────────────────────▶ WithdrawRoute
       ├─ Needs your vote row ─────────▶ ProposalRoute
       ├─ Your cabals row ─────────────▶ CabalRoute
       └─ Top investors row ───────────▶ UserProfileRoute
Feed ──── item ──▶ ProposalRoute | TransactionRoute | AssetRoute | CabalRoute | FeedItemDetail
Cabals ┬─ + ──▶ New cabal sheet ─┬─ Start a cabal ──▶ create ──▶ CabalRoute
       │                         └─ Join with an invite code ──▶ JoinRoute ──▶ CabalRoute
       ├─ search result / card / Top cabals row ──▶ CabalRoute
CabalRoute ─┬─ i ──▶ Cabal details sheet (invite code, rules, treasury, edit, leave)
            ├─ Add money ──▶ FundRoute
            ├─ Propose ────▶ ProposeRoute ──▶ Buy | Sell | Add a trading bot
            ├─ Cash out ───▶ CashOutRoute
            ├─ Chat ───────▶ ChatRoute
            ├─ proposal ───▶ ProposalRoute
            ├─ holding ────▶ AssetRoute
            ├─ trading bot ▶ AgentRoute (#691)
            └─ activity row ▶ TransactionRoute
Stocks ── row ──▶ AssetRoute ── Propose buy ──▶ Pick a cabal ──▶ Amount ──▶ Review
Profile ┬─ Followers / Following ──▶ FollowListRoute
        ├─ Invite friends ──▶ InviteRoute
        ├─ Find friends ────▶ Friends on Monaco (#663, search from #2142)
        └─ Settings ────────▶ SettingsRoute ─┬─ Notifications
                                             ├─ Activity ──▶ AccountActivityRoute
                                             ├─ Withdraw ──▶ WithdrawRoute
                                             ├─ Blocked people
                                             ├─ Advanced
                                             └─ Delete account ──▶ DeleteAccountRoute
```

#2134 adds the slots and route stubs this page names that do not exist yet: `HomeCabalsSlot`, `CabalSliceSlot`, `CabalValueChartSlot`, `CabalActionsSlot`, `CabalHoldingsSlot`, `CabalAgentSlot`, `CabalRulesSlot`, `ProfileStatsSlot`, `ProfileCabalsSlot`, and the routes `JoinRoute()`, `FundRoute(cabalID:)`, `ChatRoute(cabalID:)`, `SettingsRoute()`, `AccountActivityRoute()`, `AgentRoute(cabalID:)`, `ProposeFromAssetRoute(symbol:kind:)`. It removes the `CabalCashOutSlot`, `CabalChatSlot` and `ProfileDeleteAccountSlot` stubs, whose jobs move to the action row and Settings.

## Home

Pull to refresh. Refreshes on the hints its slots name. Toolbar: the viewer's avatar at the top right opens the Profile tab (#2134).

| Order | Slot | Owner | Shows |
| --- | --- | --- | --- |
| 1 | `HomeNudgeSlot` | #644 | One onboarding nudge (link your phone, link X). Hidden when there is none |
| 2 | `HomePortfolioSlot` | #660 | The ink hero: "Your money in cabals", the total in `moneyHero` with the digits rolling on change, a chip "▲ $0.14 · 0.1%" and "all time", an area chart of `GET /v1/me/pnl-history` with range chips 1H to All (default 1D). Empty: "$0.00", chip "$0.00 · all time", a flat hairline, no chips |
| 3 | `HomeBalanceSlot` | #610 | A row with the coin glyph, "Account balance" and the amount from `GET /v1/me/balance`. Under it two text buttons, "Add money" (`DepositRoute()`) and "Withdraw" (`WithdrawRoute()`). While funds are moving, a grey line "$50.00 funding a cabal" |
| 4 | `HomePendingVotesSlot` | #612 | "Needs your vote" with a count badge. One proposal card per pending vote (the card below), soonest to close first, up to three, then "See all". Hidden when empty |
| 5 | `HomeCabalsSlot` | #660 | "Your cabals". One row per cabal from `GET /v1/me/portfolio`: tile, name, "Pot $950.69", your slice value on the right over its return. Row opens `CabalRoute`. Empty: "No cabals yet" / "Start one with friends or join an open one." with an outline button "Browse cabals" that selects the Cabals tab |
| 6 | `HomePeopleBoardSlot` | #699 | "Top investors", chips 1H 1D 1W 1M All (default All), the "Everyone" / "Friends" segment (Friends after #658), ranked rows (crown for first, then numbers, avatar, name, return over gain or loss), the viewer's row pinned at the bottom when off the page. Empty: "No investors yet" / "Fund a cabal to get on the board." |

## Cabals tab

Large title "Cabals". Toolbar "+" opens the **New cabal sheet** (#606): two rows, "Start a cabal" / "Name it and set the rules" (pushes the create screen) and "Join with an invite code" / "Paste the code a friend sent you" (pushes `JoinRoute`, #646).

| Order | Slot | Owner | Shows |
| --- | --- | --- | --- |
| 1 | `CabalsJoinSlot` | #646 | Search field "Find a cabal by name". While a query is typed, results replace everything below: rows with tile, name, "3 members · Open" or "· By request", and Join or Request on the right. No match: "No cabal called “<q>”" |
| 2 | `CabalsInvitesSlot` | #696 | "Cabal invites" with Accept and Decline per row. Hidden when empty |
| 3 | `CabalsListSlot` | #606 | "Your cabals" as horizontal cards: tile, name, pot value, a return chip, a request badge when `pending_request_count > 0` (creator only), an unread badge from chat (#704). The last card is a dashed "+ New cabal" that opens the New cabal sheet. Empty: "No cabals yet" / "Search above or start one with the + button." |
| 4 | `CabalsValueChartSlot` | #660 | "Your cabals' return", one line per cabal in its tint, chips 1D 1W 1M All. Hidden when the viewer has no cabal |
| 5 | `CabalsBoardSlot` | #699 | "Top cabals" / "Ranked by return across everyone on Monaco". Rows: rank, tile, name, return over pot value. Empty: "No cabal has put money in yet" / "The first one to fund takes the top spot." |

**Start a cabal** (#606). Title "Start a cabal". Name field with "Pick a name your friends will recognize." Then "The rules", each a title, a one-line description that changes with the choice, and a segmented control: "Who can join" ("Anyone" / "I approve"), "Who votes" ("Everyone" / "Just me"), "To pass" ("Majority" / "Everyone agrees"), "Votes stay open" ("1 hour" / "1 day" / "1 week", the three `proposal_expiry_seconds` values in [cabals.md](architecture/cabals.md#rules)). Primary "Create cabal", "Creating…" while it runs, then push `CabalRoute` with the toast "Cabal created."

**Join a cabal** (`JoinRoute`, #646). Title "Join a cabal". A mono field "Invite code" with an ink paste button at its right edge, the helper "Paste the invite code your friend shared.", primary "Join cabal" (or "Request to join" for a request cabal). Success pushes `CabalRoute` with "You're in." or "Request sent. You'll be in once the creator says yes."

## Cabal screen (`CabalRoute`)

Toolbar: back on the left, and an "i" button on the right that opens the details sheet. The title is empty over the hero and shows the cabal name once scrolled (#2134). While loading, a skeleton of the hero, four action circles and three rows. Load error: "Couldn't load this cabal." with "Try again".

| Order | Slot | Owner | Member | Non-member | Shows |
| --- | --- | --- | --- | --- | --- |
| 1 | `CabalHeaderSlot` | #606 | yes | yes | Hero, ink: tile (creator can tap to change the picture, #647), name in `Typo.title`, up to five stacked member faces and "3 members". Faces open the member board |
| 2 | `CabalPotSlot` | #2137 | yes | yes | Hero, ink: "In the pot", pot value from `GET /v1/cabals/{id}/pot` (#2136), and the all-time chip "▲ $0.73 · all time" |
| 3 | `CabalValueChartSlot` | #660 | yes | yes | Hero, ink: the pot's value curve from `GET /v1/cabals/{id}/value-history`, chips 1D 1W 1M All (default 1M). Short history: a hairline with "Not enough history for the last month yet" |
| 4 | `CabalSliceSlot` | #2137 | yes | hidden | Hero, ink, under a hairline: "Your slice", its value, and on the right "38% of the pot" over your gain or loss. No stake: "$0.00" with "Add money to get a slice" |
| 5 | `CabalPauseSlot` | #657 | yes | yes | A warning row when the cabal is paused, with the reason and "Funding and cash outs resume after…". Hidden otherwise |
| 6 | `CabalJoinSlot` | #646 | creator only | yes | Non-member: primary "Join cabal", "Request to join", or "Request sent" with "Cancel request". Creator: "2 people want to join" with Approve and Deny per row. Hidden otherwise |
| 7 | `CabalActionsSlot` | #2134 | yes | hidden | Four round ink buttons, equal width: "Add money" (`plus`, `FundRoute`), "Propose" (`arrow.up.right`, `ProposeRoute(cabalID:)`, #613), "Cash out" (`arrow.down.left`, `CashOutRoute`), "Chat" (`bubble.left`, `ChatRoute`, with an unread dot from #704). Propose is disabled with the caption "Only voters can propose" when `me.can_vote` is false. While the cabal is paused the buttons stay enabled, and Fund and Cash out show the pause on their own screens (#651, #657) |
| 8 | `CabalProposalsSlot` | #612 | yes | yes, read-only | "Needs your vote" with a count badge and "See all" (the full open and closed list). Open proposal cards, newest first. Empty for a member: "No open votes" / "Propose the first buy." |
| 9 | `CabalHoldingsSlot` | #2137 | yes | yes | "Holdings": an allocation bar with a legend ("● GOOGL 25%  Cash 75%"), then one row per holding (logo, ticker, "0.73 shares · $341.58", value over its gain or loss) and a "Cash" row. Rows open `AssetRoute`. Empty pot: the Cash row and "Nothing bought yet. Propose the first buy." Zero pot: "Add money, then propose the first buy." |
| 10 | `CabalAgentSlot` | #691 | yes | yes | "Trading bot": one row with the bot's name, "$200.00 budget" and its state ("Active", "Paused"). Opens the trading bot screen. Hidden when the cabal has no bot |
| 11 | `CabalMemberBoardSlot` | #699 | yes | yes | "Leaderboard": ranked members with the viewer's row washed and labelled "You". Rows open `UserProfileRoute` |
| 12 | `CabalActivitySlot` | #654 | yes | hidden | "Activity" with "See all" past five rows. Rows: glyph, title ("Bought Alphabet", "Money added", "Cashed out", "Scout · Bought Nvidia"), age plus a status in amber ("Pending") or red ("Failed · Retry"), amount. Rows open `TransactionRoute`. Empty: "Nothing yet" / "Money added and trades show up here." |

**Cabal details sheet** ("i"). Title "Cabal details", "Done" at the top right. Detents medium and large.

| Order | Slot | Owner | Shows |
| --- | --- | --- | --- |
| 1 | `CabalInviteCodeSlot` | #646 | Card "Invite code", the code in mono, "Friends paste this code to join the cabal.", primary "Copy code" (turns into "Copied") and secondary "Share" |
| 2 | `CabalInviteMemberSlot` | #696 | Row "Invite someone" that pushes the invite-by-handle screen |
| 3 | `CabalRulesSlot` | #2135 | "Rules", read-only for everyone: who can join, who votes (with the voter names when it is a list), what passes, how long votes stay open. The creator's rows open the editor (#647) |
| 4 | `CabalTreasurySlot` | #651 | "Cabal treasury", the warning "Cabal treasury. Do not send funds here. Transfers are returned.", the address, and "View on Solscan". No copy button |
| 5 | `CabalEditSlot` | #647 | Creator only: "Edit cabal" |
| 6 | `CabalLeaveSlot` | #697 | Destructive "Leave cabal" with the confirm dialog |

### Proposal card

Used by Home's "Needs your vote", the cabal's proposals and the proposals list (#612). Top row: asset logo, ticker, "Buy" or "Sell", and on the right "Closes in 23h" while open or a status chip once closed ("Bought", "Buying", "Didn't pass", "Expired", "Withdrawn", "Voided by Monaco", "Couldn't buy", "Couldn't sell"). The amount in `moneyLarge`. Proposer avatar, name and age. The reason, up to three lines. The vote tracker: one dot per voter, filled as votes arrive, and "1 of 3 voted · 2 yes to pass". For a voter who has not voted: "Yes" (primary) and "No" (secondary). After voting: "✓ You voted yes" with "Change" (`POST /v1/proposals/{id}/votes` changes a ballot). Toast "Vote in". A paused cabal adds the caption "Trading is paused. If this passes, it won't buy until trading resumes."

## Proposal screen (`ProposalRoute`)

| Order | Slot | Owner | Shows |
| --- | --- | --- | --- |
| 1 | `ProposalDetailSlot` | #612 | The card's header and amount. "Proposed by Jordan · 33m". The tracker, then each voter's line ("Priya voted yes"). Voting buttons as on the card. "Why buy" with the reason as a quote. "Expected" with `quote_out_amount` as "about 0.73 shares at $341.57". A "Status" stepper "Voting", "Buying", "Done" that marks the reached step; a failed trade shows "Couldn't buy" at the last step with `swap.failure_message`, a "Retry" button when `retryable`, and a link to its `TransactionRoute`. The proposer sees "Withdraw proposal" while it is open (confirm "Withdraw this proposal?" / "Votes so far are dropped.", toast "Proposal withdrawn."). A gold coin burst plays once when a buy reaches Done |
| 2 | `ProposalCommentsSlot` | #711 | "Comments", the thread, and a composer pinned at the bottom, "Add a comment". Empty: "No comments yet" |

## Propose (#613)

**Propose chooser** (`ProposeRoute(cabalID:)`), pushed from the action row. Title "Propose". Rows: "Buy a stock" / "Your cabal votes on it first"; "Sell something the cabal owns" / "Apple, Nvidia and 1 more" (disabled with "Nothing to sell yet" when there are no holdings); and in M23 "Add a trading bot" / "Give a bot a budget from the pot" (#691).

**Buy, step 1.** Title "Buy". Search "Search Apple, Tesla, NVDA…", then "Popular". Rows show logo, ticker over name, price over the day's change. A stock that cannot trade is dimmed: "<name> · Can't buy right now".

**From a stock.** The asset screen's "Propose buy" skips step 1. When the viewer can vote in more than one cabal it first pushes **Pick a cabal**: "Which cabal should buy GOOGL?" and one row per cabal with its pot. With one cabal it goes straight to Amount. With none it shows the sheet "Join a cabal first" with "Browse cabals".

**Amount.** Title "Amount". The stock row. Amount entry with chips "$25", "$50", "$100", "Max" (Max is the pot), helper "The pot has $500.00". "+ Add a reason" expands a field "Why should the cabal buy this?" (280 characters, the server's cap, with a counter from 230). The `GET /v1/cabals/{id}/proposals/preview` result shows inline: over the pot "More than the pot has", no route "Can't buy <name> right now. Try a smaller amount or another stock." Both disable "Review", because the server refuses them ([product.md](product.md#trading)).

**Review.** Title "Review". "Buy $250.00 of GOOGL". Rows: "Cabal gets" "about 0.73 shares", "Price" "about $341.57 a share", "Pot" "50% of $500.00", "Who votes" the cabal. "Why buy" with the reason. Primary "Send to cabal". Success closes the flow with the toast "Proposal sent to Sunday Investors".

**Sell.** Title "Sell", header "What the cabal owns", holdings largest first. Amount in dollars with "25%", "50%", "All" and helper "The cabal holds $278.47"; a holding without a price takes a token quantity field labelled "Shares" (or "Tokens" for pre-IPO). Review: "Sell 0.6017 shares of AAPL", rows "Raises" "about $139.00", "Cabal keeps", "Who votes". Reason header "Why sell".

## Money

**Add money** (`DepositRoute`, #610 then #650). Title "Add money". #650 makes it a chooser: "Card" (Apple Pay or card, opens the fund page) and "Crypto". Crypto shows the card "Your deposit address", the address, primary "Copy address" (toast "Address copied."), and "Only send USDC on Solana to this address." Then the balance row and "How it works": "Send USDC to the address above from an exchange or another app.", "Your account balance updates a few seconds after it arrives.", "Fund a cabal to move it into the pot and grow your slice." Toast on arrival: "Deposit received: $25.00".

**Fund this cabal** (`FundRoute`, #651). Title "Fund this cabal". Amount entry with "$25", "$50", "$100", "Max", helper "$1,000.00 available" (with "· $300.00 funding" while one is in flight), the explainer "The money leaves your account balance and joins the pot. Your slice grows by the same amount.", the treasury warning line, primary "Add $500 to the pot". Zero balance replaces the amount entry with "Add money first" / "Send USDC on Solana to your deposit address, then fund this cabal." and a primary "Add money" (`DepositRoute(cabalID:)`). Toasts: "Funding this cabal…", then "Added $500 to Sunday Investors.", or "Funding didn't go through. Your balance wasn't charged."

**Cash out** (`CashOutRoute`, #657). Title "Cash out". Amount entry with "25%", "50%", "All", helper "Your slice is worth $200.15", explainer "We sell this much of your slice and move the cash to your account balance. You stay in the cabal.", primary "Cash out $50.03". Under $0.10: "Too small to cash out". No stake: "Nothing to cash out yet" / "Add money to this cabal first. Your slice shows up here." Pops back with "Cashing out $50.03. It lands in your balance in about a minute". The pot, slice and Home balance refresh on the job's hint.

**Withdraw** (`WithdrawRoute`, #652). Title "Withdraw". Amount entry with "Max", helper "$850.03 available", "Send to" with the field "USDC address on Solana", inline address errors, the caveat "A Solana address that accepts USDC. Transfers can't be undone.", primary "Continue". **Confirm**: "You're withdrawing" over the amount, rows "To" (the full address), "From" "Account balance", "Arrives" "About a minute", the caption "Double-check the address. Transfers can't be undone.", primary "Withdraw". Toast "Withdrawing $50.00. It lands in about a minute." The Solscan link appears on the account activity receipt once the withdrawal confirms.

**Account activity** (`AccountActivityRoute`, #2138). Title "Activity". `GET /v1/me/txns`, paged. Rows: glyph, title ("Deposit", "Withdrawal", "Funded Sunday Investors", "Cashed out of Sunday Investors"), date, status ("Pending" amber, "Failed" red), signed amount. A row opens a receipt with the amount, status, time and a "View on Solscan" link. Empty: "No activity yet" / "Deposits, withdrawals and cabal moves show up here."

## Stocks tab

Large title "Stocks". Search "Search Apple, Tesla, NVDA…"; results replace the sections, and no match shows "No stocks match “<q>”". Sections: "Popular" (`filter=popular`), "Pre-IPO" (`filter=pre_ipo`), "All stocks" (`filter=all`, paged). Rows: logo, ticker over name, a moon glyph when the home market is closed, price, day change chip. #577 owns the tab; it is the only live tab today.

**Asset screen** (`AssetRoute`, #577). Title is the ticker. Name and ticker, price in `quoteHero`, the change chip for the selected range ("Past day · GOOGL"), market session chip ("Market closed"), "Trading 24/7 on Solana", a scrubbable chart with chips 1D 1W 1M 3M 1Y ALL, then "Stats" (open, day high and low, previous close, 52-week high and low, buy premium) and the 52-week range bar. Pre-IPO adds "Private-market reference" with the premium or discount, "Also available from" issuers, and "About <token>". When one of the viewer's cabals holds it: "Your cabals' position" with one row per cabal. Pinned CTA "Propose buy" with the caption "Your cabal votes before anything is bought", plus "Propose sell" when a cabal holds it. Short history: "Price history builds up over time." / "The curve draws as the token trades."

## Feed tab

#671 owns it: chips "All", "Proposals", "Trades", "Price moves", "Cabals"; an "Everyone" / "Following" toggle; search; cells with the actor's avatar (opens `UserProfileRoute`), title, detail, time and comment count. Empty: "Nothing here yet." Following with no follows: "Follow people to see what they do." with "Find friends". #702 adds hide and mute, #711 the item detail with comments.

## Chat (`ChatRoute`, #676)

Title is the cabal tile and name. Empty: the tile, the name, "No messages yet. Say hi to your cabal or float a stock idea before someone proposes a buy." Day dividers in mono. Others' messages on the left with the author's avatar (opens `UserProfileRoute`) and name over the first of a run; mine on the right in ink. A failed send shows "Not sent · Retry" under the bubble. Composer "Message your cabal" with a send disc. A non-member or removed member sees the composer replaced by "You're no longer in this cabal, so its chat is closed to you." #703 adds threads and delete, #704 "Seen by N" and unread badges, #2147 @mentions.

## Trading bot (M23, #691)

Title "Trading bot". The bot's name and state. "Budget" / "From the pot" / "$200.00", then "Spent" and "Holdings" from the bot's trades. The "Bot key" card: the key masked with "Show key" (the reveal is audited), "Paste this key into your bot. Anyone in the cabal can copy it here until the bot is removed.", primary "Copy connect instructions", secondary "Copy key". Below, "Trades" lists the bot's swaps with their `reason`. "Spent", "Holdings" and "Trades" need an agent read route that M23 does not ticket yet. Toast "Connect instructions copied".

## Profile tab

| Order | Slot | Owner | Shows |
| --- | --- | --- | --- |
| 1 | `ProfileHeaderSlot` | #644 | Centred: the photo picker (96 pt avatar with a camera badge, opens "Your face": eight animals and "Choose a photo"), the name with a pencil that opens "Edit profile", "@handle" (opens `HandleEditRoute`), "Member since Sep 2026" |
| 2 | `ProfileFollowCountsSlot` | #620 | "12 Followers · 8 Following", each opening the follow list |
| 3 | `ProfileStatsSlot` | #2140 | Three columns between rules: "In cabals" (total slice value), "All time" (return), "Cabals" (count) |
| 4 | `ProfileBalanceSlot` | #610 | The same balance row as Home, with "Add money" and "Withdraw" |
| 5 | `ProfileCabalsSlot` | #2140 | "Your cabals", the same rows as Home's. Empty: "No cabals yet" / "Start a cabal or join one from the Cabals tab." |
| 6 | `ProfileInviteSlot` | #682 | Row "Invite friends" |
| 7 | `ProfileFindFriendsSlot` | #663 | Row "Find friends" (contacts, plus search by name or handle from #2142) |
| 8 | `ProfileSettingsSlot` | #2139 | Row "Settings" |
|  | Sign out | shell | Destructive "Sign out" with the confirm "Sign out of Monaco?" / "Your money stays where it is. You'll need a new code to sign back in." |

**Settings** (`SettingsRoute`, #2139). Title "Settings". Rows: "Notifications" (On or Off, opens the system settings when off, #2143), "Activity" (`AccountActivityRoute`), "Withdraw" (`WithdrawRoute`), "Blocked people" (#2145), "Advanced" / "Block explorers", "Terms" and "Privacy" (open in Safari), and destructive "Delete account" (`DeleteAccountRoute`, #695). Footer: app version.

**User profile** (`UserProfileRoute`). Header (#620): avatar, name, "@handle", follower counts, "Follow" or "Following", and a "…" menu with "Report" and "Block" (#2145). Hidden on your own profile: the Follow button and the menu. Then "Cabals you share" (#660): rows with pot figures. Empty: "No cabals in common" / "You and Maya aren't in a cabal together yet." A banned or deleted user: "This account isn't available."

## Who builds what

The slot tables above name each owner. Screens and controls added by this map:

| Ticket | Builds |
| --- | --- |
| #2134 | The slots and routes above that do not exist yet, the live `CabalActionsSlot`, the Home avatar button |
| #2135 | `CabalRulesSlot` and the voter picker in the cabal editor |
| #2136 | `GET /v1/cabals/{id}/pot` |
| #2137 | `CabalPotSlot`, `CabalSliceSlot`, `CabalHoldingsSlot` |
| #2138 | Account activity |
| #2139 | Settings and `ProfileSettingsSlot` |
| #2140 | `ProfileStatsSlot`, `ProfileCabalsSlot` |
| #2141, #2142 | People search |
| #2143 | The push pre-prompt and the Notifications row |
| #2144, #2145 | Report and block |
| #2146 | A banned cabal's state |
| #2147 | @mentions in chat |
| #2133 | The journey that walks the demo story through all of the above |
