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

## Sign in

`LoginView` draws two layouts over one `OTPLoginForm`, so the address, the cooldown and the typed code survive a step change. The design is `docs/design/v3/` (`signin` and `verify`). The journey is [Sign in](journeys/auth/sign-in.md), and it drives the same identifiers in both layouts.

| Layout | Top to bottom |
| --- | --- |
| Sign in | Wordmark "monaco" (`MonacoWordmark`, header). Headline "Your people. / Your next chapter." and "Pool funds. Propose an investment. / Decide together." The sign-out reason, when set, as an info notice (`signOutReasonNotice`). The "Text message" / "Email" segments, when both methods are on. A label ("Phone number" or "Email address") over a 48 pt field with the placeholder "+1 (202) 555-0100" or "you@example.com", then the helper "We'll send a code to sign you in." Primary "Send code" (`smsSendCodeButton`, `emailSendCodeButton`). The cover card "One pot. Shared decisions." / "Join a cabal with people you know, or start your own." (`AuthCoverCard`: a `CabalCover` thumb with the brand tint gradient and the mark, no stock photo, [decision 8](design/v3/decisions.md)). Debug builds add "Dev: sign in as …" under it |
| Verify code | `VerifyCodeHeader`: a glass "Back" button (`smsVerifyBackButton`, `emailVerifyBackButton`) and the title "Verify code". "Check your messages." (SMS) or "Check your email.", then "Enter the 6-digit code sent to" over the address. Six code boxes, one text field (`smsCodeField`, `emailCodeField`). Primary "Continue" (`smsVerifyButton`, `emailVerifyButton`). Text links "Send a new code" (`smsResendCodeButton`, counts down as "Send a new code in 23s") and "Change number" or "Change email" (`smsChangeAddressButton`) |

Back, "Change number" and "Change email" all return to the address step with the address kept. A code of six digits submits on the sixth.

| State | Sign in | Verify code |
| --- | --- | --- |
| Loading | Info notice "Sending your sign-in code…" under the helper, button "Sending…" and held | Info notice "Verifying your code…", button "Verifying…" and held |
| Empty | The empty field with its placeholder | Six empty boxes |
| Error row | The address can't be sent to: the field border and the line "Enter your full mobile number." (or "Enter an email address, like you@example.com.") turn `loss` above the helper. Any other failed send: an error notice with the `LoginFailureCopy` message | A rejected code: the boxes turn `loss`, the code clears, and an error notice reads "That code didn't work. Check it, or send a new one." Any other failure: an error notice with its message |
| Error toast | The offline failure also raises an error toast | The same |
| Connection error | The offline failure replaces the method segments, the field, the button and the cover card with "Couldn't load this view" and "Try again", under the wordmark and headline. "Try again" clears the failure and keeps the address | Replaces everything under the bar. "Try again" clears the failure and the boxes, and keeps the cooldown |

Privy does not report an expired code apart from a rejected one, so the design's expired state ("That code expired. Request a new one.", warn tone) is not drawn: both show the rejected notice. Draw it when `LoginFailure` gains an `expired` case.

## Home

Pull to refresh. Refreshes on the hints its slots name. The v3 brand toolbar: the "monaco" wordmark on the left (a header, read as "Monaco"), then the bell (`ActivityFeedRoute`; its unread dot stays hidden until a notifications API exists) and the viewer's avatar, which opens the Profile tab. Slots sit `Space.section` (24) apart; the cabal rail bleeds to the screen edge.

