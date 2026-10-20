# v3 components

Every reusable piece in the prototype. Columns: anatomy and sizes (pt), states, tokens (names from [tokens.md](tokens.md)), template ids that use it (open them in [monaco-ui-v3.html](monaco-ui-v3.html) or see the [README](README.md) index), and the SwiftUI type and file planned for it. "Today" means the closest existing type that the planned one replaces or retunes. Paths are under `apps/mobile/Monaco/`. Foundation tickets F1–F8 build the `Design/` files; screen tickets S1–S14 use them. `Design/*` and `Features/Shell/*` are frozen once the screen tickets start.

Common rules: icons are SF Symbols (24-grid line icons in the design, stroke 1.75, mapped through the new `Design/MonacoIcon.swift` enum so screens never type symbol names); sizes 22 default, 16 small, 30 large, 18 notice, 14 link chevron. Every interactive element has a 44pt hit area. Every data-backed screen keeps four states: loading skeleton, empty, error row, error toast.

## Chrome

| Component | Anatomy and sizes | States | Tokens | Used by | SwiftUI (planned) |
| --- | --- | --- | --- | --- | --- |
| Tab bar | Floating pill, 5 items (Home, Cabals, Markets, Feed, Profile): icon 23 + label. Design: 70 tall, inset 8, bottom 22, items 56 | on (`brandSoft` bubble, `accent` icon and label, bold) / off (`muted`); large text 74 tall | `glassFill`, `glassStroke`, `Shadow.floating`, `brandSoft`, `accent`, `muted` | every tab root | System `TabView`, never hand-drawn. `Features/Shell/MainTabView.swift`, `MainTab.swift`, `Design/MonacoAppearance.swift` (F2) |
| Toolbar | 61 tall. Left: 44 glass back. Centre: title 16 semibold. Right: 44 round(s) or spacer. Brand variant: wordmark (110 wide) + bell + 32 avatar. Cabal variant: back, name, share, info (40 rounds) | root (no bar), pushed, brand, cabal | `glassFill`, `hairline`, `ink` | all screens | System navigation bar via `MonacoAppearance`; `monacoToolbarIcon` (F2) |
| Round glass button | 44 circle, 22 icon; `.quiet` has no fill; sheet title 44 | default, quiet, selected star (`brandSoft` fill, filled `accent` star) | `glassFill`, `hairline`, `surface` (reduce transparency) | home, cabals, feed, profile, cabal, instrument, watchlist, sheets | `Design/MonacoGlassButton.swift` (new, F2): material on 18, `glassEffect` on 26 |
| Root title | Large title 30 bold + optional trailing round button(s). Margin `16 / 16` | with "+" (cabals), with bell and filter (feed), with settings (profile) | `Typo.display`, `ink` | cabals, markets, feed, profile, notifications, leaderboard, activity, discover | Large title in `Typo.display` (F2) |
| Section head | h2 22 + optional count pill + optional trailing text link (accent, chevron 14). Min height 34 | with count, with link, link opens a sheet | `Typo.section`, `brandSoft`, `accent` | home, cabal, markets, feed, profile, instrument, holding, proposal, chat | `MonacoSectionHeader` in `Design/MonacoList.swift` (F4) |
| Dock | Glass bar above the tab bar with 1–2 flex 48 buttons; info caption 15 centred | with tabs (bottom 106) / without (26) | `glassFill`, `hairline` top | instrument ("Propose a buy"), proposal (Vote No / Vote Yes), chat, post | `BottomCTA` (retuned, F2) |
| Composer | Input 44 on `surfaceSunken` radius 20, send disc 44 in `brandFill` | chat / comment placeholder, disabled offline | `surfaceSunken`, `brandFill`, `onBrand` | chat, post, proposal | `CommentComposer`, `GroupChatParts` (restyle in S10, S12) |
| Sheet chrome | Radius 31, grab handle 36×5, title 22 with close and expand round buttons, footer button 48. Detents: review 0.66 / `.large`, votes 0.53 / `.large`, deposit confirmed height 310 / `.large`; all `.large` at accessibility text sizes | partial, expanded, dismiss disabled only while sending | `sheetGlass`, `sheetStroke`, `Shadow.sheet`, `sheetDim` | review-proposal, votes, fund-cabal (completed), invite, feed filter | `monacoSheetChrome` (system background on 26, `.regularMaterial` on 18) + `Design/MonacoSheetHeader.swift` (new, F2) |
| Action sheet | `confirmationDialog` style: rows 60 tall with top rule, icon 20 + title 17 semibold `accent`, optional sub line | row disabled (.5) | `Typo.headline`, `accent`, `hairline` | propose (Buy stock / Sell a holding / Cancel), proposal (Change vote Yes / No) | `confirmationDialog` (S6, S7). The design draws no bot row; decision 3 in [decisions.md](decisions.md) keeps the app's trading-bot entry, restyled in the same language |
| Status bar, home indicator | Drawn in the prototype only | | | | Not drawn |

