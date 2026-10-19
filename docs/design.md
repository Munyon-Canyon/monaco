# Monaco design language (iOS)

The look in one sentence: **cream paper, forest ink, one green accent, glass chrome; money is the biggest thing on every screen.**

This is the UI v3 direction, adopted wholesale from the [v3 reference package](design/v3/README.md). The package holds the prototype, the reference renders, and the exact values in [tokens.md](design/v3/tokens.md) and [components.md](design/v3/components.md). This page is the rule set; the package is the picture.

Kept: SF Pro only, tabular figures, every colour and font a token, the five cabal tints (now also the cover gradient), the slot architecture, toasts for confirmations, the product words, pixel-animal default avatars, the 15pt text floor. Gone: the flat white / near-black canvas, the rows-not-boxes rule, the no-capsules rule, the old 56-point primary bar, the four-disc action row.

## Principles

Each one is a check you run on a screenshot.

1. **The number is the hero.** On every money screen the largest thing is a figure, set bold and tabular (`moneyFont(.hero)`).
2. **Colour means one thing.** Green text or a green-on-wash pill is a gain, red is a loss. The forest accent (`brand`) means "you can tap this" or "selected". A cabal tint is identity. P&L may sit inside a capsule pill (decision 1).
3. **Cream canvas, bordered cards.** Content groups sit in `surface` cards with a 1px `hairline` border, radius 20 (18 for compact cards). Lists inside a card are ruled rows. Cards carry no shadow.
4. **One primary action per screen.** A 48pt capsule (`.monacoPrimary`) names the outcome, and sits pinned in the glass dock when the screen scrolls. Everything else is a row, a text link, an outline or soft button, or a toolbar glyph.
5. **Cabals look like places.** A cover is the cabal's tint gradient plus its uploaded picture under a dark scrim, with white text. No stock photography (decision 8).
6. **Glass is chrome only.** It appears on the tab bar, the 44pt round toolbar buttons, sheets and the dock. Content is never glass. Reduce Transparency swaps every glass surface for `surface` (decision 6).
7. **A face for every act.** Anything a person did (proposed, voted, bought, joined) shows their avatar; anything a cabal did shows its mark.

## Colour

Tokens live in `apps/mobile/Monaco/Design/MonacoTheme.swift`; feature code uses the role names, never a hex value. `MonacoContrastTests` holds every text pair to AA on all three surfaces. Values come from [tokens.md](design/v3/tokens.md), except the three AA-corrected light values marked †.