| Order | Slot | Shows |
| --- | --- | --- |
| 1 | `HomeNudgeSlot` | One onboarding nudge (link your phone, link X) as an info `MonacoNotice` with a close button. Hidden when there is none |
| 2 | `HomePortfolioSlot` | Optional warn notice "Prices delayed" / "Last updated N minutes ago." when a refresh failed over loaded data. Then the hero: the signed P&L for the selected period in `moneyFont(.hero)` ("−$417.68"), under it the signed percent in profit or loss and the period ("past week", "past month", "all time"), the line chart of `GET /v1/me/pnl-history` at 170pt with its start and end dates (hold for the crosshair, pinch to zoom; the hero follows the scrubbed point), and the period track 1W · 1M · All (default 1M; 3M and 1Y open when the backend serves them). Short history: "Not enough history for the last month yet" at chart height. Cash only: "$0.00", "No positions yet", a quiet chart "No investment history", no periods. Empty: "$0.00" and "No investment history yet". Loading: a page skeleton labelled "Loading Home". All three reads offline: "Couldn't load this view" with "Try again" |
| 3 | `HomeBalanceSlot` | The money strip under a hairline: Net worth (cabal shares plus cash, summed in MonacoCore) and Cash from `GET /v1/me/balance`, each a button that opens Your money (`BalancesRoute()`), and a compact "Deposit" capsule (`DepositRoute()`). The card-deposit "arriving" and pending lines stay under it in the warning colour. Balance failed: "—" and an error row. Hidden when there is no cabal and no cash. At accessibility sizes the strip stacks |
| 4 | `HomePendingVotesSlot` | "Proposals" with a count pill of the cards that need the viewer and "See all" (`PendingVotesRoute()`). Up to two decision cards, needs-your-vote first, then trades in progress. Nothing pending: "No proposals need your attention." and "Closed decisions remain in proposal history.". Loading: one skeleton card. Failed: a card "Proposals couldn't load" with "Try again". Hidden when there are no positions |
| 5 | `HomeCabalsSlot` | "Your cabals" and "See all" (the Cabals tab). A horizontal rail of 300pt `CabalCoverCard`s from `GET /v1/me/portfolio`: tint and picture cover, name, member count, a "1 vote needed" badge, your share, your ownership of the pot, your all-time P&L or "Not funded", and a share disc. A card opens `CabalRoute`. No cabals: "Start with your people" / "Create a cabal or open an invitation from a friend." with "Create a cabal" and an outline "Explore cabals" that selects the Cabals tab |
| 6 | `HomeLeaderboardSlot` | "Leaderboard" and "View all" (`HomeLeaderboardStubRoute` until S13). A Cabals / People menu select and a 1W / 1M / All menu select (default Cabals, 1M), the line "Cabal P&L · 1M" or "Personal P&L · 1M", and the top three `BoardRow`s. The rank-change glyph and "Rank change" stay hidden until the backend serves rank movement. Empty: "No investors yet" / "Fund a cabal to get on the board." or "No cabals ranked yet" / "Public cabals show up here once they're funded." |
| 7 | `HomeMarketUpdatesSlot` | "Market updates" and "Markets" (the Markets tab). The largest 24 hour mover in the catalogue ("NVIDIA · +4.18%", "24h token move", opens `AssetRoute`) and a static "News" row, "News opens soon.", until there is a news source. Hidden when there is no cabal and no cash |

## Your money (`BalancesRoute`)

Opens from the Home money strip (the "Net worth" and "Cash" metrics). A pushed screen with the inline title "Your money", the tab bar visible and pull to refresh, which reloads both reads. `BalancesModel` composes `GET /v1/me/portfolio` and `GET /v1/me/balance`; net worth is the portfolio total plus available cash, summed in `Int64` micros in MonacoCore.

| Part | Shows |
| --- | --- |
| Performance | The eyebrow "Net worth", the figure in `moneyFont(.hero)` and the helper "Your cabal shares + cash" |
| Two stats | "In cabals" and "Cash" between hairlines in `moneyFont(.card)`. At accessibility sizes the columns stack |
| Actions | Three `CircleAction` tiles: "Deposit" (`DepositRoute()`), "Withdraw" (`WithdrawRoute()`) and "Activity" (`AccountActivityRoute()`) |
| Your allocation | One row per portfolio cabal, in portfolio order: a 40pt circular `CabalMark`, the name, "Your share", the value and a chevron (`CabalRoute`). The Cash row is always last: a wallet glyph, "Cash", "Not allocated to a cabal", the cash value (`DepositRoute()`) |
| Closing notice | Info notice "These are your own shares, not the full values of your cabals. Personal returns exclude deposits as investment gains." |

