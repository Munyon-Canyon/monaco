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
       ├─ bell ────────────────────────▶ ActivityFeedRoute
       ├─ Net worth / Cash ────────────▶ BalancesRoute
       ├─ Deposit ─────────────────────▶ Deposit half sheet
       ├─ Proposals card ──────────────▶ ProposalRoute
       ├─ Proposals "See all" ─────────▶ PendingVotesRoute
       ├─ Your cabals card ────────────▶ CabalRoute
       ├─ Leaderboard row ─────────────▶ CabalRoute | UserProfileRoute
       ├─ Leaderboard "View all" ──────▶ LeaderboardRoute
       └─ Market updates row ──────────▶ AssetRoute
Feed ──┬─ item ──▶ ProposalRoute | TransactionRoute | AssetRoute | CabalRoute | FeedItemRoute
       └─ bell ──▶ ActivityFeedRoute
Cabals ┬─ + ──▶ Start a cabal ──▶ create ──▶ CabalRoute
       ├─ empty state "Explore cabals" | "Search public cabals" link | keyboard Search ──▶ DiscoverCabalsRoute ──▶ CabalRoute
       ├─ card / leaderboard row ──▶ CabalRoute
CabalRoute ─┬─ non-member ──▶ Public cabal (join, request, accept an invite)
            ├─ i, Rules & members ──▶ CabalInfoRoute ─┬─ Members & voters ──▶ MembersRoute ──▶ UserProfileRoute
            │                                         ├─ Invite ──▶ Invite to cabal sheet
            │                                         └─ Edit ──▶ Cabal settings
            ├─ share ──▶ iOS system share sheet (cabal card preview and invite link), members only
            ├─ Deposit ────▶ FundRoute
            ├─ Propose ────▶ action sheet ──▶ ProposeRoute ──▶ Buy | Sell ──▶ Amount ──▶ Review sheet
            ├─ Cash out ───▶ CashOutRoute
            ├─ Chat preview ▶ ChatRoute
            ├─ proposal ───▶ ProposalRoute
            ├─ holding ────▶ CabalHoldingRoute
            ├─ Cash row ───▶ GroupCashRoute
            └─ activity row ▶ TransactionRoute
Markets ┬─ row / mover tile ──▶ AssetRoute ── Propose buy ──▶ Pick a cabal ──▶ Amount ──▶ Review
        └─ Manage / Add ──────▶ WatchlistRoute ── row ──▶ AssetRoute
