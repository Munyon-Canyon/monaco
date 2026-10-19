# v3 design tokens

Exact values from the 1.5.3 CSS (`base.css`, then `revision-v15.css`, `presentation-v15.css`, `revision-cabals-v151.css`, `revision-density-v152.css`, `revision-deposit-v153.css`; a later file wins) and the SwiftUI name each one gets in `Design/MonacoTheme.swift`. Feature code uses the role name and never a hex value (the `raw-color` rule). `px` in the design is `pt` in the app. Names marked **new** do not exist today; the rest are retunes of existing tokens. Ticket F1 owns the code.

## Colours

Light and Dark are the two appearances. `rgba()` and 8-digit hex keep their alpha.

### Core

| CSS var | Light | Dark | Used for | SwiftUI |
| --- | --- | --- | --- | --- |
| `--bg` | `#FAF7F0` | `#03130F` | canvas, composer | `canvas` |
| `--text` | `#0C3526` | `#F5F8F4` | headings, hero figures, row titles | `ink` |
| `--body` | `#172C24` | `#E6EFEB` | prose, chat, thesis | `bodyText` **new** |
| `--muted` | `#6B746E` | `#9EAFA5` | captions, eyebrows, resting tab labels, chevrons | `muted` |
| `--line` | `#E5E7DF` | `#18342A` | 1pt rules, card borders, switch off, vote-bar track | `hairline` |
| `--surface` | `#F6F7F2` | `#0A2119` | cards, fields, keypad keys, mover tiles | `surface` |
| `--surface2` | `#EDEFE7` | `#102A20` | search box, segmented track, skeletons, other's chat bubble | `surfaceSunken` |
| `--accent` | `#07643E` | `#60EFAA` | links, selected chip text, tab "on", notice text, focus | `accent` |
| `--soft` | `#E8F0E7` | `#14372A` | secondary button, selected chip, tab bubble, count pill, my chat bubble, action tiles, unread row | `brandSoft` **new** (opaque) |
| `--gain` | `#108153` | `#68EBAB` | positive text, chart stroke and fill, "yes" votes | `profit` (and `profitVivid`, same value) |
| `--loss` | `#BD5455` | `#F1807F` | negative text, chart, "no" votes, danger, invalid border | `loss` (and `lossVivid`) |
| `--warning` | `#866017` | `#E6C477` | pending, stale, warn badge text | `warning` |
| `--warnbg` | `#F3ECD8` | `#2C2819` | warn badge and notice fill | `warningWash` **new** |
| `--button` | `#0D5C3A` | `#66EFAB` | primary button, selected segment, switch on, send disc, compact Deposit | `brandFill` |
| `--buttontext` | `#FFFFFF` | `#072219` | text on `--button` | `onBrand` |
| `--scrim` | `#01150E55` | `#000000BB` | batch-sheet overlay | `scrim` **new** |

Contrast: light `muted` is 4.51:1 on `canvas` and 4.16:1 on `surfaceSunken`; light `loss` is 4.30:1; light `gain` is 4.58:1. `MonacoContrastTests` pins AA, so F1 darkens those three light values one step and records the final hex in `docs/design.md` (risk R1). The values above are the design's, not the shipped ones. Dark pairs are all at least 7:1.

### Glass and sheets

| CSS var | Light | Dark | SwiftUI |
| --- | --- | --- | --- |
| `--glass` | `rgba(251,252,247,.86)` | `rgba(12,35,27,.85)` | `glassFill` **new** |
| `--glassborder` | `rgba(255,255,255,.98)` | `rgba(176,234,200,.2)` | `glassStroke` **new** |
| `--modal` (sheet) | `rgba(248,250,245,.90)` | `rgba(17,41,31,.94)` | `sheetGlass` **new** |
| `--modal-border` | `rgba(255,255,255,.95)` | `rgba(168,227,194,.21)` | `sheetStroke` **new** |
| `--modal-dim` | `rgba(0,14,8,.3)` | `rgba(0,6,4,.48)` | `sheetDim` **new** |

In the app these are fallbacks and design intent: iOS 26 uses system Liquid Glass, iOS 18 uses `.ultraThinMaterial` / `.regularMaterial` (see Glass below).

### Washes and status

