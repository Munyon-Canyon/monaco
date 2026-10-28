# Monaco design language (iOS)

The look in one sentence: **Apple native, dark first: a black (or white) canvas, system colours, one forest-green tint, a neon gain and a bright loss; dense flat lists; money is the biggest thing on every screen.**

This is design option F, "Native", which the founder chose on 10-26. The [F package](design/f/README.md) holds the frames, the gallery, the [notes](design/f/notes.md) with the build map and fixture table, and the [kit](design/f/kit.md). This page is the rule set; the package is the picture.

Kept: SF Pro only, tabular figures, every colour and font a token, the 15pt text floor, the slot architecture, toasts for confirmations, the product words, pixel-animal default avatars, the "monaco" wordmark as Home's title, the rounded profile card, native chart behaviour (candles, intervals, pinch and pan, crosshair).

Gone: the v3 paper canvas and its ink-green text, cards drawn with a border everywhere, capsule P&L pills, cover-art cabal cards with text on a scrim (rows lead with the cabal's **crest**; the cabal page keeps its picture as a plain header with nothing drawn on it), the dark identity card's palette (the profile card is a `surface` card at radius 12), segment pickers for sections, range pills, the chart gradient, hand-tuned glass fills.

## Principles

Each one is a check you run on a screenshot. Frames: [`01-tokens-controls`](design/f/screens/01-tokens-controls.png) shows the controls, [`02-home-dark`](design/f/screens/02-home-dark.png) shows them together.

1. **The number is the hero.** Home's net worth is 32 semibold (`moneyFont(.home)`). The keypad's amount is 44 (`moneyFont(.hero)`). Elsewhere, a screen's one hero figure is the 28 display figure (`moneyFont(.display)`). The cents are secondary colour.
2. **Colour means one thing.**
   - `gain` and `loss` are money moving and Yes / No.
   - `tint` (forest) is "you can tap this" or "selected".
   - `brand` fills the one prominent button.
   - `crown` is #1 only.
   - An identity hue is a person or cabal without a picture, and is never green.
   - Nothing is a gradient.
3. **Flat lists, few cards.** Lists sit on the canvas with an inset hairline that starts at the label column. A card (`cell` fill, radius 12, no border, no shadow) holds one object only: the vote card, the cash row, the stat row, a settings group, the profile card. Never stack two separators.
4. **One prominent button per screen**: 50pt, radius 10, `brand` fill. A button row holds at most two buttons.
5. **Two lines per row, one trailing fact.** A figure over its change counts as one fact. Previews show four or five rows plus "See all".
6. **Images everywhere.** Ticker logos, people's photos (circles), and cabal crests (a square cut of the cabal's picture, radius 6). Without a picture, a crest is the cabal's initials on its identity hue (cabals have no symbol field, so the kit's "one symbol" is not built; founder, 10-27: Q22).
7. **Native chrome.**
   - the system tab bar;
   - large titles that fold into the inline bar;
   - at most one underline-tab row per screen, for sibling sections of that one screen;
   - chart ranges and periods (the leaderboard's Week · Month · All time) as plain labels;
   - chips as capsules, at most four;
   - sheets at the half detent (the accessibility-size exception belongs to the sheet helper, #6470);
   - toasts in the nav row.
8. **Votes are live and held.** Counts (✓ n / ✕ n) show in every state, before and after you vote. Yes is never pre-highlighted. You pick, then hold for 0.6s; lifting cancels. Proposals settle at quorum.
9. **A face for every act.** Anything a person did (proposed, voted, bought, joined) shows their avatar; anything a cabal did shows its crest.
10. **Every Feed post takes replies.** Reactions and the reply control end every post's reaction row; reply reactions are per-emoji chips; every chip, quick emoji and text Reply is 44 × 44 to tap. Replies carry text, emoji, GIFs (GIPHY), polls, a quoted thesis or a shared stock; chats take no image uploads.

## Colour

Tokens live in `apps/mobile/Monaco/Design/MonacoTheme.swift`; feature code uses the role names, never a hex value. Values come from [`01-tokens`](design/f/screens/01-tokens.png). #6432 owns the colour code and the final names; if they differ, #6432 updates this table in its PR.

| Role | Token | Dark (default) | Light | Used for |
| --- | --- | --- | --- | --- |
| Canvas | `background` | `#000000` | `#FFFFFF` | every screen; light is pure white, never off-white |
| Cell | `surface` | `#1C1C1E` | `#F2F2F7` | the one-object card, fields, keypad keys |
| Fill | `surfaceSunken` | `#2C2C2E` | `#E5E5EA` | search field, skeletons, other people's bubbles |
| Separator | `hairline` | `#38383A` | `#C6C6C8` | the inset row rule |
| Label | `primaryText` | `#FFFFFF` | `#000000` | titles, figures, row titles |
| Secondary | `secondaryText` | `#A1A1A6` | `#636366` † | sub-lines, captions, cents, resting range labels |
| Tertiary | `tertiaryText` | `#6E6E73` | `#AEAEB2` | glyphs only, never text |
| Tint | `tint` | `#4DAE78` | `#1E5C3A` | tappable and selected: links, selected chip, underline tab |
| Brand | `brandFill` | `#1E5C3A` | `#1E5C3A` | the one prominent button (white text, 7.9:1) |
| Gain | `profit` | `#30F57A` | `#00773A` † | gains, Yes |
| Loss | `loss` | `#FF6259` † | `#C80014` † | losses, No, danger |
| Crown | `crown` | `#FFD60A` | `#FFCC00` | #1 only |

Identity hues, the same in both appearances: indigo `#5E5CE6`, orange `#FF9F0A`, teal `#40C8E0`, pink `#FF375F`, purple `#BF5AF2`, brown `#AC8E68`, blue `#0A84FF`, cyan `#64D2FF`. They colour a person or a cabal without a picture, picked by a stable hash of the id; never green. Default avatars and cabal crests are never Monaco green.

The identity hues' label (`onFill`) is `#FFFFFF` on indigo and `#000000` on the other seven †. The vote-needed badge is a red count: `voteNeededFill` `#D70015` † in both appearances with a `#FFFFFF` label. Chart strokes `profitVivid` and `lossVivid` keep the board's values (`#00843D` / `#30F57A`, `#D70015` / `#FF453A`). `onDestructive` is `#FFFFFF` in light and `#000000` in dark. The allocation bar's blue and purple are `#0040DD` † and `#8944AB` † in light and the board's `#0A84FF` and `#BF5AF2` in dark.

† marks a value corrected one step from the board so the pair clears AA (founder, 10-27: Q23). The board's `secondaryText` `#6C6C70` is 4.16:1 on a fill; `#636366` gives 5.99 / 5.37 / 4.77 on canvas / cell / fill. The board's light gain `#00843D` is 4.31 on a cell and 3.83 on a fill; `#00773A` gives 5.68 / 5.09 / 4.52. The light loss `#D70015` is 4.29 on a fill and the dark `#FF453A` is 4.09; `#C80014` gives 6.06 / 5.43 / 4.83 and `#FF6259` gives 7.14 / 5.79 / 4.74. White on the board's badge red `#FF3B30` is 3.55:1 under the 15 pt semibold count; `#D70015` (the board's own light loss red) gives 5.38:1. White fails 4.5:1 on seven of the eight hues (1.72 to 3.65), so seven take black: orange 10.22, teal 10.56, pink 5.96, purple 5.96, brown 6.82, blue 5.76, cyan 12.20, and indigo takes white at 5.06. The light allocation blue and purple were 2.14 and 2.07 against the cash track; the corrections give 4.43 and 3.54 there.

The contrast contract, shared by #6432 and #6585 and asserted by `DesignContrastTests`:

| Role | Bar | Pairs |
| --- | --- | --- |
| Text, any size or weight | 4.5:1 on canvas, cell and fill, light and dark | `primaryText`, `secondaryText`, `tint`, `profit`, `loss` (`destructive`), `warning`; `onBrand` on `brandFill`; `onDestructive` on `destructive`; `voteNeededLabel` on `voteNeededFill`; `onFill` on each of the eight hues; `toastLabel` on `toastFill` |
| Meaningful non-text marks | 3:1 against what they sit on | the chart line (`profitVivid`, `lossVivid`) and the flat line (`secondaryText`); the `AllocationBar` segments against canvas and their cash track; toast glyphs at 4.5 |
| Decorative, exempt | none | `tertiaryText`: glyphs only, a glyph that carries meaning uses `secondaryText`. `crown`: the rank and its accessibility label say it. The identity hue as a fill against the canvas, and its stroke: the cabal's name sits beside every crest |
| Inactive, held quiet | [2.5, 4.5) on the three surfaces and at least 1 below `secondaryText` | `disabledLabel` |
| System-drawn, not a token | measured by #6585's `.contrast` UI audit | the system tab bar and its badge, system materials |
| Hue separation | asserted | no identity hue in the green band (OKLab 120° to 170°); every hue at least 30° from `profit`; `profit` and `loss` at least 90° and ΔE 0.2 apart; `tint` and `profit` ΔE ≥ 0.08 (light 0.087, dark 0.20) |

The v3 checks that every pair of identity hues is 60° apart and that no hue sits within 30° of `loss` are removed: F's eight hues are the board's, pink `#FF375F` sits 11° from `loss`, a crest never carries a figure and a loss always carries "−".

Ticker tiles keep the issuer logo, with a `surfaceSunken` tile and `primaryText` as the fallback.

## Type

One family, SF Pro, at the size floor #4110 set after hands-on QA found text too small. A role at a system text style's size uses that style, so Dynamic Type scales every role. Every figure is `.monospacedDigit()`. No letter-spacing overrides.

**The 15pt floor (founder, 10-27: Q1):** the frames draw row sub-lines, stat labels and captions at 13pt; the app sets them at 15pt, so screens come out a little taller than the frames. Never shrink text to match one.

From [`01-tokens-type`](design/f/screens/01-tokens-type.png). #6440 owns the type, spacing and radii code; if its final names differ, #6440 updates these tables in its PR.

| Role | Token | F size / line · weight | App size | Used for |
| --- | --- | --- | --- | --- |
| Large title | `Typo.display` | 28/34 bold | 28 | root tab titles |
| Title 1 | `Typo.title` | 28/34 bold | 28 | moments ("You're in.") |
| Title 2 | `Typo.title2` | 22/28 bold | 22 | the name on a pushed screen (cabal, stock, proposal) |
| Display figure | `moneyFont(.display)` | 28/34 heavy | 28 | a screen's one hero number: treasury, price |
| Section | `Typo.section` | 17/22 semibold | 17 | section and sheet titles |
| Button | `Typo.button` | 16/21 semibold | 16 | buttons |
| Figure | `moneyFont(.card)` | 20/25 semibold | 20 | a figure in a tile or card |
| Rank | `Typo.rank` | 15/20 semibold | 15, tabular | rank numerals in rows and on the podium |
| Row title | `Typo.headline` | 15/20 semibold | 15 | row, post, comment and tile titles; row values |
| Feed post ticker and amount | `Typo.section` | 17/22 semibold (Headline) | 17 | the line a 36 logo leads; lines without a logo keep `Typo.headline` 15 |
| Content | `Typo.body` | 15/20 regular | 15 | thesis, comments, sheet and empty-state copy |
| Row sub | `Typo.subhead` | 13/18 regular | **15** (floor) | second lines, trailing changes, stat labels |
| Caption | `Typo.caption` | 13 | **15** (floor) | timestamps, chips, chart dates |
| Home figure | `moneyFont(.home)` | 32/36 semibold | 32 | Home's net worth only (founder, 10-26) |
| Money hero | `moneyFont(.hero)` | 44/48 semibold | 44 | the keypad's amount only |

Rules:

- Nothing under 15pt in the app. System surfaces keep the system's sizes: status bar, inline nav title, tab bar, keyboard, menus, notifications.
- No caps except ticker tiles and system group headers.
- No SF Mono except the wallet address, inside `MonacoWalletAddressText` only.
- Tabular figures wherever money or a count appears.

## Spacing, radii, elevation

- Spacing, on a 4pt grid: margin 16 (`Space.gutter`), card padding 12, row padding 10 × 16 with a 44 tap minimum, section gap 16, root block gap 24, title gap 6, stack gap 8, button 50 (small 32 visible on a 44 target), tab clear 104, home clear 40.
- The Feed post's reaction chip is 24 visible on a 44 × 44 tap (padding 8, 4 between chips, at least 44 wide; `ReactionChipMetrics`, #6440). A reply's compact chip is 22 visible on the same 44 × 44 tap. The Feed post's logo is 36.
- Radii, all continuous:

  | Element | Radius |
  | --- | --- |
  | cell, card | 12 |
  | button | 10 |
  | share card | 16 |
  | small: field, stat row, podium step, rank badge | 8 |
  | ticker tile | 6 |
  | logo | 15% of its size (the Feed post's 36 logo is 5.4) |
  | keypad key | 8 (press highlight only) |
  | chip, reaction, toast, search field, composer, nav glass | capsule |
  | crest | 6 |
  | cabal squircle | 22.5% |
  | sheets | the system radius |

- Elevation: none. No shadows, no gradients, no borders around cards; the system tab bar's own shadow is the only one.

## Materials and transparency

- Only system materials, on chrome only: the tab bar, the 44pt round toolbar buttons, sheets, the toast capsule and the docked Deposit shelf. The composer's dock is opaque.
- Keep the system `TabView`. On iOS 26 it is the floating system tab bar; on iOS 18, the standard bar. Never hand-draw a tab bar.
- With `accessibilityReduceTransparency` on, every material becomes `surface`.
- Never draw a status bar, a home indicator or a blur compositor.

## Icons and motion

- SF Symbols, regular weight, named through `MonacoIcon` so screens never type symbol strings: 22 in rows, 20 in bar circles, 17 inline, 15 in count lines and chips.
- Keep today's motion and haptics, plus: the hold is 0.6s with a ring or sweep showing progress; lifting cancels; `.success` plays on commit; the count ticks on commit.
- The toast shows for at least 4s, and under VoiceOver until its Undo window ends (the frame says 2.5s; founder, 10-27: Q24). An ordinary vote's toast has Undo; the vote that settles a proposal has none.
- Reduce Motion keeps the hold (it is how you vote) but draws its progress without the sweep animation.

## Charts

From [`01-chart-variants`](design/f/screens/01-chart-variants.png).

- The line chart runs full width on the canvas with a **flat 10% tint** of the line colour under it and an end dot. Sparklines are line only.
- Ranges are plain labels: 1H 1D 1W 1M 3M 6M All, the selected one in `primaryText` semibold, the rest secondary.
- The gradient variant is for comparison only and is never shipped.
- Candles, intervals, pinch and pan, and the crosshair stay.

## Decisions

The founder made these decisions and they are final. Tickets apply them and do not reopen them. They are listed in the tracking issue (#6410) and in §1 of [notes.md](design/f/notes.md); where a note and this page differ, the 10-26 and 10-27 notes win.

1. **F Native is the app.** One Apple-native foundation: system colours and materials, large titles, flat lists, native rows, sheets at the half detent, the system tab bar, SF Symbols.
2. **Two phases.** Phase 1 (launch) is every in-app screen plus notifications whose Yes / No open the hold sheet. Phase 2 (later) is widgets, StandBy, Live Activities and the Dynamic Island, the Follow control, Spotlight, Siri, seasons and streaks. Phase 2 frames are reference and are never built in this milestone. Frames the designer did not choose are not in the repo and are never built.
3. **Dark first.** Dark is the default and the priority. Light is pure white (`#FFFFFF`) with colour as accent.
4. **Colour:** black (or white) canvas, one forest-green tint used sparingly, a separate neon gain green and bright loss red, a crown gold.
5. **No gradients anywhere.** The chart fill is a flat 10% tint. Only the tab bar carries a shadow.
6. **Squarer and denser** (10-23): cards 12, buttons 10, share card 16, small controls 8. Lists sit flat on the canvas. Two lines per row, one trailing fact.
7. **Type:** SF Pro only. Home's figure is 32 (founder's choice, 10-26). The keypad keeps the 44 money hero.
8. **Underline tabs** only for sibling sections of one screen, at most one row per screen. Chart ranges and the leaderboard's periods are plain labels. Chips are capsules, at most four.
9. **Half-height sheets.** Every sheet opens at about half the screen.
10. **Live counts and hold to vote.** Everyone sees ✓ n / ✕ n before and after they vote. A vote is a pick, then a 0.6s hold. Yes is never pre-highlighted. Proposals settle at quorum.
11. **Toasts over popups.** An ordinary vote's toast has Undo; the vote that settles a proposal has none (founder, 10-27).
12. **Keep:** the 3.5% APY label on cash rows, the Propose "Trading bot" row, multichain withdraw (Solana, Base, Arbitrum, Ethereum), the Phantom keypad, docked Deposit, the rounded profile card, the "monaco" wordmark as Home's title.
13. **Comments and GIFs on every Feed post** (10-27). Any post can be replied to from the Feed; reply reactions are per-emoji chips; GIF sits in both reply sheets; every chip, quick emoji and text Reply is 44 × 44 to tap. **No image uploads in chats** for the first version (10-27 evening): a reply is text, emoji reactions, a GIF (from GIPHY through its iOS SDK, rated pg-13, never an upload), a Poll, a quoted thesis or a shared stock. The composer's ⊕ menu is Poll · Quote the thesis · Share a stock, with no Photo. Profile and cabal pictures are not chats and stay. **Feed post sizes** (10-27 evening): reaction chips 24 visible on a 44 × 44 tap, the asset logo 36, the ticker and amount line Headline 17.
14. **The 15pt text floor (#4110) stays** (founder, 10-27: Q1). F draws row sub-lines, stat labels and captions at 13pt; the app sets them at 15pt, and parity accepts the extra height.

Other founder answers of 10-27, each by its number in the tracking issue:

- **Q19, Q20:** the two exceptions to "no crypto words", under Composition rules.
- **Q21:** sheets take the large detent at accessibility text sizes; the sheet helper (#6470) owns it.
- **Q22:** a crest without a picture is the cabal's initials on its identity hue.
- **Q23:** AA corrections to the colour values are #6432's, written back to the Colour table.
- **Q24:** the toast stays at least 4s.
- **Q25:** screens with no F frame, below.

### Screens with no F frame

Sign in and verify, onboarding, Settings and its children, Add money (deposit methods), Cash out, the trading bot screens, cabal chat, Create cabal, Edit profile, Friends, Referrals, Moderation and Report.

These take F's tokens and shared controls (#6440 to #6470, #6432) with no screen ticket of their own. They are judged against the nearest F screen: settings-style lists against [`03-cabal-rules`](design/f/screens/03-cabal-rules.png), amount entry against [`09-fund-cabal`](design/f/screens/09-fund-cabal.png), forms against [`08-review-sheet`](design/f/screens/08-review-sheet.png).

## References

- Renders: `docs/design/f/screens/<frame>.png`, 393 × 852pt at 2x; Dark unless the name ends `-light`.
- Captions and groups: `docs/design/f/index.html`, opened from a checkout.
- What each frame becomes, and who builds it: the build map in [notes.md](design/f/notes.md).
- Also drawn in the package: [`04-composer-menu`](design/f/screens/04-composer-menu.png) (the ⊕ menu), [`05-feed-A`](design/f/screens/05-feed-A.png) (the Feed post at its final sizes), and [`01-tokens-later`](design/f/screens/01-tokens-later.png) (phase 2 only: Follow, the streak row, the season row; never built here).
- **Parity** means the Launch frame's order, tokens, controls, copy and states match, in Dark and in Light, allowing the extra height the 15pt floor causes, with real data, real photos and real crests in place of the fixtures. Later frames are never parity targets in this milestone.

## Composition rules

- Labels are sentence case.
- Less text: icons where a word repeats (✓ / ✕ with counts, copy and share glyphs).
- Fine print goes in an i-sheet.
- No "Season" or streak anywhere at launch.
- Copy: "cabal", never club or group; "Deposit" for money in; "Fund this cabal" for moving cash into a cabal; "Propose buy" / "Propose sell" (no "a").
- No crypto words: USDC shows as dollars, and chain names appear only in the Withdraw network chips.
- Two founder exceptions (10-27): "USDC" may appear in the cash label ("Cash · USDC", Q19) and on Withdraw ("≈ 120.00 USDC · fee $0.50", "USDC on Solana"), each from one copy constant; chain names may appear in the Market stats i-sheet ([`12-info-sheet`](design/f/screens/12-info-sheet.png), Q20), whose audit exception #6562 owns. Every other surface keeps the rule.
- Sample harnesses cover every state; `scripts/qa/screens.sh` is the acceptance gallery.