## Buttons and links

| Component | Anatomy and sizes | States | Tokens | Used by | SwiftUI (planned) |
| --- | --- | --- | --- | --- | --- |
| Primary | Capsule, min 48, padding `10 18`, 15 semibold, optional 16 icon, gap 7. `.full` = full width | enabled, disabled (.45), working (label swap, e.g. "Sending…") | `brandFill`, `onBrand` | all forms, empty states, outcomes | `.monacoPrimary` 56 → 48 in `Design/MonacoButtons.swift` (F3) |
| Secondary | Same size, `brandSoft` fill, `accent` text | as primary | `brandSoft`, `accent` | cabals (Create a cabal), profile, members (Review), markets (Add) | `.monacoSecondary` (F3) |
| Outline | No fill, 1pt `hairline`, `ink` text | as primary | `hairline`, `ink` | second actions, Vote No, Change vote, Retry, Cancel | `.monacoOutline` **new** (F3) |
| Danger | `loss` fill, `canvas` text | as primary | `loss` | defined, unused in v3 screens | `.monacoDestructive` (F3) |
| Small | Min 38, 15 text, padding `9 13`, hit area extended | as primary | as primary | decision card Review / Status, members Review | `.monacoCompact` 38 (F3) |
| Compact Deposit | Label-only capsule, visual ≈ 32 × 82 inside a 44 × 82 hit box, `brandFill`, 15 semibold `onBrand` | enabled; hit stays ≥ 44 at large text | `brandFill`, `onBrand` | home and profile money strips | `.monacoCompactProminent` retuned to 32 visual (F3) |
| Text link | `accent`, optional chevron 14, min height 44 | | `accent` | section heads (See all, View all, Manage, Open chat), verify (Resend, Change number), profile (Edit), notifications (Mark all read) | `.monacoText` (F3) |
| Action tile | 44 circle in `brandSoft` with 22 `accent` icon, label below, gap 7; tile 66 × 72; `actions15` has 3 columns | disabled (.35) | `brandSoft`, `accent` | cabal (Deposit, Propose, Cash out), balances (Deposit, Withdraw, Activity) | `CircleAction` retuned (F3) |
| Load more | Full width 48, `muted` text | idle, loading, "caught up" | `muted` | feed, home market updates, cabals rank more | pagination `onAppear` (S2, S8, S12) |

## Selection and inputs