| CSS var / rule | Light | Dark | Used for | SwiftUI |
| --- | --- | --- | --- | --- |
| `--buy-surface` | `#E9F4EC` | `#0E3022` | feed buy block, trade-history buy tile | `buyWash` **new** |
| `--sell-surface` | `#F8ECE9` | `#30201F` | feed sell block, trade-history sell tile | `sellWash` **new** |
| `--price-surface` | `#EDF0F9` | `#13232B` | price-move block (unused since 1.5.2: border only) | `priceWash` **new** |
| `.vote-needed152` | `#083123` on `#C8F7CF` | same | "1 vote needed" badge on cover cards | `voteNeededLabel`, `voteNeededFill` **new** |
| `.event-pill152.discussion` | `#535AAE` on `#676BDD13` | `#C7B9F7` on `#BCA4FF1A` | feed "Discussion" pill | `discussionTint`, `discussionWash` **new** |
| allocation segments 2, 3 | `#7B91A7`, `#9A83B4` | same | allocation bar (segment 1 `accent`, segment 4 `hairline`) | `allocationTints` **new** |
| `.chart-tooltip`, `.toast` | `--text` fill, `--bg` text | same | tooltip pill, toast | `toastFill`, `toastLabel` (retune) |

Ticker tiles: the prototype tints only NVDA (`#C7F69F` on `#1A3614`), MSFT (`#F0DFBD` on `#5E4214`) and AAPL (`#DEDEE7` on `#1C2230`). Do not copy this. The app keeps the issuer logo, and the fallback tile is `surfaceSunken` with `ink` text.

### Identity card and pencil

| Rule | Light | Dark | SwiftUI |
| --- | --- | --- | --- |
| `--identity` | `#0C3828` | `#0D2C21` | `identityFill` **new** |
| `--on-identity` | `#F8FAF5` | `#F5FAF6` | `onIdentity` **new** |
| handle, stat labels, joined (`#B6D5C3`) | `#B6D5C3` | same | `onIdentityMuted` **new** |
| bio (`#E0EEE3`) | `#E0EEE3` | same | `onIdentityBody` **new** |
| P&L on the card | `#81E8B1` / `#FFAAAA` | same | `profitOnIdentity`, `lossOnIdentity` **new** |
| pencil badge | glyph `#0C3828` on `#E6F5E8`, 2pt ring in `identityFill` | same | `pencilFill` **new** |
| card edge | 1pt `#FFFFFF19` | same | inside `identityFill` card |

### Cover cards (text and scrims sit on a picture, so they do not change with appearance)

| Rule | Value | SwiftUI |
| --- | --- | --- |
| cover text | `#FFFFFF` | `onCover` **new** |
| secondary text on cover (members line, labels, caption, "—") | `#C4D7CB`, `#C8DACF`, `#D0D9D5`, `#D0DDD5`, `#E2ECE5`, one family | `onCoverMuted` **new** (F1 picks one value from the family) |
| P&L on cover | gain `#83F1B9`, loss `#FFB2B0` | `profitOnCover`, `lossOnCover` **new** |
| share disc on cover | fill `#FFFFFF18`, border `#FFFFFF24` | `onCoverGlass` **new** |
| card inner stroke (v1.5.1) | inset `#FFFFFF12` | `coverStroke` **new** |
| Member card scrim 300×218 (winning) | `linear-gradient(180deg, #00150C26, #00130BD9 42%, #000C09F5 100%)` | `CoverScrim.memberCard` |
| Cover card scrim 214×138 | `linear-gradient(180deg, #00170C1A 10%, #00110912 30%, #000C08ED 100%)` | `CoverScrim.card` |
| Cover hero scrim | `linear-gradient(transparent, #011A17D9)` | `CoverScrim.hero` |
| Reduce transparency | flat `#00130BE8` | `CoverScrim.solid` |

The design's cover photos are not shipped (decision 8). The app draws the cabal's tint gradient plus its uploaded picture under these scrims.

### Chart and state

| What | Value | SwiftUI |
| --- | --- | --- |
| line stroke | 1.7pt, `profit` or `loss` | `ChartStyle` (names proposed, F1 and F6 finalise) |
| area fill | line colour at opacity .14 fading to 0 | `ChartStyle.fillOpacity = 0.14` |
| end dot | radius 3.5 | `ChartStyle` |
| tooltip | `ink` pill, `canvas` text, radius 9, padding `7 9` | `ChartStyle` |
| disabled | opacity .45 (buttons), .35 (action tiles) | `MonacoButtonMetrics.disabledFillOpacity = 0.45` |
| focus ring | 3pt `accent`, offset 2 | web only, n/a |

## Typography

Family: SF Pro (system). The CSS stack is `-apple-system, BlinkMacSystemFont, Inter, Arial, sans-serif`, so every render is SF Pro; nothing is bundled and "Inter" is a web fallback only. All figures are tabular (`.monospacedDigit()` on figures only). The design's letter-spacing is negative on large type (−1.85px at 43px); do not add tracking, SF Pro's optical tracking covers it. The "Larger text" variant multiplies sizes by `--t = 1.22`.

**15pt floor** (decision 2): nothing renders below 15pt. Every design size under 15 maps to `Typo.caption` / `Typo.captionStrong` (15). Sizes from 15 up map to the nearest role. Weights: 500–570 → `.medium`, 600–640 → `.semibold`, 650–720 → `.bold`.

