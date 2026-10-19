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

**Coming soon.** A slot whose route is not implemented yet keeps its section header and shows one grey caption, "<Thing> shows up here soon." (for example "Holdings" over "Holdings show up here soon."), in `Typo.caption` with `MonacoTheme.muted`, or `MonacoTheme.onHeroMuted` on the ink hero. A disabled action keeps its label from this page, is disabled, and has one caption in the same style, "<Thing> opens soon." (for example "Funding opens soon."). Such a slot never shows a made-up figure, never uses this page's real-empty copy ("$0.00", "No investors yet") as a placeholder, and never calls a legacy route. Where an implemented route fails, the slot shows its normal error state. The owning ticket replaces the caption with the real content.

**Type scale.** Text sets in SF Pro through a `MonacoTheme.Typo` role, which scales with Dynamic Type. No role is smaller than #4110's scale. Figures are tabular. Sizes are the base at the default text size; the full table is in [design.md](design.md).

| Role | Token | Size |
| --- | --- | --- |
| Display | `Typo.display` | 36 |
| Title | `Typo.title` | 28 |
| Section header | `Typo.section` | 22 |
| Row title, buttons | `Typo.headline` | 18 |
| Body | `Typo.body` | 18 |
| Helper copy | `Typo.subhead`, `subheadStrong` | 17 |
| Caption, metadata | `Typo.caption`, `captionStrong` | 15 |
| Money: hero, large, row, small | `MoneyStyle` | 48, 32, 18, 15 |

**Shared patterns.**

- **Ink hero.** The cabal hero is the one dark `heroInk` band, and at most one per screen. Home's money sits on the paper, not on ink. Cabal hero slots stacked one after another each paint the same full-bleed `heroInk` background, so they read as one band.
- **Section.** Header in `Typo.section`, an optional count badge, an optional "See all" link on the right, then ruled rows (`MonacoSectionHeader`, `MonacoGroupedList`). Sections are ledgers on paper, not cards.
- **Range chips.** A row of capsule chips under a chart or board. Selected is ink with white text. Boards and value curves use ranking's ranges: "1H", "1D", "1W", "1M", "All". Stock charts use market's ranges: "1D", "1W", "1M", "3M", "6M", "1Y", "ALL".
- **Amount entry** (fund, propose, cash out, withdraw). A large centred "$0" that turns ink when non-zero. Quick-pick chips under it. A grey helper line ("$1,000.00 available"). A short grey explainer. A full-width primary button pinned above the tab bar whose label echoes the amount ("Add $500 to the pot"). The button is disabled at $0 and over the limit, and the helper turns red with the limit message.
- **Bottom CTA.** Primary actions sit in a pill pinned above the tab bar, with a spinner and a "…ing" label while submitting.
- **Toasts** confirm every action. A toast is the dark rounded panel above the tab bar. No alerts or banners for confirmations.
- **People and cabals.** A person is an avatar (photo, or the animal their id hashes to) and a display name. Tapping a person anywhere (boards, members, feed, chat, comments, votes) opens `UserProfileRoute`. A cabal is its picture or tinted initials tile and its name. Tapping a cabal opens `CabalRoute`.
- **Assets** show `AssetDisplayName` and the ticker, never a mint or "xStock". Addresses show through `MonacoWalletAddressText`, never hyphenated.

## Navigation

Five tabs, left to right: "Home" (`house`), "Cabals" (`person.3`), "Markets" (`chart.line.uptrend.xyaxis`), "Feed" (`newspaper`), "Profile" (`person.crop.circle`). The tab bar stays visible on pushed screens. The demo film shows four tabs because it predates the feed (M18).