States. Cash only (the portfolio has no rows): "In cabals" reads "$0.00" and an info notice "No funds are allocated to a cabal yet." replaces the cabal rows. Loading: a skeleton labelled "Loading your money". Either read failed with nothing cached: "Couldn’t load this view" with "Try again", which reloads both reads. A refresh that fails over cached figures keeps them and raises the error toast. Harness: `-balancesHarness <ready|cashOnly|loading|error>`.

## Activity (`ActivityFeedRoute`)

Opens from the Home toolbar bell. A pushed screen with the inline title "Activity" and the tab bar visible. `ActivityFeedModel` is fixture-backed: the live source returns `comingSoon` until the backend serves notifications.

| Part | Shows |
| --- | --- |
| Mark all read | A trailing text button in every loaded state, including empty. It marks every item read with a 0.2 s fade; a no-op when nothing is unread |
| Group label | "New" while any item is unread, otherwise "Earlier" |
| Items | A 38pt avatar, the title, up to three lines of body, "<Cabal> · <age>" and an optional `StatusBadge` (for example the warn badge "Execution pending"), then a chevron. Unread items sit on a `brandSoft` block 8pt apart; read items are plain rows divided by hairlines. A tap marks the item read and opens its `PushRoute` destination through `PushRouter` |
| Account update | A wallet-glyph `MonacoRow` ("Deposit confirmed", "$100.00 · Yesterday") that opens `AccountActivityRoute()` |

States. Empty: "You’re caught up" / "Replies, votes and proposal updates appear here.". Loading: five row skeletons labelled "Loading activity". Failed: "Couldn’t load this view" with "Try again". Live app until the API ships: the nav title and one centred caption, "Your activity shows up here soon.", with no fixture people or amounts. Harness: `-activityFeedHarness <unread|read|empty|loading|error|live>`.

Missing API (file a backend ticket): `GET /v1/me/notifications` (paged, newest first, each item with `id`, `kind`, `title`, `body`, `cabal`, `actor`, `created_at`, `read_at`, an optional status badge and a destination that maps onto the `PushRoute` cases), `POST /v1/me/notifications/read` (all, or up to an item id), and an unread count or `has_unread` flag on an existing Home read for the bell's dot. Read state is local to the model until then.

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

Used by Home's "Needs your vote", the cabal's proposals and [All proposals](#all-proposals-pendingvotesroute) (#612). The design is `docs/design/v3/`; the card has no template of its own, so compare it with Home's Proposals section. It is a `MonacoCard`. Nothing on it casts a vote: members vote on the proposal screen. Every badge, button title, note and footer string comes from `ProposalBadge` and `ProposalCardCopy` in MonacoCore, keyed by `ProposalState`.

| Part | Shows |
| --- | --- |
| Top row | The proposer's avatar and the cabal name (the proposer's name when the caller passes none), and the status badge on the right (`proposal-status-chip`) |
| Instrument row | `StockMark`, "Buy NVIDIA" or "Sell NVIDIA", "$400.00 from the cabal pot" (a sell reads "0.6 shares from the cabal's holding"), and one small button that opens the proposal. At accessibility sizes the button drops below the text |
| Note | One line, "5 yes votes needed · Closes in 8h" while open (`proposal-closes-in`), a sentence about the outcome once closed. A paused cabal adds "Trading is paused. If this passes, it won't buy until trading resumes." (`proposal-paused-caption`) |
| Footer | A ruled row that opens the proposal: an icon, the tally "3 yes · 1 no · 4 not voted" while open or the outcome line, and a chevron |

