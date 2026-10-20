# v3 decisions and risks

Eleven founder decisions, all adopted and final, then the five risks the design review found. Tickets apply these and do not reopen them. Tokens are in [tokens.md](tokens.md), components in [components.md](components.md), screens in the [README](README.md) index.

## Decisions

### 1. Adopt v3 wholesale

The app takes the whole v3 look: cream canvas, bordered cards, capsule P&L pills, cover-style cabal cards, the dark identity card and glass chrome. Today's `docs/design.md` deliberately removed the cream canvas, hero bands, cards, capsule badges and glass; v3 brings each of them back, so a half-adoption would leave the app with two visual languages and every screen ticket arguing about which one applies. `docs/design.md` and the `ios-screen` skill are rewritten to match in ticket T0, so engineers and agents read one source of truth before any code changes. The advisor's fallback (take the palette and cards but render P&L as plain coloured text) is not taken.

### 2. Keep the 15pt text floor

Nothing renders below 15pt, even where the design draws captions, pills, tab labels and chart dates at 10–13px. The floor came from hands-on QA (#4110) and small money text is the failure it fixed. Each design role maps to the nearest `Typo` or `moneyFont` role at or above 15 (see the table in [tokens.md](tokens.md)). Screens therefore come out about 10% taller than the reference images, and parity QA accepts that height difference. Engineers must not add `.system(size:)` to match a pixel size; the `fixed-font` rule fails the build outside `Design/`.

### 3. Keep trading bots, restyled

The design removed bots from its prototype (no bot entry in Propose, no bot section on the cabal), but the app has them: `AgentRoute`, `ProposeAddAgentView`, `CabalAgentSlot`, `AgentKeyRevealView`. Removing a shipped feature because a prototype left it out would be a product decision nobody made, so the bot screens, the "Add a trading bot" entry in the Propose flow and the cabal's bot slot stay and take the v3 components (cards, rows, buttons, notices). No reference image exists for them; they are built from the existing components and judged against the neighbouring v3 screens.

### 4. Keep native chart behaviour, restyle the line

The design draws a plain line with a hover pill and no intervals. The app has a candle chart with intervals, pinch and pan on the stock screen and a crosshair with pinch on Home, which is richer than the prototype can show. Those stay. The line chart takes the v3 look: 1.7pt stroke, fill fading from .14 to 0, end dot radius 3.5, an ink tooltip pill and a dates row. Candles remain the stock default per `docs/screens.md`, behind the design's line look where a line is shown.

### 5. Tabs are Home, Cabals, Markets, Feed, Profile

"Stocks" is renamed "Markets" and the order follows the design. Changing only one of name and order would leave the app disagreeing with every reference image, and deciding it once in T0 stops each ticket from picking its own. Journeys, `docs/screens.md` and `docs/journeys/*` are updated for the new order and name in the Q5 ticket. The icon for Markets is `chart.line.uptrend.xyaxis`.

### 6. Glass: system `TabView`, material fallback, no drawn compositor

The deployment target is iOS 18, where the system tab bar and sheets get Liquid Glass for free on iOS 26. A hand-drawn floating tab bar would fight the system bar and the `scrollEdgeEffectStyle` work. So the app keeps the system `TabView`; glass buttons use `glassEffect` on iOS 26 and `.ultraThinMaterial` / `.regularMaterial` on iOS 18; sheets use the system background on 26 and `.regularMaterial` on 18. Reduce transparency falls back to solid `surface` / `canvas`. The prototype's status bar, home indicator and blur compositor are never drawn. The browser-only values in [tokens.md](tokens.md) (blur radii, saturate) describe intent, not code to copy.

### 7. Build against fixtures where the backend has no data

The design shows things the backend cannot serve yet: the notifications list, balances detail, group-cash yield, cabal holding cost basis (average buy and sell, realized and unrealized), feed likes, profile bio, the pnl-private permission, rank movement, card-funding return status, and a watchlist (a local store is fine). Each screen is built against fixtures with an "opens soon" caption where data is missing, so the UI ships and is reviewed now. The ticket's Context section lists the missing API so a backend ticket can be filed. Figures such as a yield are never asserted without an API value. The `network` chooser is built (decision 10).

### 8. Cabal covers: solid tint and uploaded picture, no stock photos

The three cover JPEGs in the design package have no recorded licence or provenance, so they are not shipped, and the 3D-character avatars are fixtures too. A cabal cover is drawn from the cabal's own solid tint plus its uploaded picture when there is one, under one flat `coverOverlay`, so white text stays readable on any picture (decision 12). Default avatars stay as today (photo, initials, pixel animals). A curated default cover set can be added later without changing the card.

### 9. Wordmark in SF Pro Bold until a vector arrives

The design's wordmark is a 154×37 raster PNG, too small to ship crisply at 3x. Until the designer sends a vector or a @3x export, "monaco" is set in SF Pro Bold as `Auth/MonacoMark.swift` does today. The font itself is not a licensing issue: the design is SF Pro (the system font), and "Inter" in the CSS stack is a web fallback only. The vector wordmark is a follow-up, as an asset-catalog image set with light and dark appearances.

### 10. Keep the network chooser: the app is multichain

The `network` template ("Choose a network") ships as the deposit and withdraw network chooser. Deposits and withdrawals take USDC on Solana, Base, Arbitrum and Ethereum (#5450). Stocks trade on Solana, Base and Robinhood Chain only (#5492). Monad appears as a display-only network in deposit, withdraw and best price (#5487). The rows use chain logos where the reference draws letter tiles.

### 11. "Cabal", never "group"

The reference still says "group" in places (Group trades, from the group pot, Join a group, Group · 1M). The app says "cabal" everywhere, and parity review counts that wording as a match, not a difference.

### 12. No gradients anywhere in the app (founder, 2026-10-20)

Covers use one flat `coverOverlay` colour (`coverOverlayReduced` under Reduce Transparency) over a solid cabal tint and the uploaded picture. Charts draw the line only, with no fill under the price, performance or sparkline curves. The `CoverScrim` gradients and every gradient helper are deleted, and a RepoRulesTests rule fails on `LinearGradient`, `RadialGradient`, `AngularGradient`, `EllipticalGradient`, `.linearGradient(` and `Gradient(` in the app and in `packages/mobile-core`. This supersedes the tint gradient and scrims in decision 8 and the gradient fill in the chart specs.

## Risks

### R1. Contrast

Light `muted` (4.51:1 on `canvas`, 4.16:1 on `surfaceSunken`), `loss` (4.30:1) and `gain` (4.58:1) are at or below AA for normal text, so `MonacoContrastTests` fails until F1 darkens them one step. The design's 10–11px muted captions would fail outright; at the 15pt floor they need at least 4.5:1. F1 records the final hex values in `docs/design.md`.

### R2. Prototype defects not to copy

- The proposal screen's vote dock overlaps the "Discussion" header in `proposals__proposal__open__light.jpg` (dock padding against scroll padding).
- `--price-surface` is unused after v1.5.2 (`--subtle` is undefined), so price blocks are border-only.
- `design-tokens.json` and `component-contract-v1.5.json` carry stale card sizes (176 / 104 and 214×138); the 300×218 member card wins.
- The chart tooltip hardcodes "Oct n" dates.
- The two leaderboard `board()` definitions disagree (segmented against selects); the selects win.
- `settings` is the v1.4 layout and keeps prototype-only toggles ("Larger text", "Reduce transparency") that must not ship as app settings.

### R3. What native cannot or should not do

`backdrop-filter: saturate()` is approximated with a material. `color-mix` washes are precomputed hex in [tokens.md](tokens.md). Variable font weights 550–690 map to `.medium`, `.semibold`, `.bold`. Cover `text-shadow` is replaced by the flat cover overlay. The sheet "expand arrow" becomes detents and drag. Browser `<select>` becomes a menu `Picker`. The Apply-draft filter sheet becomes a `Menu` of toggles that apply instantly, as the component contract already says.

### R4. Parallelism

Two engineers work in parallel only if `Design/*` and `Features/Shell/*` are frozen after the foundation tickets, and lanes never edit each other's files (Lane A and Lane B own disjoint feature folders). `ProposalCard` (Lane A) lands before the cabal screen ticket (Lane B) uses it. Any shared-file change goes back to the owning foundation ticket as a small follow-up.

### R5. Scope

43 templates and 255 states is about 14 screen tickets after about 8 foundation tickets, several weeks for two engineers. Sequence by value: F1–F4, then Home (S2), then Cabals (S8, S9), then Propose and Proposal (S6, S7), then the rest.