| Component | Anatomy and sizes | States | Tokens | Used by | SwiftUI (planned) |
| --- | --- | --- | --- | --- | --- |
| Segmented | Track `surfaceSunken` + 1pt `hairline`, radius capsule, padding 3, gap 2, min 42; thumb `brandFill` with `onBrand` text | on / off; wraps at large text | `surfaceSunken`, `hairline`, `brandFill` | signin, members, proposals, activity, create-cabal, people | `MonacoSegmented` in `Design/MonacoSegmented.swift` (F3) |
| Periods | Same track at full width, min 44, buttons min 36: `1W 1M 3M 1Y All` | | as segmented | home, cabal, holding | `MonacoRangeChips` as a segmented variant (F3). Instrument has no periods |
| Chip | Min 36, padding `8 12`, 1pt `hairline`, capsule, `muted` text; on = `brandSoft` + `accent`, no border | on / off | `hairline`, `brandSoft`, `accent` | amount presets (`$25 $50 $100 Max`), home market updates | `MonacoChipLabel` / `Design/MonacoChipBar.swift` (F3) |
| Select (menu) | 44 tall capsule, 1pt `hairline`, `surface`, label + chevron 12, min width 85 | | `surface`, `hairline` | home and cabals boards, leaderboard, markets Sort / Filter, select-stock | `Design/MonacoMenuSelect.swift` (new, F3) over `Picker(.menu)` |
| Filter menu | `Menu` with `Toggle` rows (50 tall, checkbox) and `.menuActionDismissBehavior(.disabled)`; applies instantly (no Apply draft) | | `accent` | feed filters, leaderboard filters | `Menu` (S12, S13) |
| Switch | System `Toggle` | on / off | `brandFill`, `hairline` | settings | `Toggle` |
| Selection tick | 21 circle 1pt `hairline`; on = `brandFill` with 16 check | | `brandFill` | members (choose voters) | `CabalVoterChecklist` (S9) |
| Watch star | Round button, selected = `brandSoft` fill and filled `accent` star | | `brandSoft`, `accent` | instrument, watchlist | `star` / `star.fill` in a glass button (S11) |
| Reactions | 3 buttons min 44: heart + count, chat + count, share (right); liked = `accent` filled heart. Row 40 tall | liked / not | `accent`, `muted` | feed | `FeedItemCell` (S12) |
| Search box | Min 47, `surfaceSunken`, radius 16, icon 18, input 16, no border | | `surfaceSunken` | cabals, discover, markets, select-stock, watchlist, feed, people | `MonacoSearchField` in `Design/MonacoPrimitives.swift` (F3) |
| Field | Label 15 semibold, input `surface` + 1pt `hairline`, radius 15, min 48, helper below, error text and border in `loss` | default, invalid, helper | `surface`, `hairline`, `loss` | signin, create-cabal, invite, edit-profile, withdraw | `MonacoTextField` 56 → 48 (F3) |
| Textarea | Same chrome, min 91 | | as field | trade-amount (Thesis), edit-profile (Bio) | `TextEditor` wrapped (F3) |
| OTP | 6 boxes, gap 7, height 57, radius 13, digit 23 semibold, first has `.oneTimeCode` | ready, invalid, expired, verifying | `surface`, `hairline`, `loss` | verify | `Features/Auth/OTPCodeField.swift` (F3) |
| Amount box | Eyebrow, 48 figure centred (editable), helper, centred preset chips | insufficient (figure kept, error notice) | `Typo`/`moneyFont(.hero)`, `muted` | fund-cabal, cash-out, trade-amount, withdraw | `Design/MonacoAmountEntry.swift` (F3) |
| Keypad | 3 × 4 grid, gap 7, keys `surface` radius 14, 49 tall, digit 23 | | `surface` | fund-cabal, cash-out, withdraw | `Design/AmountKeypad.swift` 56 → 49 (F3) |
| Code field and QR | Code: monospace on `surfaceSunken` radius 13. QR placeholder: 173 square dashed (the app draws a real QR) | | `surfaceSunken` | deposit, invite | `DepositQRCode`, `MonacoWalletAddressText` (S4) |

## Content containers and rows

