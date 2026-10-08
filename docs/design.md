# Monaco design language (iOS)

The look in one sentence: **Robinhood's screen, Coinbase's manners, Fomo's people.** A neutral canvas, one typeface with tabular figures, one accent, numbers that are the biggest thing on every screen, coloured text for gains and losses, full-bleed charts, big bars for actions, and faces everywhere a person acted.

Kept: the forest `#0F291C` as the single accent (it is the logo), the five cabal tints, every colour and font a token, the slot architecture, toasts for confirmations, the product words. Gone: the cream canvas, the dark green hero bands, Avenir Next, SF Mono as a market voice, the gold coin material, ruled-ledger section chrome, capsule P&L badges, pixel animals as the default face, the four-disc action row.

## Principles

Each one is a check you run on a screenshot.

1. **The number is the hero.** On every money screen the largest thing is a figure, set bold and tabular; nothing above it is larger than 15pt.
2. **Colour means one thing.** Green is a gain, red is a loss, ink is "you can tap this", a cabal tint is identity. No other hue appears, and no gain or loss sits in a capsule.
3. **One typeface, one canvas.** Every glyph is SF Pro; every screen is white (light) or near-black (dark) edge to edge, with no coloured bands.
4. **One primary action per screen, pinned.** A 56pt bar at the bottom names the outcome; everything else is a row, a text link or a toolbar glyph.
5. **Rows, not boxes.** Content is 64pt rows separated by a hairline; the only cards are things you can act on (a vote, a deposit address), and they are sunken, not outlined.
6. **Charts run to the edge.** Every price or value curve bleeds past the gutter, with the figure above it changing under your finger.
7. **A face for every act.** Anything a person did (proposed, voted, bought, joined) shows their avatar; anything a cabal did shows its mark.

## Colour

Tokens live in `apps/mobile/Monaco/Design/MonacoTheme.swift`; feature code uses the role names. `MonacoContrastTests` holds every text pair to AA on all three surfaces.

| Role | Token | Light | Dark | Used for |
| --- | --- | --- | --- | --- |
| Canvas | `background` / `canvas` | `#FFFFFF` | `#0B0F0D` | every screen, tab bar, nav bar when scrolled |
| Surface | `surface` | `#FFFFFF` | `#131816` | sheets, menus; the same as canvas in light on purpose |
| Sunken | `surfaceSunken` | `#F2F4F2` | `#1B211E` | fields, chips at rest, cards, secondary button, skeletons, pressed rows |
| Text | `primaryText` / `ink` | `#0E1512` | `#F2F4F2` | |
| Secondary text | `secondaryText` / `muted` | `#5E6963` | `#9AA59E` | subtitles, labels under figures |
| Tertiary text | `tertiaryText` | `#68716C` | `#86908A` | timestamps, ranks, attribution |
| Disabled label | `disabledLabel` | `#949C97` | `#626B66` | placeholders, disabled CTAs; below AA on purpose |
| Separator | `border` / `hairline` | `#E8EBE8` | `#242B27` | 1px, never 1pt |
| Accent fill | `brandFill` | `#0F291C` | `#EDF2EF` | primary button, selected chip and segment, my chat bubble |
| On accent | `onBrand` | `#FFFFFF` | `#0F291C` | |
| Accent text | `brand` | `#1F5A3D` | `#9ED5B3` | text links, focus ring |
| Accent wash | `brandWash` | `#0F291C` 8% | `#9ED5B3` 14% | the viewer's own row on a board |
| Gain | `profit` | `#0A7F46` | `#2EDB8A` | text |
| Gain vivid | `profitVivid` | `#12B76A` | `#2EDB8A` | chart stroke and live dot only |
| Loss | `loss` / `destructive` | `#D0342C` | `#FF6B5E` | text |
| Loss vivid | `lossVivid` | `#E5484D` | `#FF6B5E` | chart stroke only |
| Warning | `warning` | `#8A5A16` | `#E5B26A` | pending, paused, after hours |
| Crown | `goldGlyph` | `#C9A24A` | `#D9B85A` | the rank-1 crown glyph only |
| Toast | `toastFill` / `toastLabel` | `#33413A` / white | `#F2F4F2` / `#0E1512` | the toast inverts the scheme |
| Cabal tints | `CabalTint` ×5 | pine, ochre, plum, indigo, moss | | marks, chart lines, allocation bar |