| State | Badge | Button | Footer |
| --- | --- | --- | --- |
| Open, can vote, no ballot | "Needs your vote" | "Review" (primary) | Tally, muted |
| Open, voted | "You voted Yes" or "You voted No" | "Review vote" | Tally, muted |
| Open, cannot vote | "Voting open" | "Review" | Tally, muted |
| Passed | "Execution pending" (warn) | "View status" | "Approved by the cabal · Not completed", warn |
| Executed | "Executed" | "View trade" | "Trade confirmed · View the original decision", gain |
| Expired | "Expired" | "View decision" | "Voting closed · No trade placed", muted |
| Trade blocked or swap failed | "Trade not completed" (negative) | "View status" | "Decision retained · No completed trade", loss |
| Did not pass | "Not approved" | "View decision" | "Voting closed · No trade placed", muted |
| Withdrawn, voided | "Withdrawn", "Voided by Monaco" | "View decision" | "Voting closed · No trade placed", muted |

The card is one VoiceOver element, "<cabal>: <badge>. Buy NVIDIA, <amount line>. <note>", with the button and the footer as actions.

## Proposal screen (`ProposalRoute`)

| Order | Slot | Owner | Shows |
| --- | --- | --- | --- |
| 1 | `ProposalDetailSlot` | #612 | The sections below, top to bottom |
| 2 | `ProposalCommentsSlot` | #711 | "Discussion", the thread, and a composer pinned at the bottom, "Add a comment". Empty: "No replies yet" / "Start the discussion." |

| Section | Shows |
| --- | --- |
| Hero | `StockMark`, the status badge (`proposal-status-chip`: "Voting open", "You voted Yes", "View only", "Approved · Executing", "Executed", "Expired", "Not executed", "Not approved", "Withdrawn", "Voided by Monaco"), "Buy NVIDIA" or "Sell NVIDIA", the amount (a sell shows its shares and "About $139.00"), the proposer (opens their profile) and "Your cabal · 2h ago". The proposer's return pill is not drawn yet: the API has no return on a proposal |
| Thesis | "Thesis" and the reason as prose (`proposal-reason`). Omitted without a reason |
| Votes | "Votes" with "See all", which opens the votes sheet, then the vote bar: "5 yes votes to pass" (or "Voting closed"), the bar and "3 yes · 1 no · 4 not voted" (`proposal-votes`) |
| Expected | "Cabal gets", "about 0.73 shares at $341.57", while open or passed (`proposal-expected`) |
| Network and price | The chain and, when its live quote is missing, the reason (`proposal-chain-row`), then the best price (`best-price-loading` while it is found), while open or passed |
| Status | Once voting is over: a notice (warn "The vote passed. Execution is pending.", info "The trade is confirmed.", info "Voting closed without enough yes votes. No trade was placed.", error "The trade did not go through." with the failure message, info "The proposal did not pass.", info for withdrawn and voided), the price-moved rows (`proposal-price-moved`), "Retry" or "Buy at current price" when the trade can be retried (`proposal-retry`), and for passed, executed and blocked proposals the "Execution details" disclosure: Status, Executed price ("Not confirmed", or "Opens soon" for an executed trade until the API carries it), Units (executed only, "Opens soon"), Network and the "Transaction record" row that opens `TransactionRoute`. The section reads as the `ProposalStepper` label for VoiceOver (`proposal-tracker`) |
| Withdraw | The proposer sees "Withdraw proposal" while it is open (confirm "Withdraw this proposal?" / "Votes so far are dropped.", toast "Proposal withdrawn.") |

The dock above the tab bar sits over the composer "Add a comment":

| State | Dock |
| --- | --- |
| Open, can vote | "Vote No" (`proposal-vote-no`) and "Vote Yes" (`proposal-vote-yes`). Toast "Vote recorded." |
| Open, voted | "Change vote" (`proposal-change-vote`), which asks "Your current vote: Yes" with "Vote Yes", "Vote No" and Cancel |
| Open, cannot vote | "View-only · Voting is restricted to eligible members" (`proposal-view-only`) |
| Closed | The composer only |

| State | Shows |
| --- | --- |
| Loading | Skeletons for the hero, the title, the amount, a block and three rows |
| Error | "Couldn't load this view", "Your existing information hasn't been changed. Try again when your connection is available." and "Try again" (`proposal-detail-error`) |
| Vote, retry or withdraw failed | An error toast, with the content kept |