| Component | Anatomy and sizes | States | Tokens | Used by | SwiftUI (planned) |
| --- | --- | --- | --- | --- | --- |
| Card | `surface`, 1pt `hairline`, radius 20, padding 16; `.inset` variant `0 15` holds key rows. No shadow | | `surface`, `hairline` | public-cabal, cabal-info, transaction, deposit, instrument, trade-amount | `Design/MonacoCard.swift` (new, F4); replaces `monacoActionCard` |
| Key row | Label `muted` left, value `ink` right (max 63%), `13 0`, bottom rule | | `muted`, `ink`, `hairline` | review-proposal, cabal-info, public-cabal, transaction, deposit, withdraw, cash-out, holding | `Design/ReceiptLine.swift` (F4) |
| List row | Lead (avatar 40 / circle token 44 / ticker tile 42 / rank + avatar) + title + sub 15 `muted` + trailing + chevron 16; padding `14 0`, min 72, bottom rule except last | tappable / static; badge trailing | `ink`, `muted`, `hairline` | nearly every screen | `MonacoRow` 64 → 72 in `Design/MonacoList.swift` (F4) |
| Decision (proposal) card | Radius 20, `surface`, padding `13 13 0`. Top: cabal avatar 23–36, name, status badge. Main: ticker tile 35 + "Buy NVIDIA" 15 semibold + "$400.00 from the group pot" + Review / Status small button. Footer 44 min, top rule: chat icon + "3 yes · 1 no · 4 not voted" + chevron. Footer colour by status | open (Needs your vote), voted, executing (`warning`), completed (`gain`), expired, blocked (`loss`); loading skeleton; no-proposals quiet empty; error card with Try again | `surface`, `hairline`, `StatusBadge` | home, cabal, proposals, instrument, holding | `ProposalCard` in `Features/Proposals`, restyled by S7; same API, used by S2 and S9 |
| Chat proposal card | Avatar + "Mia proposed NVIDIA" + 2 sub lines + Review small; foot: faces + "8 replies" | open, approved | `surface` | chat, proposals | new, in `Groups/GroupChat*` (S10) |
| Cabal cover card | 300 × 218 rail, full-width × 208 list, radius 20, padding `14 15`. Cover image + flat overlay. Head: name 17 bold `onCover` + "8 members" `onCoverMuted` + vote-needed badge. Values: "Your share" 24 bold, "Ownership" 20. Rule `#FFFFFF25`. "Your P&L · All time" 18 + signed %. Foot 40: share disc 40, "Group · 1M −4.12%", chevron. Whole card taps; share separate. Large text 330 / 260 | ready, no-share ("Not funded"), pnl-unavailable ("—"), vote-needed, gain / loss; rail vs list | `coverOverlay`, `onCover`, `onCoverMuted`, `profitOnCover`, `lossOnCover`, `voteNeededFill` | home (rail), profile (rail), cabals (list) | `Design/CabalCoverCard.swift` (new, F7). Cover = solid tint + uploaded picture under a flat overlay (decisions 8, 12) |
| Cover hero | 173 tall (short 117, min 130), radius 22. h1 29 (short 25) white, members line, optional "Public cabal" badge, share disc bottom-left | | `coverOverlay`, `onCover`, `onCoverGlass` | cabal, public-cabal, cabal-info, invite | `Design/CabalCover.swift` (new, F5) |
| Money strip | Top rule. "Net worth" label + `$11,582.32` 18 bold (flex 1.4, right rule), "Cash" metric, compact Deposit | cash-only; wraps at large text | `hairline`, `moneyFont(.row)` | home, profile | `HomeBalanceSlot`, `ProfileBalanceSlot` (S2, S14) |
| Two stats | Rules top and bottom, two cells: label + figure 21 | single cell (cash-out) | `moneyFont(.card)` | cabal ("Your share" / "Cash"), balances, cash-out | `CabalSliceSlot` (S9) |
| Stat grid | 2 columns, cell label 15 + figure 17 semibold, rule under each, min 73 | | `moneyFont(.row)`, `hairline` | instrument (6 market stats), holding (6 position stats) | `Design/StatGrid.swift` (new, F6) |
| Trade asset header | Ticker tile 47 + name 23 bold + ticker `muted` (+ star on instrument) | | `Typo.section` | trade-amount, holding, instrument | `ProposeStockRow` (S6) |
| Trade context card | Radius 16 card `13 15`: cabal avatar 34 + name + "$5,100.00 cash" | | `surface` | trade-amount, fund-cabal | `ProposePotTotalRow` (S6, S4) |
| Review hero | Ticker tile 48 + "Buy NVIDIA" + `$400.00` 34 bold; cabal chip (`brandSoft`, `accent`, radius 10, `9 12`) | buy / sell | `moneyFont(.large)` | review-proposal | `ProposeReviewScreen` (S6) |
| Proposal hero | Ticker tile + status badge; h1 25 "Buy NVIDIA"; figure 35; author row with avatar 24 + P&L pill; "Big Ballers · 2h ago" | Voting open, You voted Yes, View only, Approved · Executing, Executed, Expired, Not executed, Not approved | `moneyFont(.large)`, `StatusBadge` | proposal | `ProposalDetailSlot` (S7) |
| Thesis | h3 + prose 15, line height 1.7 (sheet 1.55) | long-thesis | `bodyText` | proposal, review-proposal | `ProposalReasonSection` (S7) |
| Vote bar | "5 yes votes to pass"; bar 8 tall radius 10, gap 3 (yes `gain`, no `loss`, waiting `hairline`); labels "3 yes · 1 no · 4 not voted" | open, voted, closed | `profit`, `loss`, `hairline` | proposal, votes | `Design/VoteBar.swift` (new, F6) |
| Voter list | Sticky totals; rows avatar 36 + name + badge Yes / No / Not voted; rows 62 | open, recorded, closed, expired, rejected, empty, loading, error (notice + Retry) | `StatusBadge` | votes | `ProposalVotersView` (S7) |
| Allocation bar and holdings | 7 tall segmented bar (`accent`, `#7B91A7`, `#9A83B4`, `hairline`), holding rows (ticker tile, name, "NVDA · 30.0000 units", value) + Cash row | cash-only | `allocationTints` | cabal, balances | `Design/AllocationBar.swift` (new, F6); `CabalHoldingsSlot` (S9) |
| Chat preview card | Radius 18 card: avatar 34 + "Alex 12m" + last message + "Big Ballers · 3 new messages" + count pill + chevron | | `surface`, `brandSoft` | cabal | new (S9) |
| Member standings | Rank 18 wide + avatar 32 + name + signed % | | `profit`, `loss` | cabal | `CabalMemberBoardSlot` (S9) |
| Rank row | Position column 22 (rank + movement ↑n green / ↓n red / –), avatar 36, name + "@handle" or "12 members", value `+$12,690.00` + `+42.30%`, chevron 12; 66–78 tall | skeleton, empty, error (Retry), load more, "All n shown" | `profit`, `loss`, `muted` | home board (3), cabals board (3 + more), leaderboard (10) | `Design/MonacoBoardRow.swift`, `MonacoBoardList.swift` (F7); keep the crown for #1 |
| Board tools | Category select (or "Cabals" label) + period select (86 wide) + filter round 44; context line "Cabal P&L · 1M" + "Rank change ⓘ" | | | home, cabals, leaderboard | `MonacoBoardList` header (F7) |
| Mover tile | 244 wide, radius 18, padding 14: ticker tile 31 + name + ticker + `+4.18%` coloured; rule; "24h volume" + `$9.4M` | gain, loss | `surface`, `profit`, `loss` | markets | `MoverTile` (new, S11) |
| Stock row | Ticker tile 42 + name + ticker + price + day % + chevron; 73 tall | with price, no price (select-stock), sell (units sub) | | markets, select-stock | `MonacoStockRow` (S11) |
| Watch row | Ticker tile + name / ticker + star round | starred | | watchlist | new (S11); local store |
| Compact feed event | Radius 18 card `10 12 0`. Author row 44: avatar 32 or market icon 36, name, type pill, "+8.2% · 1M" right. Asset block radius 12 (buy / sell / price wash): ticker tile 28, ticker, name, amount. Detail line, thesis clamped to 2 lines. Reply variant: 2pt left rule + quote. Cabal variant: 44 cover thumb. Reactions row 40 | proposal (buy / sell), trade, price, reply, threshold (Approved, Execution pending), cabal, pnl-private | `buyWash`, `sellWash`, `priceWash`, `surface` | feed | `Feed/FeedItemCell.swift` (S12) |
| Event pill | Radius 6 `3 6`: Proposal, Propose sell, Buy, Sell, Price move (warn), Discussion, Approved (warn), Cabal | | `discussionTint`, `discussionWash`, `warningWash` | feed | `StatusBadge` pill variant (F4) |
| Activity item | Avatar 38 + title + body + "Big Ballers · 2h" + optional badge; unread = `brandSoft` radius-16 block | unread, read, empty | `brandSoft` | notifications | new `Notifications/ActivityView.swift` (S3), fixture-backed |
| Post summary | Author row + radius 18 card padding 19: h2 20 + prose + asset row | | `surface` | post | `FeedItemDetailView` (S12) |
| Comment | Avatar 30 + name + P&L pill + time + text + Reply link; nested indent 40 | replying, sending, failed | | post, proposal | `CommentThreadView` (S12) |
| Chat message | Theirs: avatar 26 + bubble `surfaceSunken` radius `4 17 17 17`, author above. Mine: `brandSoft`, radius `17 4 17 17`, right. Max 87%. Day label, time | sending, failed (Not sent + Retry), offline notice | `Radius.bubble` | chat | `GroupChatRowView` (S10) |
| Identity card | `identityFill`, radius 22, padding 18. Photo 64 with 2pt ring + pencil badge (28 visual, 44 hit); name 25 + handle + "Joined October 2026"; share round 38; bio; rule `#FFFFFF22`; stats "Positions value" / "Total P&L · 1M" 22 | self (pencil, positions), public (Follow below), private ("Private"), new ($0) | `identityFill`, `onIdentity*`, `pencilFill`, `profitOnIdentity` | profile, person | `ProfileHeaderSlot`, `UserProfileHeaderSlot` (S14) |
| Social counts | Three 44 tall buttons "12 followers · 8 following · 3 cabals", number 15 bold | | `muted`, `ink` | profile, person | `ProfileFollowCountsSlot` (S14) |
| Invite cover and link | 140 cover radius 18 + name 25; link box radius 12; Share link / Copy link | share, preview, invalid, expired, pending | | invite | `CabalInviteSheet`, `InviteMemberView` (S9) |
| Step row, trade-history row, disclosure | Step: 26 numbered dot `brandSoft` + title + text. Trade history: sign tile 35 radius 11 (`buyWash` / `sellWash`) + "Buy · 24.4444 units" + date + total + chevron. Disclosure: radius 14, 1pt `hairline`, summary 48 tall | open / closed | `buyWash`, `sellWash`, `hairline` | deposit, holding, proposal | `DepositView`, new holding view (S4, S10), `DisclosureGroup` (S7) |