Nothing draws a P&L wash or a dark band. The retired hero, wash and on-ink tokens alias the canvas and the bare money colours until their last call sites move.

## Type

One family, SF Pro, at the size floor #4110 set after hands-on QA found text too small. A role at a system text style's size uses that style; display, headline and body pre-scale with `Typo.scaled`, so Dynamic Type scales every role. SF Pro Display engages at 20pt and up. Every figure is `.monospacedDigit()`. No letter-spacing overrides.

| Role | Token | Size / line | Weight | Used for |
| --- | --- | --- | --- | --- |
| Hero number | `moneyFont(.hero)` | 48 / 56 | Bold | portfolio total, pot value, stock price, amount entry |
| Large title | `Typo.display` | 36 / 43 | Bold | tab root titles, sign-in wordmark, profile name |
| Large number | `moneyFont(.large)` | 32 / 38 | Bold | proposal amount, your slice, review figure |
| Title | `Typo.title` | 28 / 34 | Semibold | headings in content, stock name, keypad digits |
| Section | `Typo.section` | 22 / 28 | Semibold | section titles |
| Headline | `Typo.headline` | 18 / 23 | Semibold | row titles, button labels, card headlines |
| Body | `Typo.body` | 18 / 23 | Regular | prose, chat, reasons |
| Row number | `moneyFont(.row)` | 18 / 23 | Semibold | prices and values in rows |
| Subhead | `Typo.subhead` | 17 / 22 | Regular | row subtitles, helper lines, labels under figures |
| Subhead strong | `Typo.subheadStrong` | 17 / 22 | Semibold | text links, chip and segment labels, toast text |
| Small number | `moneyFont(.caption)` | 15 / 20 | Semibold | day change, return under a value |
| Caption | `Typo.caption` | 15 / 20 | Medium | timestamps, "Closes in 2d", attribution |

Tickers set in `Typo.subhead` secondary. Wallet addresses are the one monospaced text, inside `MonacoWalletAddressText` only.

## Spacing, radii, elevation

- Spacing: 4 · 8 · 12 · 16 · 24 · 32 · 48. `Space.gutter` is 16 and the only horizontal inset. 32 between sections, 12 between a section title and its first row. Rows are 64pt minimum with 12 vertical padding. Card padding 16. Bottom bar: 12 top, 8 bottom plus the safe area.
- Radii: buttons, chips, segments are capsules · card 16 · field 12 · cabal mark 12 at 40pt (size × 0.3) · stock mark and avatar are circles · toast 14 · chat bubble 18 · sheets use the system radius. All continuous.
- Elevation: none. No shadows, gradients or strokes around containers. Depth is sunken fill against the canvas, and the hairline. The bottom bar is canvas with a hairline on top.

## Icons and motion

- SF Symbols only, monochrome. Tab bar: default size, `.fill` when selected. Toolbar: 17pt semibold (`plus`, `ellipsis`, `bubble.left`, `magnifyingglass`, `gearshape`). Row leading glyph: 17pt medium in `secondaryText` on a 40pt sunken circle.
- State change `.easeOut(0.2)`. Segment and chip selection `.spring(response: 0.3, dampingFraction: 0.85)`. Button press scales to 0.97. Numbers roll with `.numericText`. Charts draw on once per range per appearance. Skeletons pulse 1 to 0.55 over 1.2s.
- Haptics: `.selection` for chips, segments, tabs, presets and chart scrub; light impact on button press; `.success` when a vote, proposal or money movement lands; `.warning` on every error toast.
- Reduce Motion turns everything off except the toast fade.

## Composition rules

- Labels are sentence case. No eyebrow labels, no ALL CAPS, no "P&L", no "NAV".
- Sample harnesses cover every state; `scripts/qa/screens.sh` is the acceptance gallery.