| Role | Token | Light | Dark | Used for |
| --- | --- | --- | --- | --- |
| Canvas | `canvas` | `#FAF7F0` | `#03130F` | every screen, composer |
| Text | `ink` / `primaryText` | `#0C3526` | `#F5F8F4` | headings, hero figures, row titles |
| Body text | `bodyText` | `#172C24` | `#E6EFEB` | prose, chat, thesis |
| Secondary text | `secondaryText` / `muted` | `#646D67` † | `#9EAFA5` | captions, labels under figures, resting tab labels, chevrons |
| Separator | `hairline` / `border` | `#E5E7DF` | `#18342A` | 1px rules, card borders, vote-bar track |
| Surface | `surface` | `#F6F7F2` | `#0A2119` | cards, fields, keypad keys, mover tiles |
| Sunken | `surfaceSunken` | `#EDEFE7` | `#102A20` | search box, segmented track, skeletons, other's chat bubble |
| Accent text | `brand` / `accent` | `#07643E` | `#60EFAA` | links, selected chip text, "on" tab, notice text, focus |
| Accent soft | `brandSoft` | `#E8F0E7` | `#14372A` | secondary button, selected chip, count pill, my chat bubble, action tiles, row glyph tile |
| Accent fill | `brandFill` | `#0D5C3A` | `#66EFAB` | primary button, selected segment, switch on, send disc |
| On accent | `onBrand` | `#FFFFFF` | `#072219` | text on `brandFill` |
| Gain | `profit` | `#0F7A4E` † | `#68EBAB` | gain text, "yes" votes |
| Gain vivid | `profitVivid` | `#108153` | `#68EBAB` | chart stroke and fill |
| Loss | `loss` / `destructive` | `#B54647` † | `#F1807F` | loss text, "no" votes, danger, invalid border |
| Loss vivid | `lossVivid` | `#BD5455` | `#F1807F` | chart stroke and fill |
| Warning | `warning` | `#866017` | `#E6C477` | pending, stale, warning badge text |
| Warning wash | `warningWash` | `#F3ECD8` | `#2C2819` | warning badge and notice fill |
| Scrim | `scrim` | `#01150E` at 0.33 | `#000000` at 0.73 | batch-sheet overlay |
| Glass | `glassFill` | `rgba(251,252,247,.86)` | `rgba(12,35,27,.85)` | tab bar, round toolbar buttons, dock (design intent; see Glass) |
| Glass stroke | `glassStroke` | `rgba(255,255,255,.98)` | `rgba(176,234,200,.2)` | tab bar edge |
| Sheet glass | `sheetGlass` | `rgba(248,250,245,.90)` | `rgba(17,41,31,.94)` | sheet fill (design intent) |
| Sheet stroke | `sheetStroke` | `rgba(255,255,255,.95)` | `rgba(168,227,194,.21)` | sheet edge |
| Sheet dim | `sheetDim` | `rgba(0,14,8,.3)` | `rgba(0,6,4,.48)` | overlay behind a sheet |
| Buy wash | `buyWash` | `#E9F4EC` | `#0E3022` | feed buy block, trade-history buy tile |
| Sell wash | `sellWash` | `#F8ECE9` | `#30201F` | feed sell block, trade-history sell tile |
| Price wash | `priceWash` | `#EDF0F9` | `#13232B` | price-move block (border only today) |
| Vote needed | `voteNeededLabel` on `voteNeededFill` | `#083123` on `#C8F7CF` | same | "1 vote needed" badge on cover cards |
| Discussion | `discussionTint` on `discussionWash` | `#535AAE` on `#676BDD` at 0.075 | `#C7B9F7` on `#BCA4FF` at 0.10 | feed "Discussion" pill |
| Allocation | `allocationTints` | `brand`, `#7B91A7`, `#9A83B4`, `border` | same | allocation bar segments 1 to 4 |
| Toast | `toastFill` / `toastLabel` | `#0C3526` / `#FAF7F0` | `#F5F8F4` / `#03130F` | the toast and the chart tooltip pill invert the scheme |
| Identity card | `identityFill` | `#0C3828` | `#0D2C21` | Profile identity card |
| On identity | `onIdentity` | `#F8FAF5` | `#F5FAF6` | name and figures on the identity card |
| On identity, muted | `onIdentityMuted` | `#B6D5C3` | same | handle, stat labels, joined date |
| On identity, body | `onIdentityBody` | `#E0EEE3` | same | bio |
| P&L on identity | `profitOnIdentity` / `lossOnIdentity` | `#81E8B1` / `#FFAAAA` | same | P&L on the identity card |
| Pencil badge | `pencilFill` | `#E6F5E8` (glyph `identityFill`) | same | edit badge on the identity avatar, 2pt ring in `identityFill` |
| On cover | `onCover` | `#FFFFFF` | same | text on a cover picture |
| On cover, muted | `onCoverMuted` | `#C5D1CB` | same | members line, labels, "—" on covers |
| On cover, glass | `onCoverGlass` | `#FFFFFF` at 0.09, border at 0.14 | same | share disc on a cover |
| P&L on cover | `profitOnCover` / `lossOnCover` | `#83F1B9` / `#FFB2B0` | same | P&L on a cover card |
| Cover stroke | `coverStroke` | inset `#FFFFFF` at 0.07 | same | cover card inner stroke |
| Cover scrim | `CoverScrim` | `.memberCard`, `.card`, `.hero`, `.solid` | same | gradients under cover text; `.solid` is flat `#00130B` at 0.91 under Reduce Transparency; `.memberCard` ends `#000C09` at 0.96 (all stops in `MonacoTheme.CoverScrim`) |
| Cabal tints | `CabalTint` ×5 | pine, ochre, plum, indigo, moss | | marks, cover gradients, chart lines, allocation bar |

† The package's light values for these three are `#6B746E`, `#108153` and `#BD5455`. They fail AA on `surfaceSunken` (4.16, 4.22 and 3.97 : 1; `loss` also fails on `canvas` and `surface`), so they are darkened one step to 4.61, 4.62 and 4.60 : 1 on `surfaceSunken` (5.00, 5.01 and 4.99 on `canvas`). The vivid chart colours are not text and keep the package values.

`tertiaryText` is `#646D67` light (equal to `secondaryText`: there is no AA room to be quieter) and `#8E9E95` dark. `disabledLabel` is `#8F9791` light and `#5E6B63` dark, held in [2.5, 4.5) on all three surfaces. `gold` and `goldGlyph` are unchanged. `cashFill` keeps `#76837B` / `#7A8B80`: the holdings tests need 3:1 against every surface, which the package's `border` value cannot give. `toastStroke` is clear.

`profit` and `brand` share a family on purpose in v3, so `profitAndLossCannotBeConfused` only holds profit apart from loss (hue gap at least 90°, ΔE OKLab at least 0.2); the old brand-versus-profit gate was replaced under decision 1.