| Design role | px / line / weight | SwiftUI role |
| --- | --- | --- |
| hero figure `.hero-number` | 43 / 1.08 / 665 (large text 47) | `moneyFont(.hero)` |
| medium hero `.hero-number.medium`, review hero | 35 / — / 665 | `moneyFont(.large)` |
| amount input, receipt hero | 48 / 1.15 / 660; 40 | `moneyFont(.hero)` |
| auth intro h1 | 35 / 1.12 | `Typo.display` |
| root title h1 | 30 / 1.12 / 690 | `Typo.display` (bold) |
| cover hero h1; outcome and profile h1 | 29 (short 25); 28 / 1.2 | `Typo.title` |
| identity name, proposal hero h1 | 25 / 650 | `Typo.title` |
| trade asset name | 23 / 650 | `Typo.section` |
| card share figure | 24 / 650 | `moneyFont(.card)` (new, 24 bold) |
| identity stats, two-stats | 22 / 620; 21 | `moneyFont(.card)` |
| sheet title, empty-state h2, section h2 | 22 / 650; 19 / 660 | `Typo.section` (keep 22) |
| keypad digit, OTP digit | 23 / 500; 23 / 600 | `Typo.title` |
| ownership figure | 20 / 590 | `moneyFont(.row)` |
| metric, stat-grid and P&L figures | 17–18 / 650 | `moneyFont(.row)` |
| card name, choice row | 17 / 650; 17 / 600 | `Typo.headline` |
| toolbar title, button, row title | 15–16 / 620–660 | `Typo.headline` |
| input text, body prose | 15–16 | `Typo.body` |
| row trailing, key row, chat bubble, comment | 13–14 | `Typo.subhead` / `Typo.subheadStrong` |
| helper, eyebrow, chip, segment, link, vote labels | 12 / 550–600 | `Typo.caption` / `Typo.captionStrong` |
| badge, count pill, pnl-inline, event pill, metric label | 10–11 / 560–650 | `Typo.captionStrong` |
| tab label, dates row, decision footer, toast | 10–13 | `Typo.caption`; tab labels via `UITabBarAppearance` |

Line heights: prose 1.5, helper 1.55, thesis 1.7, company description 1.75, headings 1.08–1.25. Use SwiftUI defaults except thesis and description (`.lineSpacing(4)`).

## Spacing

Scale: **4 · 8 · 12 · 16 · 20 · 24 · 32**. `Space` already has 4/8/12/16/24/32; v3 adds `Space.gutter = 20` (today 16) and `Space.section = 24` (design 26).  Keep today's `Space` names; only gutter and section are added or changed.

| What | Design value | SwiftUI |
| --- | --- | --- |
| screen side padding | 20 | `Space.gutter` |
| gap between sections | 26 (20 for proposals and chat) | `Space.section` |
| section head → first row | 11, min height 34 | 12 |
| root title margin | `14 0 17` | top 16, bottom 16 |
| card padding | 16; inset card `0 15` | `Space.m` |
| card stack gap | 12 | 12 |
| decision card | `13 13 0`, stack gap 10 | 12 / 12 / 0, gap 12 |
| compact feed card | `10 12 0`, gap 10 | 12 / 12 / 0 |
| list row | `14 0`, gap 12, min height 73 | `MonacoRowLayout.minHeight` 64 → 72 |
| key row | `13 0`, gap 16 | row 48 |
| chart | height 170 (small 148), margin `14 -2 0` | `ChartStyle.height` 170 / 148 |
| periods control | margin `0 0 16`, padding 3, min height 44 | |
| cabal rail | gap 10, bleed to screen edge, leading inset 20 | `.contentMargins(.horizontal, 20)` |
| cabal card | rail 300×218, list full width × 208, padding `14 15` | `CabalCoverCard` |
| mover tile | 244 wide, padding 14, gap 12 | `MoverTile` |
| two stats | `17 0`, margin `18 0`, gap 20 | |
| stat grid | 2 columns, column gap 24, cell `15 0`, min 73 | `StatGrid` |
| action tiles | 3 columns, gap 4, tile 66×72, disc 44, label gap 7 | |
| allocation bar | height 7, gap 2, margin `15 0 6` | |
| notice | `13 14`, margin `14 0`, icon 18 | `MonacoNotice` |
| empty state | `36 14 30`; icon tile 68; text max 280 | `EmptyState` |
| search box | min 47, `0 13`, margin-bottom 17 | |
| field | margin `17 0`, input `13 14` min 48 | `MonacoFieldChrome.height` 56 → 48 |
| amount box | `22 0 16`, figure margin `20 0 8` | |
| keypad | 3 columns, gap 7, margin `20 0`, key 49 | `AmountKeypadGrid.rowHeight` 56 → 49 |
| OTP | 6 boxes, gap 7, height 57 | |
| dock (pinned actions) | padding `11 20`, gap 10 | `BottomCTA` |
| composer | `10 15`, gap 8, input 44, send disc 44 | |
| toast | inset 20, `13 17` | `MonacoToastBanner` |
| sheet | padding `9 18 15`, grab handle 36×5, title min 48 | detents |
| identity card | padding 18, row gap 13, stats gap 12 | |
| vote bar | height 8, gap 3, margin `14 0 10` | `VoteBar` |
| rank row | min 66–78, `9–15 0`, gap 10, rank column 22 | `BoardRow` |
| tab bar | height 70, inset 8, bottom 22, items 56 | system `TabView` |