Before the tabs, the session gate runs sign-in, then the first-run steps: handle (#693), phone (#694), X (#694), and later find friends (#663). Each step has a working primary button that calls its route and moves the gate forward. No step may leave the user on a screen whose button does nothing.

```text
Home ──┬─ avatar button ───────────────▶ Profile tab
       ├─ Add money ───────────────────▶ DepositRoute
       ├─ Withdraw ────────────────────▶ WithdrawRoute
       ├─ Needs your vote row ─────────▶ ProposalRoute
       ├─ Your cabals row ─────────────▶ CabalRoute
       └─ Top investors row ───────────▶ UserProfileRoute
Feed ──── item ──▶ ProposalRoute | TransactionRoute | AssetRoute | CabalRoute | FeedItemDetail
Cabals ┬─ + ──▶ New cabal sheet ──── Start a cabal ──▶ create ──▶ CabalRoute
       ├─ search result / card / Top cabals row ──▶ CabalRoute
CabalRoute ─┬─ i ──▶ Cabal details sheet (rules, treasury, edit, leave)
            ├─ share ──▶ Share sheet (the gain card, Share, Copy link), members only
            ├─ Fund ───────▶ FundRoute
            ├─ Propose ────▶ ProposeRoute ──▶ Buy | Sell
            ├─ Cash out ───▶ CashOutRoute
            ├─ Chat ───────▶ ChatRoute
            ├─ proposal ───▶ ProposalRoute
            ├─ holding ────▶ AssetRoute
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
                                             ├─ Delete account ──▶ DeleteAccountRoute
                                             └─ Sign out
```

#2134 adds the slots and route stubs this page names that do not exist yet: `HomeCabalsSlot`, `CabalSliceSlot`, `CabalValueChartSlot`, `CabalActionsSlot`, `CabalHoldingsSlot`, `CabalRulesSlot`, `ProfileStatsSlot`, `ProfileCabalsSlot`, and the routes `JoinRoute()`, `FundRoute(cabalID:)`, `ChatRoute(cabalID:)`, `SettingsRoute()`, `AccountActivityRoute()`, `ProposeFromAssetRoute(symbol:kind:)`. It removes the `CabalCashOutSlot`, `CabalChatSlot` and `ProfileDeleteAccountSlot` stubs, whose jobs move to the action row and Settings.

## Home

Pull to refresh. Refreshes on the hints its slots name. Toolbar: the viewer's avatar at the top right opens the Profile tab (#2134).

| Order | Slot | Owner | Shows |
| --- | --- | --- | --- |
| 1 | `HomeNudgeSlot` | #644 | One onboarding nudge (link your phone, link X). Hidden when there is none |
| 2 | `HomePortfolioSlot` | #660 | Home's money on the paper (no ink band): "Your money in cabals", the total in `moneyFont(.hero)` with the digits rolling on change, a chip "▲ $0.14 · 0.1%" and "All time", a line chart with a soft gradient fill of `GET /v1/me/pnl-history` (no axes; a dashed rule at the range's first point and a value tag on the right showing the all-time P&L (the percent alone when the dollars read $0.00); the change line above the chart is the range change; press and hold for a crosshair whose time pill and readout follow the finger while the line right of it dims; pinch to zoom, one finger pans, "»" returns to the latest point) with range chips 1H to All (default 1D). Empty: "$0.00", chip "$0.00 (0.0%) All time", no chart and no chips |
| 3 | `HomeBalanceSlot` | #610 | A row with the coin glyph, "Account balance" and the amount from `GET /v1/me/balance`. Under it two text buttons, "Add money" (`DepositRoute()`) and "Withdraw" (`WithdrawRoute()`). While funds are moving, a grey line "$50.00 pending" |
| 4 | `HomePendingVotesSlot` | #612 | "Needs your vote" with a count badge. One proposal card per pending vote (the card below), soonest to close first, up to three, then "See all". A passed proposal whose trade has not finished (chip "Buying" or "Selling") sits under a separate "In progress" section until the trade is bought, sold or failed. Each section is hidden when empty |
| 5 | `HomeCabalsSlot` | #660 | "Your cabals". One row per cabal from `GET /v1/me/portfolio`: tile, name, under it "34% of your cabals" (your share of the total across your cabals, whole percent; none when you are in one cabal), your slice value on the right over its return. Row opens `CabalRoute`. Empty: "No money in a cabal yet" / "Cabals you fund show up here with their value. Fund one you're in, or find one to join." with an outline button "Go to Cabals" that selects the Cabals tab |
| 6 | `HomePeopleBoardSlot` | #699 | "Top investors", chips 1H 1D 1W 1M All (default All), the "Everyone" / "Friends" segment (Friends after #658), ranked rows (crown for first, then numbers, avatar, name, return over gain or loss), the viewer's row pinned at the bottom when off the page. Empty: "No investors yet" / "Fund a cabal to get on the board." |

## Cabals tab

Large title "Cabals". Toolbar "+" opens the **New cabal sheet** (#606): one row, "Start a cabal" / "Name it and set the rules" (pushes the create screen). Joining a cabal goes through the search field below and the cabal screen's "Join" or "Request to join".

| Order | Slot | Owner | Shows |
| --- | --- | --- | --- |
| 1 | `CabalsJoinSlot` | #646 | Search field "Find a cabal by name". While a query is typed, results replace everything below: rows with tile, name, "3 members · Open" or "· Approval required", and Join or Request on the right. No match: "No cabal called “<q>”" |
| 2 | `CabalsInvitesSlot` | #696 | "Cabal invites" with Accept and Decline per row. Hidden when empty |
| 3 | `CabalsListSlot` | #606 | "Your cabals" as horizontal cards: tile, name, pot value (`MoneyText`) and return chip (`PnLBadge`), both joined from `GET /v1/me/portfolio` by cabal id, a request badge when `pending_request_count > 0` (creator only), an unread badge from chat (#704). The last card is a dashed "+ New cabal" that opens the New cabal sheet. Empty: "No cabals yet" / "Start one with friends, or search above to find one." with a "Start a cabal" button that opens the New cabal sheet. Under review: "Search above to find one." with no button, and no New cabal card |
| 4 | `CabalsBoardSlot` | #699 | "Top cabals" / "Ranked by return across everyone on Monaco". Rows: rank, tile, name, return over pot value. Empty: "No cabal has put money in yet" / "The first one to fund takes the top spot." |

**Start a cabal** (#606). Title "Start a cabal". Name field with "Pick a name your friends will recognize." Then "The rules", each a title, a one-line description that changes with the choice, and a segmented control: "Who can join" ("Open" / "Approval required", with "Approval required" preselected), "Who votes" ("Everyone" / "People I pick", the creator is the only voter until they pick more in Cabal settings), "To pass" ("Majority" / "Everyone agrees"), "Votes stay open" ("1 hour" / "1 day" / "1 week", with "1 week" preselected; the three `proposal_expiry_seconds` values in [cabals.md](architecture/cabals.md#rules)). Primary "Create cabal", "Creating…" while it runs, then push `CabalRoute` with the toast "Cabal created."

## Cabal screen (`CabalRoute`)

Toolbar: back on the left, and an "i" button on the right that opens the details sheet. The title is empty over the hero and shows the cabal name once scrolled (#2134). While loading, a skeleton of the hero, four action circles and three rows. Load error: "Couldn't load this cabal." with "Try again".

| Order | Slot | Owner | Member | Non-member | Shows |
| --- | --- | --- | --- | --- | --- |
| 1 | `CabalHeaderSlot` | #606 | yes | yes | Hero, ink: tile (creator can tap to change the picture, #647), name in `Typo.title`, up to five stacked member faces and "3 members". Faces open the member board |
| 2 | `CabalClosedSlot` | #2146 | yes | yes | Inline warning row under the header when `status` is `banned`: `exclamationmark.octagon` in the loss colour and "Monaco closed this cabal. Everyone's slice is being cashed out to their account balance." Nothing while active, loading or failed. A banned cabal also hides `CabalActionsSlot`, and wind-down cash outs read "Closed by Monaco" in the activity |
| 3 | `CabalPotSlot` | #2137 | yes | yes | Hero, ink: "In the pot", pot value from `GET /v1/cabals/{id}/pot` (#2136), and the all-time chip "▲ $0.73 · all time" |
| 4 | `CabalValueChartSlot` | #660 | yes | yes | Hero, ink: the pot's value line from `GET /v1/cabals/{id}/value-history`, drawn and held like Home's chart, chips 1D 1W 1M All (default 1M). Short history: a hairline with "Not enough history for the last month yet" |
| 5 | `CabalSliceSlot` | #2137 | yes | hidden | Hero, ink, under a hairline: "Your slice", its value, and on the right "38% of the pot" over your gain or loss. No stake: "$0.00" with "Fund to get a slice" |
| 6 | `CabalPauseSlot` | #657 | yes | yes | A warning row when the cabal is paused, with the reason and "Funding, cash outs and trades resume once it's back." (external deposit) or "…when the pause is lifted." (Monaco pause). Hidden otherwise |
| 7 | `CabalJoinSlot` | #646 | creator only | yes | Non-member: a line naming the policy ("Open: anyone can join." or "Approval required: the creator reviews each request."), then "Join" on an Open cabal or "Request to join" on an Approval required one, or "Request sent" as a warning status row with a text "Cancel request" beneath it, never a disabled primary. After a denial, a "Request declined" row with "Ask again" (or "Join" if the cabal has since opened). Creator: "2 people want to join" with Approve and Deny per row, plus a "Can vote" switch (off by default) on each row when the cabal picks its voters; approving with it on adds the member to the voters in the same tap. Hidden otherwise |
| 8 | `CabalActionsSlot` | #2134 | yes | hidden | Four round ink buttons, equal width: "Fund" (`plus`, `FundRoute`), "Propose" (`arrow.up.right`, `ProposeRoute(cabalID:)`, #613), "Cash out" (`arrow.down.left`, `CashOutRoute`), "Chat" (`bubble.left`, `ChatRoute`, with an unread dot from #704). Propose is disabled with the caption "Only voters can propose" when `me.can_vote` is false. When the actions read fails after the cabal loaded, the buttons give way to "Couldn't load the cabal actions." with "Try again". While the cabal is paused the buttons stay enabled, and Fund and Cash out show the pause on their own screens (#651, #657) |
| 9 | `CabalProposalsSlot` | #612 | yes | yes, read-only | "Needs your vote" with a count badge and "See all" (the full open and closed list). Open proposal cards, newest first. Passed proposals whose trade has not finished sit under "In progress" until the trade is bought, sold or failed. Each section is hidden when empty. Empty: "No open votes", plus "Propose the first buy." only for a voter. When a trade the viewer voted on settles, a toast says so: "Bought $250.00 of Alphabet", "Sold 0.2161 shares of Alphabet", or "Couldn't buy: …" / "Couldn't sell: …" when it is blocked |
| 10 | `CabalHoldingsSlot` | #2137 | yes | yes | "Holdings": an allocation bar with a legend ("● GOOGL 25%  Cash 75%"), then one row per holding (logo, ticker, "0.73 shares · $341.58", or "0.73 tokens" for a pre-IPO holding, value over its gain or loss) and a "Cash" row. Rows open `AssetRoute`. Empty pot: the Cash row and "No stocks right now. Propose a buy to put the cash to work." (non-voters: "No stocks right now."). Zero pot: "Fund, then propose the first buy." (non-voters: "Nothing in the pot yet."). |
| 11 | `CabalMemberBoardSlot` | #699 | yes | yes | "Leaderboard": ranked members with the viewer's row washed and labelled "You", then "Updated …" under the list. Rows open `UserProfileRoute` |
| 12 | `CabalActivitySlot` | #654 | yes | hidden | "Activity" with "See all" past five rows. Rows: glyph, title ("Bought Alphabet", "Money added", "Cashed out"), age plus a status in amber ("Pending") or red ("Failed · Retry"), amount. Rows open `TransactionRoute`. Empty: "Nothing yet" / "Money added and trades show up here." |

**Share sheet.** A share button sits left of "i" for members of an open cabal once the pot has loaded (id `cabal-share-button`). The same sheet opens from the Profile toolbar ("Share your return", `profile-share-button`) and from the share button beside Home's portfolio chip (`home-share-button`). Title "Share", "Done" at the top right. It shows the card as a 4:5 dark image (1080 × 1350 px when shared): the all-time gain and return, the gain curve, three stats ("Put in", "In the pot", "Members" for a cabal; "Put in", "In cabals", "Cabals" for you) and the sharer's invite link. "Share" opens the system sheet with the image and the link; "Copy link" copies the link and shows the toast "Link copied.". It carries only the viewer's own figures and own cabals.

**Cabal details sheet** ("i"). Title "Cabal details", "Done" at the top right. Detents medium and large.

| Order | Slot | Owner | Shows |
| --- | --- | --- | --- |
| 1 | `CabalInviteMemberSlot` | #696 | Row "Invite someone" that pushes the invite-by-handle screen |
| 2 | `CabalRulesSlot` | #2135 | "Rules", read-only for everyone: the cabal name, who votes (with the voter names when it is a list), what passes, how long votes stay open. The creator's rows, including Name, open "Cabal settings" (#647), where "Who votes" is a rule like the others: "Everyone" / "People I pick" with the member checklist beneath, saved by the one Save in one PATCH |
| 3 | `CabalTreasurySlot` | #651 | "Cabal treasury", the warning "Do not send funds here. Transfers are returned.", the address, and "View on Solscan". No copy button |
| 4 | `CabalLeaveSlot` | #697 | Destructive "Leave cabal" with the confirm dialog |

### Proposal card

Used by Home's "Needs your vote", the cabal's proposals and the proposals list (#612). Top row: asset logo, ticker, "Buy" or "Sell", and on the right "Closes in 23h" while open or a status chip once closed ("Bought", "Buying", "Didn't pass", "Expired", "Withdrawn", "Voided by Monaco", "Couldn't buy", "Couldn't sell"). The amount in `moneyFont(.large)`. Proposer avatar, name and age. The reason, up to three lines. The vote tracker: one dot per voter, filled as votes arrive, and "1 of 3 voted · 2 yes to pass". For a voter who has not voted: "Yes" (primary) and "No" (secondary). After voting: "✓ You voted yes" with "Change" (`POST /v1/proposals/{id}/votes` changes a ballot). Toast "Vote recorded.". A paused cabal adds the caption "Trading is paused. If this passes, it won't buy until trading resumes."

## Proposal screen (`ProposalRoute`)

| Order | Slot | Owner | Shows |
| --- | --- | --- | --- |
| 1 | `ProposalDetailSlot` | #612 | The card's header and amount. "Proposed by Jordan · 33m". The tracker, then each voter's line ("Priya voted yes"). Voting buttons as on the card. "Why buy" with the reason in a ruled section. "Expected" with `quote_out_amount` as "about 0.73 shares at $341.57". A "Status" stepper "Voting", "Buying", "Done" that marks the reached step; a failed trade shows "Couldn't buy" at the last step with `swap.failure_message`, a "Retry" button when `retryable`, and a link to its `TransactionRoute`. The proposer sees "Withdraw proposal" while it is open (confirm "Withdraw this proposal?" / "Votes so far are dropped.", toast "Proposal withdrawn."). A gold coin burst plays once when a buy reaches Done |
| 2 | `ProposalCommentsSlot` | #711 | "Discussion", the thread, and a composer pinned at the bottom, "Add a comment". Empty: "No replies yet" / "Start the discussion." |

## Propose (#613)

**Propose chooser** (`ProposeRoute(cabalID:)`), pushed from the action row. Title "Propose". Rows: "Buy a stock" / "Your cabal votes on it first"; "Sell something the cabal owns" / "Apple, Nvidia and 1 more" (disabled with "Nothing to sell yet" when there are no holdings).

**Buy, step 1.** Title "Buy". Search "Search Apple, Tesla, NVDA…", then "Popular". Rows show logo, ticker over name, price over the day's change. A stock that cannot trade is dimmed: "<name> · Can't buy right now".

**From a stock.** The asset screen's "Propose buy" skips step 1. When the viewer can vote in more than one cabal it first pushes **Pick a cabal** (nav title "Pick a cabal"): the question "Which cabal should buy GOOGL?" in `title` type, then one row per cabal with its mark and pot. Loading, failed and empty states keep the canvas and the title. With one cabal it goes straight to Amount. With none it shows the sheet "Join a cabal first" with "Find a cabal".

**Amount.** Title "Buy GOOGL" or "Sell GOOGL". The stock row. Amount entry on the in-screen keypad with chips "$25", "$50", "$100", "Max" (Max is the pot), helper "The pot has $500.00". "+ Add a reason" expands a field "Why should the cabal buy this?" (280 characters, the server's cap, with a counter from 180). The system keyboard replaces the keypad while the field has focus, and its "Done" button, or a tap anywhere outside the field (the amount figure too), brings the keypad back. The `GET /v1/cabals/{id}/proposals/preview` result shows inline: over the pot "More than the pot has", no route "Can't buy <name> right now. Try a smaller amount or another stock." Both disable "Review", because the server refuses them ([product.md](product.md#trading)).

**Review.** Title "Review". "Buy $250.00 of GOOGL". Rows: "Cabal gets" "about 0.73 shares", "Price" "about $341.57 a share", "Pot" "50% of $500.00", "Who votes" the cabal. "Why buy" with the reason. Primary "Send to cabal". Success closes the flow with the toast "Proposal sent to Sunday Investors".

**Sell.** Title "Sell", header "What the cabal owns", holdings largest first. Amount in dollars with "25%", "50%", "All" and helper "The cabal holds $278.47"; a holding without a price takes a token quantity field labelled "Shares" (or "Tokens" for pre-IPO). Review: "Sell 0.6017 shares of AAPL", rows "Raises" "about $139.00", "Cabal keeps", "Who votes". Reason header "Why sell".

## Money

**Add money** (`DepositRoute`, #610 then #650). Title "Add money". #650 makes it a chooser: "Card" (Apple Pay or card, opens the fund page) and "Crypto". Crypto opens a screen titled "Crypto" that shows the card "Your deposit address", a 200 pt QR code of the plain address (not a `solana:` URI) on a white tile with the Solana mark as a badge in its centre (long-press offers "Copy address"; its accessibility label is "QR code of your deposit address"), the address (from the signed-in profile's member wallet, not the balance read, so a balance failure never hides it; an empty address shows "Couldn't load your deposit address." with "Try again", which reloads the profile), primary "Copy address" (toast "Address copied."), and "Only send USDC on Solana to this address." with the Solana mark at 20 pt before it. Then the balance row (the only part that depends on the live balance; when the read fails it shows a dash, "Couldn't load your balance." and its own "Try again") and "How it works": "Send USDC to the address above from an exchange or another app.", "Your account balance updates a few seconds after it arrives.", "Fund a cabal to move it into the pot and grow your slice." Toast on arrival: "Deposit received: $25.00".

**Fund this cabal** (`FundRoute`, #651). Title "Fund this cabal". Amount entry on the in-screen keypad (not the system keyboard) with "$25", "$50", "$100", "Max", helper "$1,000.00 available" (with "· $300.00 pending" while a fund or withdrawal is on its way), one green yield line "Your cabal earns yield on its USDC. Cash it holds earns 3.5% APY." (display only; nothing moves the cash to earn it; a paused cabal puts the pause row above the amount, as Cash out does), a secondary "Deposit" above the primary (`DepositRoute(cabalID:)`), primary "Add $500 to the pot" (disabled "Enter an amount" at $0). Zero balance replaces the amount entry with "Add money first" / "Send USDC on Solana to your deposit address, then fund this cabal." and a primary "Add money" (`DepositRoute(cabalID:)`). Toasts: "Funding this cabal…", then "Added $500 to Sunday Investors.", or "Funding didn't go through. Your balance wasn't charged."

**Cash out** (`CashOutRoute`, #657). Title "Cash out". Amount entry with "25%", "50%", "All", helper "Your slice is worth $200.15", explainer "We sell this much of your slice and move the cash to your account balance. You stay in the cabal.", primary "Cash out $50.03". Under $0.10: "Too small to cash out". No stake: "Nothing to cash out yet" / "Fund this cabal first. Your slice shows up here." Pops back with "Cashing out $50.03. It lands in your balance in about a minute." The toasts floor the amount to cents like the button. The pot, slice and Home balance refresh on the job's hint.

**Withdraw** (`WithdrawRoute`, #652). Title "Withdraw". Amount entry with "Max", helper "$850.03 available", "Send to" with the field "USDC address on Solana", inline address errors, the caveat "A Solana address that accepts USDC. Transfers can't be undone.", primary "Continue". **Confirm**: "You're withdrawing" over the amount, rows "To" (the full address), "From" "Account balance", "Arrives" "About a minute", the caption "Double-check the address. Transfers can't be undone.", primary "Withdraw". After Max the amount reads "$2.99 (full balance)", floored to cents, on the confirm step and in both toasts. Toast "Withdrawing $50.00. It lands in about a minute." The Solscan link appears on the account activity receipt once the withdrawal confirms.

**Account activity** (`AccountActivityRoute`, #2138). Title "Activity". `GET /v1/me/txns`, paged. Rows: glyph, title ("Deposit", "Withdrawal", "Funded Sunday Investors", "Cashed out of Sunday Investors"), date, status ("Pending" amber, "Failed" red), signed amount. A row opens a receipt with the amount, status, time and a "View on Solscan" link. Empty: "No activity yet" / "Deposits, withdrawals and cabal moves show up here."

## Stocks tab

Large title "Markets". Search "Search Apple, Tesla, NVDA…"; results replace the list, and no match shows "No stocks match “<q>”". A chip bar under the search field, "All" (`filter=all`, the default), "Popular" (`filter=popular`) and "Pre-IPO" (`filter=pre_ipo`), filters one paged list. Search keeps the chip, sending `q` and `filter` together. Rows: logo, ticker over name, a moon glyph when the home market is closed, price, day change chip. #577 owns the tab; it is the only live tab today.

**Asset screen** (`AssetRoute`, #577). Title is the ticker. Name and ticker, price in `moneyFont(.hero)`, the change chip for the selected range ("Past day · GOOGL"), market session chip ("Market closed"), "Trading 24/7 on Solana", a candlestick chart (a toggle switches it to a line) with a price axis on the right, a time axis at the bottom, a dotted last-price rule with a green tag on the axis, an interval picker ("5m", "15m", "1h"… offered per range), chips 1D 1W 1M 3M 6M 1Y ALL, two-finger pinch to zoom around the fingers, one-finger horizontal drag to pan back in time (older bars load as the left edge nears), a "»" button that jumps back to the latest bar once scrolled away, and press-and-hold for a crosshair that the price above follows, then "Stats" (open, day high and low, previous close, 52-week high and low, read off the day and year candles and the quote's move; a row the candles cannot give is left out) and the 52-week range bar. The buy premium waits for the API to send it. Pre-IPO adds "Private-market reference" with the premium or discount, "Also available from" issuers, and "About <token>". When one of the viewer's cabals holds it: "Your cabals' position" with one row per cabal. Pinned CTA "Propose buy" with the caption "Your cabal votes before anything is bought", plus "Propose sell" when a cabal holds it. Short history: "Price history builds up over time." / "The curve draws as the token trades."

## Feed tab

Large title "Feed" with two 44pt glass rounds, then the search field "Search the feed" and a list of compact event cards. Pull to refresh; the last five cards load the next page. #671 built the feed, #702 mute and hide, #711 the post, and the UI v3 redesign the layout below.

| Part | Shows |
| --- | --- |
| Bell | "Activity notifications". Pushes the Activity screen, a "on its way" placeholder until that screen ships. The brand dot for unread activity waits for the API |
| Filter | "Filter feed", a menu that applies each pick at once and stays open. "From": "Everyone", "My cabals", "Following". "Show": "All", "Proposals", "Trades", "Price moves", "Cabals". Then "Muted", which opens the muted list. The round fills when either pick differs from the default |
| Card | A bordered card: author row, one event block, reactions row. Long-press mutes or hides, with a toast and "Undo" |
| Author row | Avatar (opens `UserProfileRoute`), name, event pill, then "<cabal> · <time>". A price move has no person, so it shows a market glyph and "Market" |
| Event pill | "Proposal" and "Buy" (green), "Propose sell" and "Sell" (red), "Price move" and "Approved" (amber), "Discussion" (violet), "Cabal" (green) |
| Asset block | Proposals, trades and approvals. Ticker, status ("Voting open", "Executed", "Execution pending"), the thesis on two lines. Opens `ProposalRoute`, `TransactionRoute` or `AssetRoute` |
| Price block | A price move: ticker, the change the title states, and "24h token volume" with "Opens soon" |
| Cabal block | A started or joined cabal: its tint tile, name, "New public cabal". Opens `CabalRoute` |
| Reactions | A heart (toast "Likes open soon."), the comment count (opens the post), and share (toast "Sharing opens soon.") |

States. Loading is a skeleton under the live title and search. Empty: "No updates yet" / "Proposals, trades and conversations will appear here."; with a search or filter, "No matching updates" / "Change the filter to see more activity." Following with no follows: "Follow people to see what they do." with "Find friends". Error: "Couldn’t load this view" with "Try again". A page that fails to load under cards shows "Older updates couldn’t load. The items above are still here." with "Retry older updates", and no toast. The end of the list reads "You’re caught up", or "No more matching updates".

**Opens soon.** The feed API sends no likes, no author one-month return ("+8.2% · 1M", or "P&L private"), no structured amounts, no 24h figures, no reply or approval items and only one kind filter. The app draws those only from the Debug sample harnesses (`-feedHarness`, `-feedPostHarness`), never from live data, and each needs a backend ticket: feed likes, the author return and its permission, structured trade and proposal figures (amount, share of the pot, units, average price, buy or sell), price-move figures, reply and outcome item kinds, and a multi-kind filter. Until the figures exist, a proposal or trade is drawn as a buy.

**Muted** (`FeedMutesRoute`). Title "Muted". One 72pt row per mute with an icon, its label and "Unmute". Empty: "Nothing muted" / "Press and hold a post in your feed to mute its cabal, stock or person. They'll show up here."

**Post and comments** (`FeedItemRoute`, #711). Title "Post & comments", pushed inside the Feed tab.

| Part | Shows |
| --- | --- |
| Author | Avatar (opens `UserProfileRoute`), name, "<time> · <cabal>" ("Public" with no cabal). The one-month return pill waits for the API. A market item shows "Market update" instead |
| Summary card | The item title, its thesis or detail, and for a stock a row with its logo, name, cabal and a chevron that opens the item's destination |
| Discussion | "Discussion", then comments: 30pt avatar, name, time and "Reply", with replies indented one step |
| Composer | "Add a comment" field and a send disc pinned above the tab bar. "Replying to <name>" with a cancel button while replying. "Sending…" while the comment posts, and "Your reply wasn’t sent. Your text is still here." after a failure |

States. Loading is a skeleton. No comments: "No replies yet" / "Start the discussion.", composer kept. A post the viewer cannot read (404 or 403): "This update isn’t available" / "It may have been removed, or you may no longer have permission to view it." with "Back to feed", and no composer. Any other failure: "Couldn’t load this view" with "Try again". Comments that fail to load show a "Couldn't load comments." notice with "Try again". A non-member sees "Only members of this cabal can comment." in place of the field. `CommentThreadView` and `CommentComposerBar` are shared with the proposal screen.

## Chat (`ChatRoute`, #676)

Title is the cabal tile and name. Empty: the tile, the name, then "No messages yet" over "Say hi, or float a stock idea." Day dividers in mono. Others' messages on the left with the author's avatar (opens `UserProfileRoute`) and name over the first of a run; mine on the right in ink. A failed send shows "Not sent · Retry" under the bubble. Composer "Message your cabal" with a send disc. A non-member or removed member sees the composer replaced by "You're not in this cabal anymore." #703 adds threads and delete, #704 "Seen by N" and unread badges, #2147 @mentions.

## Profile tab

Pull to refresh, like Home. Toolbar: a share button at the top right opens the Share sheet with the viewer's return card.

| Order | Slot | Owner | Shows |
| --- | --- | --- | --- |
| 1 | `ProfileHeaderSlot` | #644 | Centred: the photo picker (96 pt avatar with a camera badge, opens "Your face": eight animals and "Choose a photo"), the name, centred under the avatar, with a pencil beside it that opens "Edit name" (a sheet with "Cancel"), "@handle" with a pencil (opens `HandleEditRoute`), "Member since Sep 2026" |
| 2 | `ProfileFollowCountsSlot` | #620 | "12 followers · 8 following" ("1 follower" for one, thousands separated: "12,400 followers"), each opening the follow list |
| 3 | `ProfileStatsSlot` | #2140 | Three columns between rules: "In cabals" (total slice value), "Return" (dollars over percent, current holdings only), "Cabal" or "Cabals" by count (#3939). Rules run full width. At accessibility text sizes the cells become label and value rows |
| 4 | `ProfileBalanceSlot` | #610 | The same balance row as Home, with "Add money" and "Withdraw" |
| 5 | `ProfileCabalsSlot` | #2140 | "Your cabals", the same rows as Home's. Empty: "No money in a cabal yet" / "Fund a cabal to see it here." with "Browse cabals", which opens the Cabals tab |
| 6 | `ProfileLinksSlot` | #682, #663, #2139 | One list of three rows: "Invite friends", "Find friends" (contacts, plus search by name or handle from #2142) and "Settings" |

**Settings** (`SettingsRoute`, #2139). Title "Settings". Rows: "Notifications" (On or Off; while permission is undecided a tap asks for it, otherwise it opens iOS notification settings, #2143), "Activity" (`AccountActivityRoute`), "Withdraw" (`WithdrawRoute`), "Blocked people", "Advanced" / "Block explorers", "Terms" and "Privacy" (open in Safari), and destructive "Delete account" (`DeleteAccountRoute`, #695). Blocked people lists who the viewer blocks (`GET /v1/me/blocks`): avatar, name and "@handle", and "No one blocked" / "People you block show up here." when empty. Last, in its own group, a destructive "Sign out" row with the confirm "Sign out of Monaco?" / "Your money stays where it is. You'll need a new code to sign back in." Footer: app version.

**User profile** (`UserProfileRoute`). Header (#620): avatar, name, "@handle", follower counts, "Follow" or "Following", and a "…" menu with "Report" and "Block" (#2145). Hidden on your own profile: the Follow button and the menu. Then "Cabals you share" (#660): rows with pot figures. Empty: "No cabals in common" / "You and Maya aren't in a cabal together yet." A banned or deleted user: "This account isn't available."

## Who builds what

The slot tables above name each owner. Screens and controls added by this map:

| Ticket | Builds |
| --- | --- |
| #2134 | The slots and routes above that do not exist yet, the live `CabalActionsSlot`, the Home avatar button |
| #2135 | `CabalRulesSlot` and the voter picker in the cabal editor (inline on Cabal settings since #3463) |
| #2136 | `GET /v1/cabals/{id}/pot` |
| #2137 | `CabalPotSlot`, `CabalSliceSlot`, `CabalHoldingsSlot` |
| #2138 | Account activity |
| #2139 | Settings and the "Settings" row of `ProfileLinksSlot` |
| #2140 | `ProfileStatsSlot`, `ProfileCabalsSlot` |
| #2141, #2142 | People search |
| #2143 | The push pre-prompt and the Notifications row |
| #2144, #2145 | Report and block |
| #2146 | A banned cabal's state |
| #2147 | @mentions in chat |
| #2133 | The journey that walks the demo story through all of the above |