Ticker tiles keep the issuer logo, with a `surfaceSunken` tile and `ink` text as the fallback. Do not copy the prototype's per-ticker tints.

## Type

One family, SF Pro, at the size floor #4110 set after hands-on QA found text too small. A role at a system text style's size uses that style; display, headline and body pre-scale with `Typo.scaled`, so Dynamic Type scales every role. SF Pro Display engages at 20pt and up. Every figure is `.monospacedDigit()`. No letter-spacing overrides.

**The 15pt floor (decision 2):** the references are drawn with 10–13px captions; the app sets them at 15pt, so screens come out a little taller than the references. Never shrink text to match one.

| Role | Token | Size | Weight | Used for |
| --- | --- | --- | --- | --- |
| Hero number | `moneyFont(.hero)` | 44 | Bold | portfolio total, pot value, stock price, amount entry |
| Large title | `Typo.display` | 30 | Bold | tab root titles, sign-in wordmark, profile name |
| Large number | `moneyFont(.large)` | 35 | Bold | proposal amount, your slice, review figure |
| Card number | `moneyFont(.card)` | 22 | Bold | card share figures, identity stats, two-stats cells |
| Title | `Typo.title` | 28 | Semibold | headings in content, stock name, keypad digits |
| Section | `Typo.section` | 22 | Semibold | section titles, sheet titles |
| Headline | `Typo.headline` | 18 | Semibold | row titles, button labels, card headlines |
| Body | `Typo.body` | 18 | Regular | prose, chat, reasons |
| Row number | `moneyFont(.row)` | 18 | Semibold | prices and values in rows |
| Subhead | `Typo.subhead` | 17 | Regular | row subtitles, helper lines, labels under figures |
| Subhead strong | `Typo.subheadStrong` | 17 | Semibold | text links, chip and segment labels, toast text |
| Small number | `moneyFont(.caption)` | 15 | Semibold | day change, return under a value |
| Caption | `Typo.caption` / `Typo.captionStrong` | 15 | Medium / Semibold | timestamps, pills, tab labels, chart dates, attribution |

`moneyFont(.card)` scales against `.title2` with a 0.7 minimum scale factor. Line heights follow the system style. Everything else keeps its size.

Design role to app role, for reading a reference:

- Hero figure 43 → `moneyFont(.hero)`.
- Root title 30 → `Typo.display`.
- Cover hero 29, outcome 28, identity name 25 → `Typo.title`.
- Section head 19 and sheet title 22 → `Typo.section`.
- Card name 17, toolbar title 16, button 15 and row title 15 → `Typo.headline`.
- 13–14px text → `Typo.subhead`.
- 10–12px captions, pills, tab labels and chart dates → `Typo.caption` / `Typo.captionStrong` at 15pt.

Tickers set in `Typo.subhead` secondary. Wallet addresses are the one monospaced text, inside `MonacoWalletAddressText` only.

## Spacing, radii, elevation

- Spacing: 4 · 8 · 12 · 16 · 20 · 24 · 32. `Space.gutter` is 20 and the only horizontal inset. Section gap 24, section head to first row 12, card padding 16. Rows are 72pt minimum, so a title over a subtitle at the type floor fits; they grow at larger text sizes.
- Radii, all continuous:

  | Element | Radius |
  | --- | --- |
  | buttons, chips, segments, selects, count pills | capsule |
  | card | 20 |
  | compact card (cover card, feed card, mover tile) | 18 |
  | field | 15 |
  | search | 16 |
  | notice | 15 |
  | toast | 15 |
  | badge | 8 |
  | inline pill | 6 |
  | marks (cabal and stock) | `size × 0.32` |
  | avatars, round toolbar buttons | circle |
  | sheets | the system radius |

- Elevation: cards are flat, a `surface` fill with a 1px `hairline` border and no shadow. Only the tab bar and the toast carry the soft floating shadow (`MonacoTheme.Shadow.floating`: `0 8 24`, 7% green-black in light, 20% black in dark). `Shadow.sheet` (`0 22 72`, 25% in light, 50% black in dark) is the sheet's; iOS 26 sheets keep the system's own.

## Glass and transparency

Glass is chrome only (principle 6), and the app never draws its own compositor.

- Keep the system `TabView`. Never hand-draw a tab bar.
- iOS 26: glass comes from `glassEffect` and the system, in `MonacoGlassButton`, sheets and the dock.
- iOS 18: `.ultraThinMaterial` for the round buttons and the dock, `.regularMaterial` for sheets.
- With `accessibilityReduceTransparency` on, every glass surface becomes a `surface` fill with a `hairline` stroke (`canvas` for sheets).
- Never draw a status bar, a home indicator or a blur compositor. The blur radii in [tokens.md](design/v3/tokens.md) describe intent, not code to copy.