Touch targets stay 44pt: compact Deposit is 32pt visual in a 44pt hit box, chips and pencil extend their hit area.

## Radii

| Element | Design radius | SwiftUI |
| --- | --- | --- |
| buttons, segmented, periods, chips, selects | capsule (26 at 48 tall) | `Capsule()` |
| round toolbar button, avatar, count pill | 50% | `Circle()` |
| card, proposal card, rich event | 21 | `Radius.card` 16 → **20** |
| decision card, member card | 20 | `Radius.card` |
| cover card, feed card, mover tile, post summary, chat preview | 18 | `Radius.cardTight` 18 **new** |
| cover hero | 22 | 22 |
| identity card | 22 (24 in v1.5) | 22 |
| field, input | 15; search 16; composer input 20; OTP 13; keypad key 14; QR 18 | `Radius.field` 12 → **15**; `Radius.search` 16 |
| asset block, invite link | 12 | 12 |
| notice | 15 | `Radius.notice` **new** |
| badge | 8 | `Radius.badge` **new** |
| pnl-inline, event pill | 6 | `Radius.pill` **new** |
| vote-needed badge | 10 | 10 |
| circle token, ticker tile | `size × 0.32` (44 → 14) | `MarkGeometry.radius(size)` |
| empty icon tile, status orb | `size × 0.35` (68 → 23, 70 → 25) | |
| toast | 15 | `Radius.toast` 14 → 15 |
| sheet | 31; expanded `29 29 0 0` | system on iOS 26; `Radius.sheet` on 18 |
| chat bubble | `4 17 17 17` theirs, `17 4 17 17` mine | `Radius.bubble` 17 with a 4 tail corner |
| allocation bar 8, vote bar 10, tooltip 9, skeleton 6 | | |

## Shadows, glass and blur

Cards, decision cards, event cards and mover tiles have **no shadow**; only a 1pt `hairline` border.

| Surface | Fill | Stroke | Blur | Shadow | Reduce transparency |
| --- | --- | --- | --- | --- | --- |
| Tab bar | `glassFill` | 1pt `glassStroke` | 22, saturate 1.4 | `0 8 24` `#112E2112` light / `#00000033` dark (`Shadow.floating`) | `surface`, no blur |
| Round toolbar button | `glassFill` | 1pt `hairline` | none | none | `surface` |
| Dock | `glassFill` | top 1pt `hairline` | 20 | none | `surface` |
| Sheet | `sheetGlass` | 1pt `sheetStroke` + inset top `#FFFFFF25` | 32, saturate 1.35 | `0 22 72` `#001A1540` light / `#00000080` dark (`Shadow.sheet`) | `canvas`, overlay `rgba(0,0,0,.55)` |
| Expanded sheet | `canvas` opaque | none | | | |
| Toast | `toastFill` | | | `Shadow.floating` | |
| Cover text | | | | `0 1 8 #00000066` | use the scrim instead |
| Switch knob | white | | | `0 1 3 #00000020` | |

Native mapping (decision 6): keep the system `TabView`. Round buttons are `Circle().fill(.ultraThinMaterial)` plus a hairline on iOS 18 and `glassEffect(.regular, in: .circle)` on iOS 26. Sheets use `.presentationBackground(.regularMaterial)` on 18 and the system background on 26. `accessibilityReduceTransparency` swaps every glass fill for `surface` / `canvas`. Do not draw the status bar, home indicator or blur compositor.

## Motion

The prototype has almost none: sheet height and radius ease `.2s`, toast hides after 2.6 s, everything is killed under reduced motion. The app keeps its own vocabulary from `docs/design.md`:

| Use | Native |
| --- | --- |
| state change | `.easeOut(duration: 0.2)` |
| chip and segment selection | `.spring(response: 0.3, dampingFraction: 0.85)` |
| press | scale 0.97 |
| figures | `.numericText` rolling |
| skeleton | 1.2 s pulse, off under Reduce Motion |
| toast | `MonacoToastTiming` (2.5–10 s), not 2.6 s |
| haptics | as today |