### Votes sheet

"See all" under Votes opens `ProposalVotesSheet` at the medium detent, which drags to large and opens at large at accessibility sizes. The header is "Votes" with a close button. A pinned strip reads "3 yes", "1 no", "4 not voted". One row per voter, yes then no then not voted: avatar, name and a "Yes", "No" or "Not voted" badge, opening their profile (`proposal-voter-<id>`). Loading is a strip and four row skeletons. An error reads "Votes couldn't load. Try again." over "Retry". Tallies stay live while the sheet is open.

## All proposals (`PendingVotesRoute`)

Opened from Home's "See all". The title is "All proposals" over an "Open" / "Closed" control (`proposals-segment-open`, `proposals-segment-closed`).

| Segment | Shows |
| --- | --- |
| Open | A [proposal card](#proposal-card) for each proposal that needs or has your vote, then a row for each passed proposal whose trade has not finished: the stock and amount, the status line and a badge ("Pending"), with a chevron. Empty: "No open proposals" / "New ideas will appear here. Closed decisions remain in the cabal history." with "Choose a proposal", which opens the Cabals tab (`pending-votes-empty`) |
| Closed | "Closed proposals show up here soon." The API lists open votes only. Each closed row will carry "Pending", "Expired", "Blocked", "Not approved", "Withdrawn" or "Voided" |

Loading is skeletons ("Loading proposals"). Error is "Couldn't load this view" with "Try again" (`pending-votes-error`). Pull to refresh.

## Propose (#613)

**Propose chooser** (`ProposeRoute(cabalID:)`), pushed from the action row. Title "Propose". Rows: "Buy a stock" / "Your cabal votes on it first"; "Sell something the cabal owns" / "Apple, Nvidia and 1 more" (disabled with "Nothing to sell yet" when there are no holdings).

**Buy, step 1.** Title "Buy". Search "Search Apple, Tesla, NVDA…", then "Popular". Rows show logo, ticker over name, price over the day's change. A stock that cannot trade is dimmed: "<name> · Can't buy right now".

**From a stock.** The asset screen's "Propose buy" skips step 1. When the viewer can vote in more than one cabal it first pushes **Pick a cabal** (nav title "Pick a cabal"): the question "Which cabal should buy GOOGL?" in `title` type, then one row per cabal with its mark and pot. Loading, failed and empty states keep the canvas and the title. With one cabal it goes straight to Amount. With none it shows the sheet "Join a cabal first" with "Find a cabal".

**Amount.** Title "Buy GOOGL" or "Sell GOOGL". The stock row. Amount entry on the in-screen keypad with chips "$25", "$50", "$100", "Max" (Max is the pot), helper "The pot has $500.00". "+ Add a reason" expands a field "Why should the cabal buy this?" (280 characters, the server's cap, with a counter from 180). The system keyboard replaces the keypad while the field has focus, and its "Done" button, or a tap anywhere outside the field (the amount figure too), brings the keypad back. The `GET /v1/cabals/{id}/proposals/preview` result shows inline: over the pot "More than the pot has", no route "Can't buy <name> right now. Try a smaller amount or another stock." Both disable "Review", because the server refuses them ([product.md](product.md#trading)).

**Review.** Title "Review". "Buy $250.00 of GOOGL". Rows: "Cabal gets" "about 0.73 shares", "Price" "about $341.57 a share", "Pot" "50% of $500.00", "Who votes" the cabal. "Why buy" with the reason. Primary "Send to cabal". Success closes the flow with the toast "Proposal sent to Sunday Investors".

**Sell.** Title "Sell", header "What the cabal owns", holdings largest first. Amount in dollars with "25%", "50%", "All" and helper "The cabal holds $278.47"; a holding without a price takes a token quantity field labelled "Shares" (or "Tokens" for pre-IPO). Review: "Sell 0.6017 shares of AAPL", rows "Raises" "about $139.00", "Cabal keeps", "Who votes". Reason header "Why sell".

## Money

**Shared rules (UI v3).** Cream canvas, `Space.gutter` sides, a `ScrollView` with the primary in a form footer, or in the glass `BottomCTA` while a keypad is up. Notices are `MonacoNotice` (info, warn, error). Key rows sit in a `MonacoCard(.inset)` of `ReceiptLine`s. A confirmed result is an `OutcomeView` over a key-row card (`MoneyOutcome`). A full-screen read failure is "Couldn't load this view" with "Try again". Service refusals keep their toasts. No screen shows a success state before the service has confirmed it.

**Deposit** (`DepositRoute`, template `add-money`). Title "Deposit". Eyebrow "Your cash" over the balance (`deposit-screen-balance-value`; "—" and a scoped error row when it cannot load), the header "How would you like to deposit?", then "Card or Apple Pay" (`deposit-card-row`, pushes `CardFundingRoute`) and "Send crypto" (`deposit-crypto-row`). Warn notice "Some funding methods may be unavailable in your location or account state." only after a card session failed to start; info notice "Money arrives in your cash balance. Funding a cabal is a separate step."

**Card / Apple Pay** (`CardFundingRoute`, template `card-funding`, new). Tab bar hidden. Card orb, "Continue securely.", the key rows "Destination", "Amount & fees", "Next step", an info notice, primary "Continue to provider" (`deposit-card-spinner`, "Opening…" while the session starts, opens the provider page) and outline "Choose another method". Back from the provider maps `CardDepositState`: processing is "Checking your deposit" with "View cash", done is "Deposit received", a failed session is "Funding cancelled" with the existing "Card purchase didn't go through." and "Back to funding methods". A failed start stays on the handoff with its toast.

**Choose a network** (`DepositNetworksRoute`, template `network`). Helper, one row per offered network ("Direct USDC deposit" for Solana, the arrival note for others, `deposit-network-<id>`), and an info notice.

**Receive USDC** (`DepositAddressRoute`, template `deposit`). Title "Receive USDC". "USDC on <network>" with a "Deposit" badge, a card with "Your deposit address", the scannable QR on its white tile, the address and "Copy address" (toast "Address copied."), an info notice (`deposit-network-warning`, plus the arrival and fee notes) or, while a deposit is arriving, the warn notice "Waiting for network confirmation. Your balance hasn't been credited yet." (`platform-balance-arriving`), and "How it works" with three numbered steps. When a settled deposit lands while the screen is open it becomes "Deposit received" with the key rows Asset, Network, Destination, Amount, "Fund a cabal" and "Back to Home".

**Deposit to <cabal>** (`FundRoute`, template `fund-cabal`). The cabal cover, name and "Cash $500.00" (with "· $50.00 pending" while money is in flight), a warn notice when the cabal is paused, "Deposit amount" with "$25", "$50", "$100", "Max", the keypad, the error notice "Not enough cash. Reduce the amount or deposit into your account." with an outline "Deposit to account" when over the cash, the warn notice "Processing your deposit…" while submitting, and the yield info notice. Primary "Deposit $250.00" (`fund-cabal-submit-button`), "Enter an amount" at $0.00. After the fund is accepted a glass sheet "Deposit confirmed" shows "$250.00 added to your share in <cabal>" with "Done"; both dismiss and pop back. If the response lands after the member left, the toast stays. Zero cash is "Add money first" with "Add money".

**Cash out your share** (`CashOutRoute`, template `cash-out`). An info notice (`cash-out-explainer`), "Your share in <cabal>" between rules, "Cash-out request" with "25%", "50%", "All", the keypad, error notices when over the share or under the minimum, and the primary "Review cash-out" (`cash-out-review`). Review keeps the amount and shows the key rows Requested amount, Destination, Final proceeds, Costs, a warn notice and "Request cash-out" (`cash-out-submit`, "Requesting…"); back returns to the amount. A started job shows "Cash-out requested" with Status "Processing" and "View activity" (`cash-out-progress`), then "Cash-out complete" with Status "Completed" when it settles, or "Cash-out didn't go through" if it fails. No slice is "Nothing to cash out yet" with "Fund this cabal".

**Withdraw to wallet** (`WithdrawRoute`, template `withdraw`). The helper, "Withdrawal" with "Max" and "$850.03 available", the receive line for EVM networks, "Wallet address" with Paste, the caveat, a loss border and error line for an invalid address, a "Network" menu of every `WithdrawNetwork` (`withdraw-network-picker`) and the primary "Review withdrawal". Over the balance is the error notice "The amount exceeds your available balance." Review is titled "Review withdrawal" with From, Asset, Amount, Network, the full Destination, Arrives and Network costs, a warn notice and "Withdraw" ("Withdrawing…"). An accepted withdrawal shows "Withdrawal processing" with Status "Pending", then "Withdrawal completed" once confirmed, and "View transaction", which opens activity. A balance failure is the full-screen load failure; no balance is "Nothing to withdraw yet" with "Add money".

**Account activity** (`AccountActivityRoute`, #2138, UI v3). Inline title and root title "Account activity", then an All / Pending segmented control (All is the default every time the screen opens). `GET /v1/me/txns`, paged. Rows are a 44pt glyph tile, a title ("Deposit received", "Withdrawal", "Deposit to <cabal>", "Cashed out of <cabal>"), a subtitle ("USDC · <date>", "From cash · <date>", "To cash · <date>") and the signed amount with a chevron; a pending row shows a warn "Pending" badge in place of the amount and a failed row a negative "Failed" badge. Pending filters the loaded pages on the client, keeps loading while nothing matches and more pages remain, and then shows "No pending transfers" / "Transfers that are still processing show up here." A closing info notice says "This is your personal account history. Cabal trades are viewed through their cabal and proposal records." Empty: "No transactions yet" / "Personal deposits, deposits, redemptions and withdrawals will appear here." Loading is the generic skeleton and a failed first load is the full-screen load failure with "Try again". A row opens the transaction receipt.

**Transaction receipt** (`AccountTxnReceiptRoute`, `ReceiptView`). Inline title "Transaction receipt". `ReceiptView` is the one receipt layout, fed by the MonacoCore `ReceiptContent` value; the cabal trade receipt (`TransactionRoute`, Lane B) adopts it from its side. A centred 54pt mark, the eyebrow (the row title, or "Bought NVIDIA" for a trade), the amount (`account-txn-receipt-amount`) and a status badge: Completed, Processing (trade: Execution pending), Not completed or Status unconfirmed. A warn or error notice explains pending, failed and unknown. The key-row card lists From, To, Asset, Requested amount, When, Status (`account-txn-receipt-status`), Record ID and, for a deposit to or cash out of a cabal, a Cabal row that opens the cabal. Withdrawal destinations read "External wallet" until `GET /v1/me/txns` returns a masked address. Below the card, "Account activity" returns to the transfer history, and "View on explorer" opens the Solscan page in Safari, or is disabled with "Requires a confirmed transaction ID" until the transfer has a signature. A harness-only error state shows the load failure with "Try again".

**Cabal cash** (`GroupCashRoute`, `GroupCashView`). Inline title "Cabal cash". Reads `GET /v1/cabals/{id}/pot`. The eyebrow "<cabal> · Cash", the cash figure and "Unallocated USDC in the cabal pot"; an "Idle cash" card with Asset, Cash in the cabal and Yield status; an info notice, "Current yield details are not available. The provider, rate and applicable terms will appear when confirmed."; then "Use cabal cash" with "Propose an investment" (hidden when the viewer cannot propose) and "Return to holdings". Yield status is "Rate requires live confirmation" today and never the asserted `CabalYield.apy`; it shows a rate only once the pot read carries one, and "Rate unavailable" in a warn notice when it reports cash without yield information. Missing API: the provider, rate and terms of the yield on a cabal's idle USDC. Until Lane B wires the holdings Cash row to `GroupCashRoute(cabalID:cabalName:)`, the screen is reachable only from `-groupCashHarness`.

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