## Icons and motion

- SF Symbols only, monochrome, named through `MonacoIcon` (ticket F5) so screens never type symbol strings. The design's icon grid and sizes are in [components.md](design/v3/components.md). Tab bar: default size, `.fill` when selected. Row leading glyph: a 22pt `brand` symbol in a 44pt `brandSoft` tile with radius 14.
- State change `.easeOut(0.2)`. Segment and chip selection `.spring(response: 0.3, dampingFraction: 0.85)`. Button press scales to 0.97. Numbers roll with `.numericText`. Charts draw on once per range per appearance. Skeletons pulse 1 to 0.55 over 1.2s.
- Haptics: `.selection` for chips, segments, tabs, presets and chart scrub; light impact on press of the primary button style only (the style plays it; actions never call `Haptics.tap()` behind it); `.success` when a vote, proposal or money movement lands; `.warning` on every error toast.
- Reduce Motion turns everything off except the toast fade. The package has no motion of its own, so this section is unchanged by v3.

## Charts

Decision 4 in practice. Candles, intervals, pinch and pan, and the crosshair stay. The line chart takes the v3 look:

- a 1.7pt stroke in `profitVivid` or `lossVivid`;
- a fill gradient from 14% opacity to 0;
- an end dot of radius 3.5;
- an `ink` tooltip pill while scrubbing, with `canvas` text;
- start and end dates below the chart.

Range pills are a segmented control.

## Decisions

The founder has made these nine decisions and they are final; the full reasoning is in [decisions.md](design/v3/decisions.md). Tickets apply them and do not reopen them.

1. **Adopt v3 wholesale.** Cream canvas, bordered cards, capsule P&L pills, cover-art cabal cards, a dark identity card and glass chrome. Do not take half of it: two visual languages in one app is the failure to avoid.
2. **Keep the 15pt text floor (#4110).** Captions the design draws at 10–13px become 15pt. Screens come out a little taller than the references, and parity QA accepts that.
3. **Keep trading bots (agents)** and restyle them in the v3 language. The agent rows and the "Add a trading bot" entry stay, drawn with the same rows, tiles and cards as everything else. No reference image exists for them; judge them against the neighbouring v3 screens.
4. **Keep native chart behaviour** (candles, intervals, pinch and pan, crosshair) and restyle the line chart to the v3 look (see Charts).
5. **Tabs: Home, Cabals, Markets, Feed, Profile.** "Stocks" is renamed "Markets". The tab is labelled Markets; the accessibility identifier stays `tab-assets`.
6. **Glass: keep the system `TabView`.** iOS 26 glass comes from `glassEffect` and the system; iOS 18 uses `.ultraThinMaterial` and `.regularMaterial`; Reduce Transparency falls back to solid surfaces; never draw the prototype's status bar, home indicator or blur compositor.
7. **Backend gaps: build the UI against fixtures.** The design shows things the backend lacks: the notifications list, balances detail, group-cash yield, cabal holding cost basis, feed likes, profile bio, the pnl-private permission, the watchlist (local storage is fine), rank movement and card-funding return status. Where data is missing, show a muted `Typo.caption` line "Opens soon" under the section header, or in place of the missing figure. Never show a fake number in a release build. List the missing API in the ticket's Context so a backend ticket can be filed.
8. **Cabal covers: no stock photos.** The package's JPEGs have no licence. Draw the cover from the cabal's tint gradient plus its uploaded picture when there is one, under the scrim. Default avatars stay as they are today (pixel animals).
9. **Wordmark.** Render "monaco" in SF Pro Bold until the designer sends a vector. The vector is a follow-up.

## References

- Renders: `docs/design/v3/screens/<group>__<template>__<state>__<light|dark>.jpg`, drawn at 402 × 874. Every template has its default state in Light and Dark; other states are Light only. The [README](design/v3/README.md) indexes them all.
- Prototype: open `docs/design/v3/monaco-ui-v3.html#screen=<template>&state=<state>` from a checkout in a browser. The hash is read once on load. Light / Both / Dark, Larger text and Solid surfaces are toggles there.
- **Parity** means the screen matches the reference in Light and Dark, allowing the height difference the 15pt floor causes, with the app's real data, real avatars and tint covers instead of the fixtures.

## Composition rules

- Labels are sentence case. No ALL CAPS and no "NAV".
- A muted caption above an amount field is fine ("Deposit amount"). P&L is labelled "Your P&L · All time" where the design says so.
- Sample harnesses cover every state; `scripts/qa/screens.sh` is the acceptance gallery.