## Marks, badges, feedback

| Component | Anatomy and sizes | States | Tokens | Used by | SwiftUI (planned) |
| --- | --- | --- | --- | --- | --- |
| Avatar | Circle; sizes 24 26 30 32 34 36 38 40 64 72 76 80; optional 2pt ring; faces overlap −6 with a 2pt `surface` ring | photo, initials, default animal (stays as today) | `surface` | everywhere | `Design/MonacoAvatar.swift` (F5) |
| Circle token | 44 tile, radius 14 (`size × 0.32`), `brandSoft` fill, `accent` glyph | | `brandSoft`, `accent` | activity, balances, deposit | `SunkenGlyphMark` retuned in `Design/MonacoMarks.swift` (F5) |
| Ticker tile | 42 radius 14; issuer logo first, fallback `surfaceSunken` tile with ticker | | `surfaceSunken`, `ink` | all stock rows | `StockMark` (F5); no per-ticker tints |
| Wordmark | "monaco" in the Home toolbar and sign-in, 110 wide | light / dark | `ink` | home, signin | SF Pro Bold text as `Auth/MonacoMark.swift` does today, until a vector arrives (decision 9) |
| Notice | Radius 15 `13 14`, icon 18 + text 15 (+ bold title) | info (`brandSoft` / `accent`), warn (`warningWash` / `warning`), error (`loss` wash / `loss`) | as listed | ~25 screens (stale, insufficient, offline, submitting) | `Design/MonacoNotice.swift` (new, F4) |
| Badge | Radius 8 `4 7`, 15 semibold | default (`brandSoft`), negative (`loss` wash), warn, neutral (`surfaceSunken` / `muted`) | as listed | decision cards, voter list, proposals, transaction, notifications | `Design/StatusBadge.swift` (new, F4) |
| P&L inline pill | Radius 6 `3 5`, `brandSoft` fill, coloured text ("+8.2% · 1M", "P&L private" muted) | positive, negative, muted | `brandSoft`, `profit`, `loss` | proposal author, comments, feed author | `StatusBadge` `pill` variant (F4) |
| Count pill | 21 circle, `brandSoft`, `accent` text | | `brandSoft`, `accent` | section heads, chat preview | `MonacoSectionHeader` (F4) |
| Vote-needed badge | Radius 10, `voteNeededLabel` on `voteNeededFill` | | | cabal cover cards | `CabalCoverCard` (F7) |
| Toast | `toastFill` panel, `toastLabel` text, radius 15, `13 17`, inset 20 | auto-hide per `MonacoToastTiming` | `toastFill`, `Shadow.floating` | copy address, share, like, photo | `Design/MonacoToastBanner.swift` (F4) |
| Empty state | Centred: icon tile 68 (`brandSoft`, radius 23, icon 31 `accent`), h2 22, text max 280, up to two full buttons (primary, outline) | 0 / 1 / 2 buttons | `brandSoft` | ~30 screens | `EmptyState` in `Design/MonacoList.swift` (F4) |
| Outcome | Orb 70 (radius 25) + h1 28 + text max 290 | done (`brandSoft` check), pending (warn clock), error (alert) | `brandSoft`, `warningWash` | fund-cabal, cash-out, withdraw, card-funding, deposit, invite, review-proposal | `Design/OutcomeView.swift` (new, F4) |
| Skeleton | Title bar 42 × 70%; bar 16; block 126 radius 21; row = 42 circle + two lines; ranking row = 36 circle + two bars. 1.2 s pulse | every loading state | `surfaceSunken` | all | `Design/MonacoFeedback.swift`: `SkeletonBlock`, `MonacoRowSkeleton`, `BoardRowSkeleton` (F4) |
| Generic error | Empty state with wifi icon, "Couldn't load this view", Try again | every error state | | all | `MonacoErrorRow` restyled as a notice (F4) |
| Partial error card | Decision-card shape with wifi icon, title, helper, small Try again | | | home and cabal proposals-error | `ProposalCard` error variant (S7) |
| Quiet empty | Inline line with 16 icon, `10 0` | | `muted` | proposals section, instrument | `Design/MonacoList.swift` (F4) |

## Charts

| Component | Anatomy and sizes | States | Tokens | Used by | SwiftUI (planned) |
| --- | --- | --- | --- | --- | --- |
| Performance line | 170 tall (small 148); stroke 1.7; line only, no fill under it (decision 12); end dot 3.5; tooltip pill; dates row (start, end); periods control below | gain, loss, quiet chart (104 tall `surface` radius 14, "No investment history") | `profit`, `loss`, `ChartStyle` | home, cabal, holding, instrument (no periods, "24h ago") | `Design/CurveScrubChart.swift`, `MonacoLineStyle.swift`, `MonacoChartHold.swift` restyled (F6); keeps crosshair, pinch and pan (decision 4) |
| Quiet chart | 104 tall placeholder | | `surface` | home, cabal | `Design/QuietChart.swift` (new, F6) |
| Candle chart | Not in the design | | | instrument | `MonacoCandleChart` stays, with intervals, pinch and pan; restyle colours only (decision 4) |

## Gallery

F8 adds `Design/MonacoPreviewGallery.swift` (tabs: tokens, type, controls, surfaces, marks, charts, cards) and `scripts/qa/sample-screens.txt` lines so the screenshot harness can capture each component next to its reference.