Profile ┬─ Followers / Following ──▶ FollowListRoute
        ├─ Name block ──────▶ EditProfileRoute
        ├─ Net worth / Cash ▶ BalancesRoute
        ├─ Account activity ▶ AccountActivityRoute
        ├─ Invite friends ──▶ InviteRoute
        ├─ Find friends ────▶ Find friends (#663, search from #2142)
        ├─ View as others ──▶ UserProfileRoute (your own)
        ├─ Withdraw ────────▶ WithdrawRoute
        └─ Settings round ──▶ SettingsRoute ─┬─ Edit profile ──▶ EditProfileRoute
                                             ├─ Notifications
                                             ├─ Activity ──▶ AccountActivityRoute
                                             ├─ Withdraw ──▶ WithdrawRoute
                                             ├─ Blocked people
                                             ├─ Advanced
                                             ├─ Delete account ──▶ DeleteAccountRoute
                                             └─ Sign out
```

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
| 3 | `HomeBalanceSlot` | The money strip under a hairline: Net worth (cabal shares plus cash, summed in MonacoCore) and Cash from `GET /v1/me/balance`, each a button that opens Your money (`BalancesRoute()`), and a compact "Deposit" capsule (opens the Deposit half sheet). The card-deposit "arriving" and pending lines stay under it in the warning colour. Balance failed: "—" and an error row. Hidden when there is no cabal and no cash. At accessibility sizes the strip stacks |
| 4 | `HomePendingVotesSlot` | "Proposals" with a count pill of the cards that need the viewer and "See all" (`PendingVotesRoute()`). Up to two decision cards, needs-your-vote first, then trades in progress. Nothing pending: "No proposals need your attention." and "Closed decisions remain in proposal history.". Loading: one skeleton card. Failed: a card "Proposals couldn't load" with "Try again". Hidden when there are no positions |
| 5 | `HomeCabalsSlot` | "Your cabals" and "See all" (the Cabals tab). A horizontal rail of 300pt `CabalCoverCard`s, one per cabal you belong to (`GET /v1/me/cabals`, read in parallel with `GET /v1/me/portfolio`, whose value, P&L and return are joined by cabal id): tint and picture cover, name, member count, a "1 vote needed" badge, your share, your ownership of the pot, your all-time P&L or "Not funded", and a share disc. Funded cabals come first by value, then cabals you have not funded (or fully cashed out of) by most recent join; an unfunded card reads "$0.00", "0.00%" and "Not funded" and has no vote badge. If the memberships read fails the rail shows the portfolio rows, and a failed read never shows the empty state. A card opens `CabalRoute`, whose Fund action funds it. No memberships at all: "Start with your people" / "Create a cabal or open an invitation from a friend." with "Create a cabal" and an outline "Explore cabals" that selects the Cabals tab |
| 6 | `HomeLeaderboardSlot` | "Leaderboard" and "View all" (opens `LeaderboardRoute` on the card's board and range). A Cabals / People menu select and a 1W / 1M / All menu select (default Cabals, 1M), the line "Cabal P&L · 1M" or "Personal P&L · 1M", and the top three `BoardRow`s. The rank-change glyph and "Rank change" stay hidden until the backend serves rank movement. Empty: "No investors yet" / "Fund a cabal to get on the board." or "No cabals ranked yet" / "Public cabals show up here once they're funded." |
| 7 | `HomeMarketUpdatesSlot` | "Market updates" and "Markets" (the Markets tab). The largest 24 hour mover in the catalogue ("NVIDIA · +4.18%", "24h token move", opens `AssetRoute`) and a static "News" row, "News opens soon.", until there is a news source. Hidden when there is no cabal and no cash |

## Your money (`BalancesRoute`)

Opens from the Home money strip (the "Net worth" and "Cash" metrics). A pushed screen with the inline title "Your money", the tab bar visible and pull to refresh, which reloads both reads. `BalancesModel` composes `GET /v1/me/portfolio` and `GET /v1/me/balance`; net worth is the portfolio total plus available cash, summed in `Int64` micros in MonacoCore.

| Part | Shows |
| --- | --- |
| Performance | The eyebrow "Net worth", the figure in `moneyFont(.hero)` and the helper "Your cabal shares + cash" |
| Two stats | "In cabals" and "Cash" between hairlines in `moneyFont(.card)`. At accessibility sizes the columns stack |
| Actions | Three `CircleAction` tiles: "Deposit" (opens the Deposit half sheet), "Withdraw" (`WithdrawRoute()`) and "Activity" (`AccountActivityRoute()`) |
| Your allocation | One row per portfolio cabal, in portfolio order: a 40pt circular `CabalMark`, the name, "Your share", the value and a chevron (`CabalRoute`). The Cash row is always last: a wallet glyph, "Cash", "Not allocated to a cabal", the cash value (opens the Deposit half sheet) |
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

Large title "Cabals" (`Typo.display`) with the glass "+" on the right ("Create a cabal", `cabals-new-button`, hidden under review). The "+" pushes **Start a cabal** (`CreateCabalRoute`) directly, with no sheet in between. Joining a cabal goes through Discover cabals (below) and the cabal screen's "Join" or "Request to join". Pull to refresh reloads every slot.

| Order | Slot | Owner | Shows |
| --- | --- | --- | --- |
| 1 | `CabalsJoinSlot` | #646 | The title row and the field "Find or search cabals" (`cabals-filter-field`). As you type it filters your own cabals by name (case-insensitive contains) and the "Your cabals" count follows the matches. The keyboard Search key pushes Discover cabals with the typed text, so the field reaches every public cabal (hidden under review) |
| 2 | `CabalsInvitesSlot` | #696 | "Cabal invites" with a count: a row per invite with the cabal tile, name, the inviter, and small "Decline" and "Accept" buttons. Accept opens `CabalRoute`. Hidden when empty |
| 3 | `CabalsListSlot` | #606 | "Your cabals" with a count, then one full-width `CabalCoverCard` per cabal (`cabals-list-card-<id>`): the tint gradient and uploaded picture under a scrim, the name over "8 members", then "Your share" (`MoneyText`) and "Ownership" (for example "35.00%"), then "Your P&L · All time" with the signed amount and percent . Share, ownership and P&L are joined from `GET /v1/me/portfolio` by cabal id. A round share button opens the invite share (no figures), and the footer reads "Cabal · 1M" with a chevron; the whole card opens `CabalRoute`. A badge "1 vote needed" / "2 votes needed" shows at the top right when the count is above zero and the viewer has a funded share. The count comes from the existing `GET /v1/me/pending-votes` read, grouped by cabal id. Unread and request counts are not drawn; the card's VoiceOver value reads them out ("3 unread", "2 requests"). No share: "—" and "Not funded". No return yet: "—" and "Unavailable". The cabal's own 1M return shows "— Return opens soon" until the API ships it. Loading: a card skeleton labelled "Loading your cabals" (`cabals-list-loading`). Failed: "Couldn't load this view" / "Your existing information hasn't been changed. Try again when your connection is available." with "Try again"; a failed refresh over cards shows the error toast instead. Empty (`cabals-list-empty`): "Your cabal starts here" / "Create a cabal or open a shared invitation." with a primary "Create a cabal" (Start a cabal) and an outline "Explore cabals" (Discover cabals). Under review: "Open a shared invitation to join a cabal." with no buttons. Under the cards (or under the no-match state) a text link is always visible: "Search public cabals", or "Search public cabals for “<text>”" while the field holds text (`cabals-search-public`); it pushes Discover cabals with the typed text and is hidden under review. A filter with no match: "No matching cabals" / "Try another name." |
| 4 | `CabalsBoardSlot` | #699 | "Cabal leaderboard" (`cabals-board`) with "Public" on the right, under a hairline. A tools row: "Cabals", the period menu "Leaderboard period" (`cabals-board-period`) and a round filter button "Filter and sort cabal leaderboard" (`cabals-board-filter`). The context line reads "Cabal P&L · 1M" (the period follows the menu) beside a "Rank change" button that opens the sheet "About this ranking". Rows (`cabals-board-row-<id>`): rank with its movement (↑n, ↓n, – or "New"), tile, name, signed P&L over signed percent, chevron; each opens `CabalRoute`. Three rows show first, "Load more rankings" reveals three more ("Loading more rankings…" while it fetches), and "All <n> eligible cabals shown" ends the list. Loading: three row skeletons labelled "Loading rankings". Empty: "No public rankings for this period." Failed: "Rankings couldn't load." with "Retry rankings". The filter sheet "Leaderboard filters" holds three selects ("Sort by": Return % / P&L amount / Name A–Z; "Membership": All / Accepting requests / Open / Approval; "Minimum members": Any cabal size / 5+ members / 10+ members) with "Apply filters" and "Reset". Closing without Apply drops the draft, and the filter button is highlighted while any filter differs from its default. The API has no filters yet, so the selects are disabled and the sheet shows the info notice "Filters open soon. Rankings use return % for now." Member counts under the row names and the 3M period wait on the API too |

**Discover cabals** (`DiscoverCabalsRoute(query:)`, `DiscoverCabalsView`). Pushed from "Explore cabals", "Search public cabals" or the keyboard Search key on the Cabals tab, with the nav title "Discover cabals" and the tab bar still showing. Large title "Find a cabal", then the search field "Search cabals" (`cabals-search-field`), prefilled from `query`. Results update after a short pause and replace everything below. Rows (`cabals-search-result-<id>`): tile, name, "3 members · Open" or "· Approval required", a chevron, and on the right "Join" (Open) or "Request" (Approval required) (`cabals-search-enter-<id>`), the warn badge "Request pending" (`cabals-search-requested`) or the badge "Joined" (`cabals-search-joined`). A row tap opens `CabalRoute`, which shows the public cabal to a non-member. While a request is pending, a warn notice "Your request to join <name> is pending." sits above the list. No match (`cabals-search-empty`): "No matching cabals" / "Try another name." Loading: a block and four row skeletons (`cabals-search-loading`). Failed: "Couldn't load this view" with "Try again" (`cabals-search-error`); join and request failures use the error toast. Empty query: the public cabal board's rows (tile, name, chevron) and the caption "Browsing every public cabal opens soon. Search by name to find more." (`cabals-browse-coming`), because the API has no list of every public cabal. Harness: `-MonacoDiscoverSample <results|requestPending|noResults|loading|error|browse>`.

**Start a cabal** (#606, `CreateCabalRoute`). Nav title "Start a cabal", the tab bar still showing, and a primary "Create cabal" pinned above it ("Creating…" while it runs, `create-group-submit`). While creating, the top shows "Creating your cabal" / "Saving the cabal name and decision rules." and every input is disabled. Then:

- "Cabal name" (`create-group-name`, placeholder "A name your friends will recognize"), with the hint "Pick a name your friends will recognize." underneath, or the red problem line (`create-group-name-problem`) for a name that is too short, too long or has line breaks.
- "Cover image" (`create-cover-picker`): a 110pt preview, "Add a cover photo" before a pick and the photo with a check once picked, "Choose a cover your cabal will recognize." underneath, and "Remove photo" in its context menu. Create takes no cover, so the photo uploads after the cabal exists. If that upload fails the cabal still opens, with the toast "Cabal created. The cover didn't upload. Add it from Cabal details."
- Three rule blocks, each a title, a fixed helper line and a segmented control: "Who can join" (`create-rule-join`, "Let people join, or review their requests.": "Open" / "Approval required", with "Approval required" preselected), "Who votes" (`create-rule-voters`, "Choose who participates in cabal decisions.": "Everyone" / "People I pick") and "Votes stay open" (`create-rule-expiry`, "A proposal that hasn't passed closes at this deadline.": "1 hour" / "1 day" / "1 week", with "1 week" preselected; the three `proposal_expiry_seconds` values in [cabals.md](architecture/cabals.md#rules)). With "People I pick" a static row follows (`create-rule-voters-pick`): "Choose voters" / "Only you vote until you add people in Cabal details." Create takes no voters, so they are chosen in Cabal details once the cabal exists. There is no "To pass" block, because the threshold is fixed.
- An info notice with the passing rule in force (`create-group-threshold-notice`), from `form.threshold.caption`: "Passes once more than half the voters say yes."

Create then pushes `CabalRoute` with the toast "Cabal created.". A failed create keeps the form and shows the error notice "Couldn't create your cabal. Your choices are still here." (`create-group-failed`) above the button, plus the error toast; the notice clears on the next edit.

## Leaderboard (`LeaderboardRoute`)

Pushed as `AnyAppRoute` from any tab; `LeaderboardRoute(board:range:)` defaults to Cabals and 1M. Home's "View all" opens it. The page shows one board at a time, built from `BoardRow` and a `LeaderboardPageModel` that owns one `LeaderboardLoader` per board, so switching back does not reload.

| # | Section | What it shows |
| --- | --- | --- |
| 1 | Tools (`leaderboard-category`, `leaderboard-period`, `leaderboard-filter`) | Category menu "Leaderboard category" (Cabals / People), period menu "Leaderboard period" (1H / 1D / 1W / 1M / All; 3M waits on the API) and a round filter button "Filter and sort leaderboard" with a brand dot and the value "Filters on" while any filter differs from its default |
| 2 | Context line | "Cabal P&L · 1M · Rank change vs previous snapshot" or "Shared personal P&L · 1M · …". Live data has no snapshot movement, so the tail reads "Rank change opens soon". The info button "About this ranking" (`leaderboard-info`) opens a sheet with two paragraphs |
| 3 | Rows (`leaderboard-row-<id>`) | Rank with ↑n / ↓n / – / "New" under it when the row carries movement, 36pt mark, name, "12 members" or "@handle", signed P&L over signed percent, chevron. Cabals open `CabalRoute`, people `UserProfileRoute`. Under a sort other than "Return %" the movement cell shows – and VoiceOver says "Rank movement not compared for this sort". Reaching the last row loads the next page, and "All <n> eligible cabals shown" (or "people") ends the list |

States and exact copy:

- Loading: tools and context stay; six row skeletons.
- Empty: "No eligible rankings" / "No public data for this period." People you follow: "Follow people to see how they do." with "Find friends".
- Filtered empty: "No matching rankings" / "Try removing a filter." with the outline "Reset filters".
- Stale (every row's prices delayed): the warn notice "Rankings paused while valuations update." and no rows.
- First load failed: the tools hide; "Couldn’t load this view" / "Your existing information hasn’t been changed. Try again when your connection is available." with "Try again" (`leaderboard-error`).
- A refresh or switch that fails with rows on screen shows the loader's error toast.

The filter sheet "Leaderboard filters" holds "Sort by" (Return % / P&L amount / Name A–Z), "Membership" (All / Accepting requests / Open / Approval) and "Minimum members" (Any cabal size / 5+ members / 10+ members) for cabals, and "Show" (Everyone / People you follow) for people, with "Apply filters" and "Reset". Closing without Apply drops the draft. Live, Apply sends `sort` to both boards and `membership` and `min_members` to the cabals board, and "Show" sends `filter=friends`. The board then holds the matching rows in that order, ranked from 1.

Missing APIs: rank movement against the previous snapshot, a cabal member count on a board row and the 3M range. The sample harness (`-MonacoLeaderboardSample <cabals|people|filtered|empty|loading|stale|error>`) drives all of it from fixtures.

## Cabal screen (`CabalRoute`)

Toolbar: the system back button, the cabal name centred and always shown, and two glass rounds on the right: share (`cabal-share-button`, "Share cabal", opens the iOS system share sheet in one tap) and information (`cabal-details-button`, "Cabal information"), which pushes `CabalInfoRoute`. Members only; a non-member gets the Public cabal screen below. While loading: a hero skeleton, the pot figure and chart blocks and three action circles. Load error: the full-screen "Couldn’t load this view" with "Try again" (`cabal-failed`). The slots sit on the cream canvas, a hero first and then rules and tiles, with no ink band.

| Order | Slot | Member | Non-member | Shows |
| --- | --- | --- | --- | --- |
| 1 | `CabalHeaderSlot` | yes | | Cover hero (`CabalCover`: the cabal tint, its picture under the scrim): name, "8 members" and a share disc (`cabal-hero-share`) that opens the iOS system share sheet in one tap; on Cabal information it opens the invite sheet. The creator taps the cover to change the picture or long-presses it to remove it |
| 2 | `CabalClosedSlot` | yes | | Error notice when `status` is `banned`: "Monaco closed this cabal. Everyone's slice is being cashed out to their account balance." A banned cabal also hides the actions and the chat preview |
| 3 | `CabalPotSlot` | yes | | The pot value in `moneyFont(.hero)`. The "Prices delayed" notice stays hidden until the API sends a valuation time |
| 4 | `CabalValueChartSlot` | yes | | The return line (signed percent and "past month"), the small `CurveScrubChart` (crosshair, pinch, tooltip pill), start and end dates and the periods `1W 1M All` (default 1M). A cash-only pot shows the quiet chart "No positions yet" and no periods; short history shows "Not enough history for the last month yet" |
| 5 | `CabalSliceSlot` | yes | | Two stats between hairlines: "Your share" (`$0.00` with no stake, `cabal-slice-none`) and "Cash". They stack at accessibility sizes |
| 6 | `CabalPauseSlot` | yes | | Warning notice while the cabal is paused |
| 7 | `CabalJoinSlot` | creator only | fallback | Creator: one row "Join requests" / "2 requests to review" that opens Members & voters on Requests (`cabal-join-requests-heading`). Non-member fallback: the join, pending, declined and invite buttons used by the Public cabal screen |
| 8 | `CabalActionsSlot` | yes | | Three soft tiles: "Deposit" (`FundRoute`), "Propose" and "Cash out". Cash out is disabled with no share and an info notice says so; Propose is disabled for a non-voter with "Only voters can propose" |
| 9 | `CabalChatPreviewSlot` | yes | | "Chat" with "Open chat". A card holds the newest message, its author and age, "Cabal name · 3 new messages" and an unread pill. No messages: "No messages yet". A failed read leaves an "Open chat" row |
| 10 | `CabalProposalsSlot` | yes | read-only | "Proposals" with a count and "See all", then the proposal cards. Empty: "No proposals need your attention." with "Closed decisions remain in proposal history." and, for a voter, "Propose the first buy." Hidden on a cash-only cabal with no proposals. A failed read shows a partial-error card. Settlement toasts are unchanged |
| 11 | `CabalHoldingsSlot` | yes | | "Holdings": the allocation bar and legend, one row per holding (opens `CabalHoldingRoute`) and a "Cash" row (opens `GroupCashRoute`). Empty pot copy is unchanged |
| 12 | `CabalMemberBoardSlot` | yes | | "Member standings" with "Within Big Ballers · Share return · 1M", compact rows (rank, avatar, name, signed return, "—" without a stake) and the viewer's row washed with a "You" badge. Hidden on a cash-only cabal |
| 13 | `CabalActivitySlot` | yes | | "Group activity" with "See all" past five rows and the helper "Decisions and transactions in this pot". Hidden on a cash-only cabal while empty |
| 14 | `CabalRulesRowSlot` | yes | | A "Rules & members" row that pushes Cabal information (`cabal-rules-row`) |

The trading-bot slot and its route are not on the screen: the agent feature was removed in #5493.

**Invite to cabal sheet** (`CabalInviteSheet`). Opens from Cabal information ("Invite to this cabal"). Title "Invite to cabal", one half-height detent (0.55) with no expand. It shows a cover, "Share a link with your friends.", the link box with "Cabal links open soon" under it while the link is the viewer's own invite link, "Share link" (`share-card-share`, opens the iOS system share sheet) and "Copy link" (`share-card-copy`, toast "Link copied."), then rows "Invite by handle" (the handle form) and "Share your gain as an image" (opens the iOS system share sheet with the gain card as the preview). The invitee side of the sheet reads "You’re invited", "Request sent" or "Invitation unavailable".

**System share (cabal, profile, feed).** Every Share button (the cabal toolbar and hero disc, the cabal cover disc on Home and Profile, the Profile identity round and a feed item) opens the iOS share sheet directly, with no Monaco sheet before it, at a half-height detent. The button shows a spinner while the app loads the card data and the invite link. The sheet gets one link: the sharer's invite link (`/r/<code>`), or the website link when the invite link is not available. The preview is the 4:5 card, a dark image of the all-time gain and return, the gain curve, three stats and the sharer's invite link, drawn by `ShareCardRenderer`; a feed item has no card and previews its headline. If the card data cannot load, the toast "Couldn’t open sharing. Try again." shows and nothing opens.

## Public cabal (`CabalRoute`, non-member)

Title is the cabal name. A tall cover hero with the name and member count, a return block ("—" with "Returns open soon" until the API exposes a non-member return), a "Membership" card (Joining, Voting, Voting window) and one call to action: "Join", "Request to join", a disabled "Request pending" with "Cancel request", "Ask again" after a denial, "Accept invite" with "Decline", or "Open your cabal". The private state ("This cabal is private" with "Back") is built and unreachable until the API reports a private cabal. The About block stays hidden until a cabal has a description.

## Cabal information (`CabalInfoRoute`)

Pushed from the toolbar information button and the Rules & members row. Title "Cabal information". A short cover ("You’re a member" or "You’re an administrator"), "The rules" (a card with Who can join, Who votes, Votes stay open and Current threshold, and an "Edit" link for the creator that opens Cabal settings), rows "Members & voters", "Invite to this cabal" and, for the creator, "Join requests", then "Cabal treasury" (the warning, the address, "View on Solscan"), a "Saving your changes…" notice while a save runs, an outline "View your share" (members with a share) and the destructive-outline "Leave cabal" with its confirm dialog. The chat screen’s details sheet reuses the same sections.

## Members & voters (`MembersRoute`)

Title "Members & voters" with the segments Members, Voters and, for the creator, Requests. Members: "8 members · 8 eligible voters" and a row per member with "Admin" or "Member", opening the profile. Voters: a "Voter" or "Not voting" badge per member, and for a creator who picks voters a "Choose voters" button that opens the "Eligible voters" sheet (tick rows, a pinned "Save voters", toast "Voters saved."). Requests: an info notice when voters are picked, a row per request with a "Review" button that opens a dialog (Approve, Approve and let them vote, Deny, Cancel), and the empty state "No pending requests".

### Proposal card

Used by Home's "Needs your vote", the cabal's proposals and [All proposals](#all-proposals-pendingvotesroute) (#612). The design is `docs/design/v3/`; the card has no template of its own, so compare it with Home's Proposals section. It is a `MonacoCard`. Nothing on it casts a vote: members vote on the proposal screen. Every badge, button title, note and footer string comes from `ProposalBadge` and `ProposalCardCopy` in MonacoCore, keyed by `ProposalState`.

| Part | Shows |
| --- | --- |
| Top row | The proposer's avatar and the cabal name from `cabal_name` (the proposer's name when it is empty), and the status badge on the right (`proposal-status-chip`) |
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
| 2 | `ProposalCommentsSlot` | #711 | "Discussion", the thread, and the only thing pinned at the bottom, the composer "Add a comment". Empty: "No replies yet" / "Start the discussion." |

| Section | Shows |
| --- | --- |
| Hero | `StockMark`, the status badge (`proposal-status-chip`: "Voting open", "You voted Yes", "View only", "Approved · Executing", "Executed", "Expired", "Not executed", "Not approved", "Withdrawn", "Voided by Monaco"), "Buy NVIDIA" or "Sell NVIDIA", the amount (a sell shows its shares and "About $139.00"), the proposer (opens their profile) with their one-month return pill ("+8.2% · 1M", or "P&L private" when they hide it; no pill for an unranked proposer) from `proposer_return_1m_bps` and `proposer_return_private`, and the cabal's name with the age ("Big Ballers · 2h ago", from `cabal_name`) |
| Thesis | "Thesis" and the reason as prose (`proposal-reason`). Omitted without a reason |
| Votes | "Votes" with "See all", which opens the votes sheet, then the vote bar: "5 yes votes to pass" (or "Voting closed"), the bar and "3 yes · 1 no · 4 not voted" (`proposal-votes`) |
| Your vote | `ProposalVoteSection`, right after Votes and before Discussion; it scrolls with the content. Header "Your vote" (`proposal-vote`) and a caption "2 of 5 yes votes · Closes in 8h" (`proposal-vote-progress`). States are in the table below |
| Expected | "Cabal gets", "about 0.73 shares at $341.57", while open or passed (`proposal-expected`) |
| Network and price | The chain and, when its live quote is missing, the reason (`proposal-chain-row`), then the best price (`best-price-loading` while it is found), while open or passed |
| Status | Once voting is over: a notice (warn "The vote passed. Execution is pending.", info "The trade is confirmed.", info "Voting closed without enough yes votes. No trade was placed.", error "The trade did not go through." with the failure message, info "The proposal did not pass.", info for withdrawn and voided), the price-moved rows (`proposal-price-moved`), "Retry" or "Buy at current price" when the trade can be retried (`proposal-retry`), and for passed, executed and blocked proposals the "Execution details" disclosure: Status, Executed price (`price_micros` of the linked swap, "Not confirmed" before the trade confirms), Units (executed only, `token_amount` over `token_decimals`; both read "Not available" when the swap carries no figures), Network and the "Transaction record" row that opens `TransactionRoute`. The section reads as the `ProposalStepper` label for VoiceOver (`proposal-tracker`) |
| Withdraw | The proposer sees "Withdraw proposal" while it is open (confirm "Withdraw this proposal?" / "Votes so far are dropped.", toast "Proposal withdrawn.") |

The "Your vote" section holds the vote; nothing but the composer floats at the bottom:

| State | Your vote |
| --- | --- |
| Open, can vote | "Vote Yes" (`proposal-vote-yes`) and "Vote No" (`proposal-vote-no`) side by side at equal width, stacked at accessibility sizes. Toast "Vote recorded." Both disable while a vote is being sent |
| Open, voted | "You voted Yes" (`proposal-voted`) over "Change vote" (`proposal-change-vote`), which asks "Your current vote: Yes" with "Vote Yes", "Vote No" and Cancel |
| Open, cannot vote | "View-only · Voting is restricted to eligible members" (`proposal-view-only`) |
| Closed (passed, executed, expired, not approved, blocked, withdrawn, voided) | Header "Result" and one sentence (`proposal-vote-result`), "You voted Yes. The proposal did not pass." when the viewer voted. No buttons or caption |

| State | Shows |
| --- | --- |
| Loading | Skeletons for the hero, the title, the amount, a block and three rows |
| Error | "Couldn't load this view", "Your existing information hasn't been changed. Try again when your connection is available." and "Try again" (`proposal-detail-error`) |
| Vote, retry or withdraw failed | An error toast, with the content kept |

### Votes sheet

"See all" under Votes opens `ProposalVotesSheet` at one half-height detent (0.55) that does not expand; the list scrolls inside it, also at accessibility sizes. The header is "Votes" with a close button. A pinned strip reads "3 yes", "1 no", "4 not voted". One row per voter, yes then no then not voted: avatar, name and a "Yes", "No" or "Not voted" badge, opening their profile (`proposal-voter-<id>`). Loading is a strip and four row skeletons. An error reads "Votes couldn't load. Try again." over "Retry". Tallies stay live while the sheet is open.

## All proposals (`PendingVotesRoute`)

Opened from Home's "See all". The title is "All proposals" over an "Open" / "Closed" control (`proposals-segment-open`, `proposals-segment-closed`).

| Segment | Shows |
| --- | --- |
| Open | A [proposal card](#proposal-card) for each proposal that needs or has your vote, then a row for each passed proposal whose trade has not finished: the stock and amount, the status line and a badge ("Pending"), with a chevron. Empty: "No open proposals" / "New ideas will appear here. Closed decisions remain in the cabal history." with "Choose a proposal", which opens the Cabals tab (`pending-votes-empty`) |
| Closed | `GET /v1/me/proposals` (default filter `closed`), read when the segment first opens and paged as the last row appears. Each row carries "Pending", "Expired", "Blocked", "Not approved", "Withdrawn" or "Voided". Loading: four row skeletons (`Loading closed proposals`). Failed: "Couldn't load this view" with "Try again" (`closed-proposals-error`). Empty: "No closed proposals yet." (`closed-proposals-empty`) |

Loading is skeletons ("Loading proposals"). Error is "Couldn't load this view" with "Try again" (`pending-votes-error`). Pull to refresh.

## Propose (#613)

Every propose screen sits on the v3 parts (`MonacoRow`, `MonacoCard`, `MonacoNotice`, `EmptyState`, `AmountEntry`, `MonacoTextField`, the glass `BottomCTA`), and each state has a `-MonacoProposeSample` scenario.

**Propose chooser** (`.proposeChooser(isPresented:cabalID:)`, `ProposeChooserSheet`). A fitted sheet over the cabal screen, no drag indicator: the title "Propose" in muted type, then choice rows centred in `brand`: "Buy stock", "Sell a holding" and "Add a trading bot", and an outline "Cancel". "Sell a holding" is disabled with "No holdings to sell" when the cabal holds nothing sellable; while holdings load it is redacted (`propose-kind-sell-loading`), and a failed read shows the holdings error notice. "Add a trading bot" is disabled with "Trading bots open soon." until #691 flips `ProposeChooserConfig.isBotProposalLive`. A viewer who is not a member sees "Join this cabal first" / "Membership is required to propose." instead of the choices, and a member who cannot vote sees "Only voters can propose". At accessibility text sizes the sheet is `.large`. The cabal screen's Propose tile (`cabal-action-propose`) and the empty proposals section's "Propose the first buy." (`cabal-proposals-propose-first`) present it. Choosing a row dismisses the sheet and pushes `ProposeRoute(cabalID:, start:)`; Cancel only dismisses it.

**Pushed chooser** (`ProposeRoute(cabalID:)` with no `start`, identifier `propose-chooser`). The same choices as `MonacoRow`s with a `SunkenGlyphMark`: "Buy a stock" / "Choose an investment and an amount", "Sell a holding" / "Choose from the cabal’s holdings" (or "No holdings to sell"), and under "Trading bot" the disabled "Add a trading bot" / "Trading bots open soon.". Only the cash screen's "Propose an investment" button and deep links still open it; the cabal screen's Propose tile and the proposals section's "Propose the first buy." present the sheet above. `start` is `.buy` for the stock list and `.sell` for the holdings list, so `closeProposeFlow` still closes the whole flow.

**Buy stock** (`ProposeBuyStockView`). Title "Buy stock". Search "Search stocks or tickers", then the Sort menu ("Volume", "Name A–Z", "Price change", `propose-select-sort`) and the Filter menu ("All", "Popular", "Pre-IPO", `propose-select-filter`), which apply at once. The cabal’s name sits above the rows as a caption. Rows show the stock mark, the name and the ticker, with no price; a paused stock keeps its tag. Empty search: "No stocks found" / "Try another search or filter.". Loading shows a skeleton with no search or menus, and a failed load shows the generic error with "Try again".

**Sell a holding** (`ProposeSellView`). Title "Sell a holding", search "Search holdings", Sort ("Volume", "Name A–Z") and Filter ("All", "Pre-IPO"). Rows read the name over "NVDA · 24.4444 units". A cabal with no stock positions shows "No holdings to sell" / "This cabal has no stock positions." and "Back to cabal".

**Choose a cabal** (`ProposeFromAssetRoute`). Reached from an asset’s "Propose a buy" or "Propose sell". With several eligible cabals a half sheet titled "Choose a cabal" (`propose-pick-cabal-sheet`) opens over the screen. It never expands to full screen. It asks "Which cabal should buy NVDA?" (`propose-pick-cabal-question`) over one row per cabal with its cash. Picking a cabal closes the sheet and shows the amount screen; closing the sheet without a pick goes back to the stock. One cabal goes straight to the amount screen; none shows "Join a cabal first" and "Find a cabal".

**Amount.** Title "Buy <name>" or "Sell <name>". The asset header (stock mark, name, ticker) is `propose-amount-operation`. A context card shows the cabal’s mark and name over "$5,100.00 cash" (buy) or "24.4444 units held" (sell). The amount box is the eyebrow "Buy amount" / "Sale amount" over the figure on the system decimal pad, with chips "$25", "$50", "$100", "Max" (sell: "25%", "50%", "All"). An amount over the limit shows an error notice, "Amount exceeds the cash balance." or "Amount exceeds this holding’s value.". The thesis is always visible: "Thesis" over a multi-line field "Why should the cabal make this trade?" (280 characters, counter near the limit) and is required. "Review proposal" with an empty thesis shows "Add a thesis before sending your proposal." and does not open the review. A failed cash read replaces the screen with the generic error.

**Review** (`ProposeReviewSheet`). A sheet over the amount screen at one half-height detent (0.55, also at accessibility sizes) that does not expand, titled "Review proposal" with the close round; the body scrolls and the footer stays pinned. It cannot be swiped away while sending. The hero is the stock mark, "Buy NVIDIA" and the amount; below it the cabal chip, "Thesis" with the full text, and key rows "Voting window", "Votes to pass", then the trade rows ("Cabal gets", "Price", "Raises", "Cabal keeps", "Pot") and "Who votes". The chain row and best price follow. The footer holds "<voters> will vote on this proposal." over "Send proposal" ("Sending…" while it runs). A failed send shows "Couldn’t send your proposal. Your draft is saved here." and keeps the draft, and closing and re-opening the sheet keeps the amount, thesis and chain choice. After a send the sheet shows "Proposal sent", "Buy NVIDIA · Big Ballers" and "View proposal", which opens the new proposal; closing the sheet closes the flow.

## Money

**Shared rules (UI v3).** Cream canvas, `Space.gutter` sides, a `ScrollView` with the primary in a form footer, or in the glass `BottomCTA` while a keypad is up. Notices are `MonacoNotice` (info, warn, error). Key rows sit in a `MonacoCard(.inset)` of `ReceiptLine`s. A confirmed result is an `OutcomeView` over a key-row card (`MoneyOutcome`). A full-screen read failure is "Couldn't load this view" with "Try again". Service refusals keep their toasts. No screen shows a success state before the service has confirmed it.

**Deposit** (`DepositView`, template `add-money`), a half sheet (0.55) presented by `AppNavigator.presentDeposit` and hosted by `MainTabView`. Title "Deposit" with a close round. Choosing a row closes the sheet and pushes the next screen onto the current tab, so back returns to where Deposit opened. Eyebrow "Your cash" over the balance (`deposit-screen-balance-value`; "—" and a scoped error row when it cannot load), the header "How would you like to deposit?", then "Card or Apple Pay" (`deposit-card-row`, then pushes `CardFundingRoute`) and "Send crypto" (`deposit-crypto-row`). Warn notice "Some funding methods may be unavailable in your location or account state." only after a card session failed to start; info notice "Money arrives in your cash balance. Funding a cabal is a separate step."

**Card / Apple Pay** (`CardFundingRoute`, template `card-funding`, new). Tab bar hidden. Card orb, "Continue securely.", the key rows "Destination", "Amount & fees", "Next step", an info notice, primary "Continue to provider" (`deposit-card-spinner`, "Opening…" while the session starts, opens the provider page) and outline "Choose another method". Back from the provider maps `CardDepositState`: processing is "Checking your deposit" with "View cash", done is "Deposit received", a failed session is "Funding cancelled" with the existing "Card purchase didn't go through." and "Back to funding methods". A failed start stays on the handoff with its toast.

**Choose a network** (`DepositNetworksRoute`, template `network`). Helper, one row per offered network ("Direct USDC deposit" for Solana, the arrival note for others, `deposit-network-<id>`), and an info notice.

**Receive USDC** (`DepositAddressRoute`, template `deposit`). Title "Receive USDC". "USDC on <network>" with a "Deposit" badge, then a one-line info notice directly above the card whether the address is loading, ready or waiting on a deposit (`deposit-network-warning`): "Only USDC on <network>". It never gives way to the waiting notice, which sits below the card. Then a card with "Your deposit address", the scannable QR on its white tile, the address and "Copy address" (toast "Address copied."), an info notice (the arrival and fee notes) and, while a deposit is arriving, the warn notice "Waiting for network confirmation. Your balance hasn't been credited yet." (`platform-balance-arriving`), and "How it works" with three numbered steps. When a settled deposit lands while the screen is open it becomes "Deposit received" (it shows no address, so no warning) with the key rows Asset, Network, Destination, Amount, "Fund a cabal" and "Back to Home".

**Deposit to <cabal>** (`FundRoute`, template `fund-cabal`). The cabal cover, name and "Cash $500.00" (with "· $50.00 pending" while money is in flight), a warn notice when the cabal is paused, "Deposit amount" with "$25", "$50", "$100", "Max", the keypad, the error notice "Not enough cash. Reduce the amount or deposit into your account." with an outline "Deposit to account" when over the cash, the warn notice "Processing your deposit…" while submitting, and the yield info notice. Primary "Deposit $250.00" (`fund-cabal-submit-button`), "Enter an amount" at $0.00. After the fund is accepted a glass sheet "Deposit confirmed" shows "$250.00 added to your share in <cabal>" with "Done"; both dismiss and pop back. If the response lands after the member left, the toast stays. Zero cash is "Add money first" with "Add money".

**Cash out your share** (`CashOutRoute`, template `cash-out`). An info notice (`cash-out-explainer`), "Your share in <cabal>" between rules, "Cash-out request" with "25%", "50%", "All", the keypad, error notices when over the share or under the minimum, and the primary "Review cash-out" (`cash-out-review`). Review keeps the amount and shows the key rows Requested amount, Destination, Final proceeds, Costs, a warn notice and "Request cash-out" (`cash-out-submit`, "Requesting…"); back returns to the amount. A started job shows "Cash-out requested" with Status "Processing" and "View activity" (`cash-out-progress`), then "Cash-out complete" with Status "Completed" when it settles, or "Cash-out didn't go through" if it fails. No slice is "Nothing to cash out yet" with "Fund this cabal".

**Withdraw to wallet** (`WithdrawRoute`, template `withdraw`). The helper, "Withdrawal" with "Max" and "$850.03 available", the receive line for EVM networks, "Wallet address" with Paste, the caveat, a loss border and error line for an invalid address, a "Network" menu of every `WithdrawNetwork` (`withdraw-network-picker`) and the primary "Review withdrawal". Over the balance is the error notice "The amount exceeds your available balance." Review is titled "Review withdrawal" with From, Asset, Amount, Network, the full Destination, Arrives and Network costs, a warn notice and "Withdraw" ("Withdrawing…"). An accepted withdrawal shows "Withdrawal processing" with Status "Pending", then "Withdrawal completed" once confirmed, and "View transaction", which opens activity. A balance failure is the full-screen load failure; no balance is "Nothing to withdraw yet" with "Add money".

**Account activity** (`AccountActivityRoute`, #2138, UI v3). Inline title and root title "Account activity", then an All / Pending segmented control (All is the default every time the screen opens). `GET /v1/me/txns`, paged. Rows are a 44pt glyph tile, a title ("Deposit received", "Withdrawal", "Deposit to <cabal>", "Cashed out of <cabal>"), a subtitle ("USDC · <date>", "From cash · <date>", "To cash · <date>") and the signed amount with a chevron; a pending row shows a warn "Pending" badge in place of the amount and a failed row a negative "Failed" badge. Pending filters the loaded pages on the client, keeps loading while nothing matches and more pages remain, and then shows "No pending transfers" / "Transfers that are still processing show up here." A closing info notice says "This is your personal account history. Cabal trades are viewed through their cabal and proposal records." Empty: "No transactions yet" / "Personal deposits, deposits, redemptions and withdrawals will appear here." Loading is the generic skeleton and a failed first load is the full-screen load failure with "Try again". A row opens the transaction receipt.

**Transaction receipt** (`AccountTxnReceiptRoute`, `ReceiptView`). Inline title "Transaction receipt". `ReceiptView` is the one receipt layout, fed by the MonacoCore `ReceiptContent` value; the cabal trade receipt (`TransactionRoute`, Lane B) adopts it from its side. A centred 54pt mark, the eyebrow (the row title, or "Bought NVIDIA" for a trade), the amount (`account-txn-receipt-amount`) and a status badge: Completed, Processing (trade: Execution pending), Not completed or Status unconfirmed. A warn or error notice explains pending, failed and unknown. The key-row card lists From, To, Asset, Requested amount, When, Status (`account-txn-receipt-status`), Record ID and, for a deposit to or cash out of a cabal, a Cabal row that opens the cabal. Withdrawal destinations read "External wallet" until `GET /v1/me/txns` returns a masked address. Below the card, "Account activity" returns to the transfer history, and "View on explorer" opens the Solscan page in Safari, or is disabled with "Requires a confirmed transaction ID" until the transfer has a signature. A harness-only error state shows the load failure with "Try again".

**Cabal cash** (`GroupCashRoute`, `GroupCashView`). Inline title "Cabal cash". Reads `GET /v1/cabals/{id}/pot`. The eyebrow "<cabal> · Cash", the cash figure and "Unallocated USDC in the cabal pot"; an "Idle cash" card with Asset, Cash in the cabal and Yield status; an info notice, "Current yield details are not available. The provider, rate and applicable terms will appear when confirmed."; then "Use cabal cash" with "Propose an investment" (hidden when the viewer cannot propose) and "Return to holdings". Yield status is "Rate requires live confirmation" today and never the asserted `CabalYield.apy`; it shows a rate only once the pot read carries one, and "Rate unavailable" in a warn notice when it reports cash without yield information. Missing API: the provider, rate and terms of the yield on a cabal's idle USDC. Until Lane B wires the holdings Cash row to `GroupCashRoute(cabalID:cabalName:)`, the screen is reachable only from `-groupCashHarness`.

## Markets tab

Large title "Markets". The tab was "Stocks", and its code and identifiers keep `StocksTab`, `tab-assets` and `assets-root`. Top to bottom:

- **Search.** "Search stocks or tickers". Typing switches the whole screen to search mode (below); clearing returns to browse. The 300 ms debounce sends `q` and `filter` together.
- **Stale notice.** The warn notice "Prices delayed · 14 minutes ago." (`markets-stale`) when the last refresh failed while rows from an earlier load are still on screen. The minutes count from the last good load, "1 minute ago" in the singular and "Prices delayed · just now." under a minute. The error toast still fires.
- **Top movers.** The line "24h token price change" and a rail of up to four `MoverTile`s (logo, name over ticker, signed day change, "24h volume" over "Opens soon"; `markets-mover-<symbol>`). The rail ranks the first `filter=popular` page by the size of the day change, up or down, and hides with fewer than two stocks that have a change. At accessibility text sizes the tiles stack.
- **Watchlist.** "Manage" opens `WatchlistRoute`. The first three watched stocks in the order they were added, quoted with `GET /v1/assets/{symbol}`, as stock rows (`markets-watchlist-row-<symbol>`). A quote that fails keeps its row with "—". With no stars the card "Your market, at a glance" / "Add stocks to your watchlist." with "Add" (`markets-watchlist-add`).
- **Popular now.** "Most popular on Monaco" and the first three rows of the popular page in server order (`markets-popular-row-<symbol>`).
- **All stocks.** Two menu selects. Sort (`stocks-sort`) offers "Default"; "Volume", "Trade count", "Name A–Z" and "Price change" are disabled under "More sorts open soon". Filter (`stocks-filter`) is "All" (`filter=all`, the default), "Popular" (`filter=popular`) or "Pre-IPO" (`filter=pre_ipo`). Under them the paged rows (`assets-row-<symbol>`).

Rows are logo, name over ticker, price over day change and a chevron. A moon glyph shows while the home market is closed, "Paused" when the stock is tradable but not quotable, "—" stands for a missing price or change, and an untradable stock sits at 45% with "Can't buy right now".

**Search mode.** The sections under the search field give way to "Search results", the same two selects and the rows. No match is "No matching stocks" / "Try a company name, ticker or another filter." (`assets-search-empty`).

**States.** Loading is the skeleton shapes of the screen. An empty catalogue is "No stocks to show", and an empty filter is "No stocks in this filter" / "Try a different category.". A first load that fails is the full-screen "Couldn't load this view" with "Try again" (`assets-failed`). A refresh that fails with rows on screen keeps them and shows the stale notice and the error toast. Pull to refresh reloads the list, the popular page and the watchlist quotes, and a price hint refreshes all three while the tab is visible.

**Opens soon.** The API does not serve a stock's 24h volume or trade count, a movers ordering, the other sorts or a sector filter. The tab shows "Opens soon" and the disabled sorts until it does.

## Stock screen

**Stock screen** (`AssetRoute`, template `instrument`). Pushed in whichever tab opened it (Markets, Home, Feed, a cabal), with the glass back button and the stock's name as the title, blank while it loads. Top to bottom, 24 apart:

- **Header.** Logo, name (`asset-detail-name`) over ticker, and the watch star (`asset-watch-star`), a glass round button. The star adds the stock to the local watchlist or takes it off at once, with no toast, and reads "Add to watchlist" or "Remove from watchlist". The 51st star shows "Your watchlist holds up to 50 stocks.".
- **Notices.** Up to two warn notices. "Token price delayed · 14 minutes ago." when the price is more than five minutes old or the last refresh failed with a price on screen. "This stock is unavailable for new proposals." for an untradable stock. "Trading paused right now." when it is tradable but not quotable.
- **Price.** The price in `moneyFont(.hero)` with the tick flash ("—" with no price), then the signed change and the range label ("Past day · GOOGL"). Press and hold on the chart replaces both with the held bar's price, change and time. Below them the market session badge ("Market closed", "After hours") with "Trading 24/7 on Solana", and the best-price line.
- **Chart.** Candles by default, 170 tall, with the choice kept in `@AppStorage("asset-chart-style")`. A bar interval select ("5m", "15m", "1h"... offered per range), the candle or line toggle, the chart, and the ranges 1D 1W 1M 3M 6M 1Y ALL. Two-finger pinch zooms around the fingers, one finger pans back in time and loads older bars, the "»" button jumps back to the latest bar, and press and hold draws a crosshair that the price follows. The line style is the v3 line: a stroke in `profit` or `loss` by the sign of the range change, a gradient fill, an end dot and an ink tooltip pill. Short history: "Price history builds up over time." with "The curve draws as it trades." ("The curve draws as the token trades." for a pre-IPO token). A chart that fails to load keeps the page and shows an error notice with "Try again".
- **Actions.** One row of equal-width buttons, stacked only at accessibility text sizes. "Propose a buy" (`asset-detail-propose-buy`) on the left, which reads "Unavailable" and is disabled for an untradable stock, and is disabled while trading is paused. "Propose a sell" (`asset-detail-propose-sell`) on the right shows when a cabal the viewer votes in holds the stock; without it the buy button takes the full width. Under them the caption "Your cabal votes before anything is bought", with "or sold" when the sell button shows.
- **Market stats.** The data attribution, then Open, Previous close, Day high, Day low, 52-week high and 52-week low in a two-column grid, read off the day and year candles and the quote's move (a value the candles cannot give is left out). A "24h volume" cell follows them when the asset has a volume reading from the last 36 hours. Under the grid "Market cap and supply open soon." and the 52-week range bar.
- **About.** Headed by the stock's name. A pre-IPO token shows the "About pre-IPO tokens" text, and an equity "Company profile opens soon.".
- **Cabals holding <TICKER>.** "Your cabals", then one row per cabal of the viewer's that holds the stock (cover, name, shares, value over P&L, `asset-position-<id>`), each opening `CabalRoute`. With none: "No visible holders" / "Private holdings are not shown.".
- **Proposals for <TICKER>.** "No visible proposals for this asset." and "Proposals by stock open soon.".
- **Also available from.** The other issuers' listings of the same company, each opening its own stock screen.

**States.** Loading is the skeleton shapes and four row skeletons. A detail that fails to load is the full-screen "Couldn't load this view" with "Try again" (`asset-detail-failed`). A failed refresh with the detail on screen keeps it, shows the "Token price delayed" notice and the error toast.

**Opens soon.** The API does not serve a stock's market cap, circulating units, 24h volume or trades, a company profile, its public holders or the proposals that name it. The screen shows the "opens soon" lines until it does.

## Watchlist

**Your watchlist** (`WatchlistRoute`, template `watchlist`). Pushed in the current tab, titled "Your watchlist", with the glass back button and the tab bar visible. The search field "Find stocks for your watchlist" sends `q` with the same 300 ms debounce as Markets. Under it the count line, "3 stocks selected", "1 stock selected" or "No stocks selected", then the catalogue (`filter=all`, paged) as rows of logo, name over ticker and a star (`watchlist-row-<symbol>`, `watchlist-star-<symbol>`). A row opens `AssetRoute`. The star toggles the stock at once, and the row shows no price. A starred stock is `brandSoft` with a filled star, labelled "Remove <name>"; an unstarred one is glass with an outline star, labelled "Add <name>". Empty (state `empty`): "Your watchlist is empty" / "Add stocks using the stars below." above the rows. No match: "No matching stocks" / "Try another search.", and the count line stays. Loading is four row skeletons under the search field. A failed first read is the full-screen "Couldn't load this view" with "Try again" (`watchlist-failed`). A failed second page keeps the rows and shows the error toast.

**Local watchlist.** The list lives on this device only. `WatchlistModel` holds up to 50 symbols, oldest first, and writes through `UserDefaultsWatchlistStore` under `markets-watchlist.<userID>` as a JSON array, so a second account on the device never sees the first one's list. Nothing is sent to the server. The 51st star shows the toast "Your watchlist holds up to 50 stocks." and adds nothing. A symbol the server answers `asset_not_found` for is dropped from the list without a message. `PrivacyInfo.xcprivacy` already declares the UserDefaults category with reason `CA92.1`, which covers this app-only use.

## Feed tab

Large title "Feed" with two 44pt glass rounds, then the search field "Search the feed" and a list of compact event cards. Pull to refresh; the last five cards load the next page. #671 built the feed, #702 mute and hide, #711 the post, and the UI v3 redesign the layout below.

| Part | Shows |
| --- | --- |
| Bell | "Activity notifications". Pushes the Activity screen, a "on its way" placeholder until that screen ships. The brand dot for unread activity waits for the API |
| Filter | "Filter feed", a menu that applies each pick at once and stays open. "From": "Everyone", "My cabals", "Following". "Show": six toggles that combine, "Proposals", "Completed trades", "Price moves", "Replies & votes", "Proposal outcomes" and "Cabal activity", with none on meaning everything and no "All" toggle. "Replies & votes" shows replies only, because votes have no feed item. Then "Muted", which opens the muted list. The round fills when a scope other than "Everyone" or any kind is on |
| Card | A bordered card: author row, one event block, reactions row. Long-press mutes or hides, with a toast and "Undo" |
| Author row | Avatar (opens `UserProfileRoute`), name, event pill, then "<cabal> · <time>". A price move has no person, so it shows a market glyph and "Market" |
| Event pill | "Proposal" and "Buy" (green), "Propose sell" and "Sell" (red), "Price move" and "Approved" (amber), "Discussion" (violet), "Cabal" (green) |
| Asset block | Proposals, trades and approvals. The company logo (a lettered square when the stock has none), ticker, status ("Voting open", "Executed", "Execution pending"), the thesis on two lines. Opens `ProposalRoute`, `TransactionRoute` or `AssetRoute` |
| Price block | A price move: the company logo, ticker, the change the title states, and "24h token volume" with "Opens soon" |
| Cabal block | A started or joined cabal: its picture (its tint tile and initials when it has none), name, "New public cabal". Opens `CabalRoute` |
| Reactions | A heart (likes and unlikes, with the count from the API), the comment count (opens the post), and share (opens the system share sheet with the item headline and your invite link) |

States. Loading is a skeleton under the live title and search. Empty: "No updates yet" / "Proposals, trades and conversations will appear here."; with a search or filter, "No matching updates" / "Change the filter to see more activity." Following with no follows: "Follow people to see what they do." with "Find friends". Error: "Couldn’t load this view" with "Try again". A page that fails to load under cards shows "Older updates couldn’t load. The items above are still here." with "Retry older updates", and no toast. The end of the list reads "You’re caught up", or "No more matching updates".

**Opens soon.** The feed API sends no likes, no author one-month return ("+8.2% · 1M", or "P&L private"), no structured amounts, no 24h figures, no reply or approval items. The app draws those only from the Debug sample harnesses (`-feedHarness`, `-feedPostHarness`), never from live data, and each needs a backend ticket: feed likes, the author return and its permission, structured trade and proposal figures (amount, share of the pot, units, average price, buy or sell), price-move figures, and reply and outcome item kinds. Until the figures exist, a proposal or trade is drawn as a buy.

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

Toolbar: the back button on the left, the cabal name centred in `Typo.headline` (one line, `chat-title`) and an info round on the right (`chat-info`, "Cabal information") that opens the same "Cabal details" sheet as the cabal screen's "i". No tab bar. The timeline sits on the cabal canvas.

- **Day label.** A centred caption ("Today", "Yesterday" or the date), with no rule.
- **Their messages.** A 26pt avatar (opens `UserProfileRoute`) on the first message of a run, with a spacer on the rest. The bubble is `surfaceSunken` with no stroke, 87% of the content width at most, a 4pt tail on the top-leading corner and 17pt on the others. The author's name sits inside the bubble over the first message of a run. The time sits under the bubble.
- **My messages.** `brandSoft` bubble with `primaryText`, the 4pt tail on the top-trailing corner. The status line under it reads "10:25 · Sent", "Sending…" while the post is in flight, or "Not sent" with a "Retry" text button after a failure (long-press: Retry or Delete). The "Seen by N" label joins the status line of the newest message.
- **Proposal card.** A message with a `proposal_id` renders `ChatProposalCard` between the bubbles: the proposer's 38pt avatar, "Mia proposed NVIDIA" (or "proposed selling" for a sell), "$1,200.00 from the cabal pot", "Big Ballers · 2 hr ago" and a small "Review" button that opens `ProposalRoute`. A footer row under a rule shows the reply count and opens the thread. Once the vote has passed the title reads "Proposal approved", the button reads "Status" and the footer reads "Execution pending · Not completed". The proposal is fetched once per id per session; while it loads the card is a skeleton, and if the fetch fails the card reads "A proposal" with Review still on. The reply faces from the design are not drawn: a message carries only its reply count.
- **Offline.** A warn notice above the timeline, "You're offline. Messages can be drafted but won't send yet." (`chat-offline-notice`), shown while `NetworkPathModel` reports no network path. The composer stays on; a send while offline ends in the failed state with Retry.
- **Composer.** "Message your cabal" in a sunken capsule that grows to a rounded field after one line, and a 44pt send disc (`chat-send`, "Send message"). The @mention picker is a card of `MonacoRow`s above the composer.
- **States.** Loading is five skeleton rows. Empty: a chat tile, "Start the conversation" over "Messages here stay with your cabal. Proposal-specific discussion stays attached to the proposal." First-load failure: "Couldn't load messages." with "Try again". A non-member or removed member sees the composer replaced by an info notice, "You're not in this cabal anymore."

Threads (`ChatThreadRoute`), delete, "Seen by N" (#704) and @mentions (#2147) keep their behaviour. The thread screen uses the same bubbles and composer, with the parent message over a rule and an "Also post in the cabal chat" switch.

## Cabal holding (`CabalHoldingRoute`)

`CabalHoldingRoute(cabalID:symbol:)`: one cabal's position in one stock. It keeps the tab bar. The title is "NVIDIA holding", or "Cabal asset holding" while loading and on an error. The holdings rows on the cabal screen still open `AssetRoute` until S9 wires the push.

| Order | Section | Shows |
| --- | --- | --- |
| 1 | Asset header | 47pt `StockMark`, the asset name and "NVDA · Big Ballers". One VoiceOver element |
| 2 | Stale notice | A warn notice, "Holding value last updated 14 minutes ago.", when the price read is ten minutes old or more |
| 3 | Performance | "Position value", the value in `moneyFont(.hero)`, and the return with "Position P&L · All time" |
| 4 | Chart | The position's value line with the 1W 1M 3M 1Y All periods, or `QuietChart` "Position chart opens soon" while there is no series. No position: "No position yet" / "Completed trades in this cabal appear here." |
| 5 | Stat grid | Units, Average buy, Average sell, Cost basis, Realized P&L · total, Unrealized P&L. A missing figure reads "—", with "Average prices and realized P&L open soon." under the grid |
| 6 | Trade history | "Cabal · Times shown in UTC", then a row per confirmed buy or sell of the symbol: sign tile, "Buy · 24.4444 units", the UTC date, the unit price and the total. A row opens `TransactionRoute`. "Show older trades" pages on. A failed read shows "Couldn't load trade history." with "Try again" in this section only |
| 7 | Related proposals | Up to two open proposals of this cabal on the symbol, drawn by `ProposalCard`, or "No open proposals for NVDA" |
| 8 | Position details | A disclosure: Asset, Scope, Average buy, P&L scope, and "View token market", which pushes `AssetRoute` |

States: loading skeleton (`holding-loading`); error (`holding-error`, "Couldn't load this view" with "Try again"); a reload that fails with data on screen keeps the data and shows the toast "Couldn't refresh this holding."; empty; loss (the return, chart and Unrealized cell in the loss colour); stale; not a member ("This holding is private").

Data. `GET /v1/cabals/{id}/pot` gives the units, price, value, cost basis and unrealized P&L of the holding and the price time; `GET /v1/cabals/{id}` the cabal name; `GET /v1/assets/{symbol}` the name, logo and token decimals; `GET /v1/cabals/{id}/activity` the trades (confirmed buys and sells of the symbol, filtered on the client, up to three pages of 100 per load); `GET /v1/cabals/{id}/proposals?filter=open` the related proposals. The backend has none of these (D7), so they render "—" or the fallback in production and come from `CabalHoldingFixture` only in the DEBUG harness:

1. Average buy and average sell price per cabal holding.
2. Realized and unrealized P&L split per cabal holding (only unrealized exists).
3. A position value series per cabal holding and range (1W, 1M, 3M, 1Y, All).
4. Period P&L for the holding.
5. A `symbol` filter on `GET /v1/cabals/{id}/activity`.
6. A per-trade unit price (today `usdc_micros / units`).

## Profile tab

Pull to refresh, like Home. Title "Profile" with a glass round "Settings" (`gearshape`, `profile-settings-row`) that opens `SettingsRoute`. The share round sits in the identity card and opens the iOS system share sheet in one tap.

| Order | Slot | Owner | Shows |
| --- | --- | --- | --- |
| 1 | `ProfileHeaderSlot` | #644, #5571 | The identity card (`IdentityCard`): a dark `identityFill` card with the 64 pt photo and a pencil badge (opens "Your face": eight animals and "Choose a photo"; VoiceOver "Edit profile photo"), the name, "@handle" and "Joined October 2026". The whole name block opens `EditProfileRoute`. A share round (`profile-share-button`, "Share your return") sits at the right. Below a rule: "Positions value" (the total slice value) and "Total P&L · All time" (dollars over percent in `profitOnIdentity` / `lossOnIdentity`). A bio line shows only when the API returns one. At accessibility sizes the two stats stack. Loading is one card-sized skeleton. A failed profile load replaces the screen with "Couldn't load this view" and "Try again" |
| 2 | `ProfileFollowCountsSlot` | #620, #5571 | One row, left, centre and right: "12 followers", "8 following" (each opens the follow list) and "3 cabals" (static) |
| 3 | `ProfileBalanceSlot` | #610, #5571 | `ProfileMoneyStrip`: "Net worth" and "Cash" (each opens `BalancesRoute`) and a compact "Deposit" button, under a hairline, with "Account balances are visible only to you." beneath. At accessibility sizes the Deposit button drops to its own full-width row. A card deposit in flight keeps its "arriving" line |
| 4 | `ProfileCabalsSlot` | #2140, #5571 | "Your cabals" with "See all" (opens the Cabals tab) over a horizontal rail of `CabalCoverCard`s (the share disc opens the iOS system share sheet). Hidden when empty or failed; two skeleton cards while loading |
| 5 | `ProfileLinksSlot` | #682, #663, #2139, #5571 | One list: "Account activity", "Find friends", "Invite friends", "View as others" and "Withdraw", each with a `brandSoft` glyph tile and a subtitle |

**Edit profile** (`EditProfileRoute`, new). Pushed from the identity card's name block, the Settings "Edit profile" row and `HandleEditRoute` (the Username field focused). A centred 76 pt photo with a pencil badge, then Name, Username (availability check, the 30-day change cooldown, "Choose a valid username.") and Bio (disabled with "Bio opens soon." until the API exists). A notice slot reads "Saving…" or "Profile saved.", then a full-width "Save changes", disabled when nothing changed, a field is invalid or a save is in flight. A load failure shows "Couldn't load this view" with "Try again".

**Settings** (`SettingsRoute`, #2139, #5571). Title "Settings". Section "Notifications": one row (On or Off; while permission is undecided a tap asks for it, otherwise it opens iOS notification settings, #2143), then two switches, "Decision requests" (`settings-category-decisions`) and "Replies and mentions" (`settings-category-replies`), each with its switch (`<id>-switch`). They read `GET /v1/me/notification-preferences` and stay disabled until it answers. A tap shows the new value at once and saves both with `PUT /v1/me/notification-preferences`; a failed save puts the switch back and shows an error toast. Section "Account": "Edit profile", "Account activity" (`AccountActivityRoute`), "Withdraw" (`WithdrawRoute`), "Blocked people", "Block explorers", and "Sign out" with a confirm card in place ("Sign out of Monaco?" / "You can sign back in with your existing account. This does not withdraw funds or leave your cabals." with "Sign out" and "Stay signed in"). In its own group, a destructive "Delete account" (`DeleteAccountRoute`, #695). Blocked people lists who the viewer blocks (`GET /v1/me/blocks`): avatar, name and "@handle", and "No one blocked" / "People you block show up here." when empty. Footer: app version. States: ready, saving (info notice), error ("Couldn't load this view").

**Find friends** (`FriendsRoute`, #663, #2142, #5571). A search box ("Search people by name or handle"), a Discover / Following segmented control (not shown in the first-run contacts step), and person rows (avatar, name, "@handle", chevron). Discover shows search results with a Follow toggle each, or the contacts matches or explainer. Following lists the people the viewer follows, each with a "Following" badge. An info notice reads "Search is explicit. Monaco never follows anyone for you." States: loading skeleton rows; "No people found" with "Clear search"; "You're not following anyone yet"; error with "Try again".

**User profile** (`UserProfileRoute`). The identity card without the pencil or the Positions value (name, "@handle", a joined line and bio when the API returns them, and Total P&L, which shows "—" with "Shared performance opens soon." until the API exists), then the counts row, a full-width "Follow" or "Following" button (#620), and a "…" menu with "Report" and "Block" (#2145). On your own profile ("View as others") the button and the menu give way to the notice "This is how your profile appears to others. Your account balances stay private." A private profile shows "Performance is private". Then "Cabals you share" (#660): rows with pot figures. Empty: "No cabals in common" / "You and Maya aren't in a cabal together yet." A banned or deleted user: "Profile not found" with "Find people".

## Who builds what

The slot tables above name each owner. Screens and controls added by this map:

| Ticket | Builds |
| --- | --- |
| #2134 | The slots and routes this map named before they existed, the live `CabalActionsSlot`, the Home avatar button |
| #2135 | `CabalRulesSlot` and the voter picker in the cabal editor (inline on Cabal settings since #3463) |
| #2136 | `GET /v1/cabals/{id}/pot` |
| #2137 | `CabalPotSlot`, `CabalSliceSlot`, `CabalHoldingsSlot` |
| #2138 | Account activity |
| #2139 | Settings and the "Settings" row of `ProfileLinksSlot` |
| #2140 | `ProfileCabalsSlot` |
| #2141, #2142 | People search |
| #2143 | The push pre-prompt and the Notifications row |
| #2144, #2145 | Report and block |
| #2146 | A banned cabal's state |
| #2147 | @mentions in chat |
| #2133 | The journey that walks the demo story through all of the above |
| #5558 to #5571 | The UI v3 screens: sign in, Home, Your money and Activity, the money flows, account activity and Cabal cash, Propose, the proposal screen, the Cabals tab and Discover, the cabal screens, chat and Cabal holding, Markets, the stock screen and the watchlist, Feed, the Leaderboard and Profile with Settings |
| #5572 to #5575 | The screen parity gallery, and the Dark, Dynamic Type and accessibility checks of every v3 screen |
| #5576 | This page's tab order, the Markets tab, Propose and the new screens, and the journeys: `home/activity`, `stocks/watchlist`, `cabals/discover`, `cabals/info-and-members` and the Home and Leaderboard steps of `ranking/leaderboards` |
