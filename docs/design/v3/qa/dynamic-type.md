# Dynamic Type audit, UI v3

Ticket #5574. All 43 default states, at five text sizes, in Light, on one Debug build from `staging` (`1cf21654`) plus the `MonacoSegmented` fix below. Captured headless on a SimSlim iPhone 17 Pro (iOS 27.0) with `xcrun simctl ui <udid> content_size <size>`, one launch per screen and size from the default-state lines in `scripts/qa/sample-screens.txt`. Reviewed against the rules in the issue and against `docs/design/v3/monaco-ui-v3.html` with **Larger text** on.

Sizes: `large` is the default, `xxLarge` is the prototype's Larger text, `xxxLarge` is the largest non-accessibility size, `AX1` is `accessibility-medium`, `AX5` is `accessibility-extra-extra-extra-large`.

`ok` means the screen's first viewport had no clipped or truncated money, name, ticker, button or segment label and the layout rules held. Screens longer than one viewport were judged on the first viewport; Home, Cabal, Profile and the Cabals tab were also scrolled at xxLarge and AX5. A single-line `…` on a cabal or person name in a row is allowed by the rules and is counted `ok`. A `defect` names the rule it breaks and links a follow-up from [Follow-ups](#follow-ups).

The unscrolled shots are in `.logs/qa/dynamic-type/run1/<size>/<name>.png` on the machine that ran the pass; they are not committed.

## Fixed in this PR

- `MonacoSegmented` (`Design/MonacoSegmented.swift`): from xxLarge up, when the labels do not fit one row, a control of up to three options stacks its segments and a control of more options scrolls horizontally, so a segment label is never cut. This fixes `Text messa…` in sign-in (reported on #5574), `Pend-/ing` in Account activity, `Mem-/bers` in Members, `Dis-/cover` and `Fol-/lowing` in Find friends, and the range chips on the stock screen. Default-size layout is unchanged: below xxLarge the control draws as before. `DesignLayoutTests` pins both cases.

## Results by screen

### signin / phone

| Size | Verdict | Finding |
| --- | --- | --- |
| Large | ok | No clipping, truncation or overlap. |
| xxLarge | ok | No clipping, truncation or overlap. |
| xxxLarge | ok | No clipping, truncation or overlap. |
| AX1 | ok | No clipping, truncation or overlap. |
| AX5 | defect | `MonacoSegmented` cut `Text message` to `Text messa…` (segmented). Fixed here: the control now stacks when labels do not fit. |

### verify / ready

| Size | Verdict | Finding |
| --- | --- | --- |
| Large | ok | No clipping, truncation or overlap. |
| xxLarge | ok | No clipping, truncation or overlap. |
| xxxLarge | ok | No clipping, truncation or overlap. |
| AX1 | ok | No clipping, truncation or overlap. |
| AX5 | ok | No clipping, truncation or overlap. |

### balances / ready

| Size | Verdict | Finding |
| --- | --- | --- |
| Large | ok | No clipping, truncation or overlap. |
| xxLarge | ok | No clipping, truncation or overlap. |
| xxxLarge | ok | No clipping, truncation or overlap. |
| AX1 | ok | No clipping, truncation or overlap. |
| AX5 | defect | Action tiles stay three across and `Withdraw` breaks as `With-/draw` (action tiles stack at AX). [FU2](#fu2) |

### home / loss

| Size | Verdict | Finding |
| --- | --- | --- |
| Large | ok | No clipping, truncation or overlap. |
| xxLarge | ok | Money strip stays on one line; nothing clips but Net worth and Cash sit tight. The design wraps it at this size (money strip). [FU14](#fu14) |
| xxxLarge | ok | Money strip stays on one line; nothing clips but Net worth and Cash sit tight. The design wraps it at this size (money strip). [FU14](#fu14) |
| AX1 | ok | No clipping, truncation or overlap. |
| AX5 | defect | Section heads truncate title and link (`Market upda…`, `Mark…`, `View…`) (toolbars and heads). [FU6](#fu6) |

### notifications / unread

| Size | Verdict | Finding |
| --- | --- | --- |
| Large | ok | No clipping, truncation or overlap. |
| xxLarge | ok | No clipping, truncation or overlap. |
| xxxLarge | ok | No clipping, truncation or overlap. |
| AX1 | ok | No clipping, truncation or overlap. |
| AX5 | defect | Body text truncates (`How much cash wo…`) (no clipping). [FU13](#fu13) |

### activity / ready

| Size | Verdict | Finding |
| --- | --- | --- |
| Large | ok | No clipping, truncation or overlap. |
| xxLarge | defect | Titles truncate (`Deposit rece…`) and subtitles truncate beside the signed money; rows only stack at AX1 (rows stack at AX). [FU4](#fu4) |
| xxxLarge | defect | Titles truncate (`Deposit rece…`) and subtitles truncate beside the signed money; rows only stack at AX1 (rows stack at AX). [FU4](#fu4) |
| AX1 | ok | No clipping, truncation or overlap. |
| AX5 | defect | `Pending` segment broke as `Pend-/ing` (segmented). Fixed here. |

### add-money / ready

| Size | Verdict | Finding |
| --- | --- | --- |
| Large | ok | No clipping, truncation or overlap. |
| xxLarge | defect | Row subtitle truncates (`Continue to the deposit provi…`) (no clipping). [FU3](#fu3) |
| xxxLarge | defect | Row subtitle truncates (`Continue to the deposit provi…`) (no clipping). [FU3](#fu3) |
| AX1 | ok | No clipping, truncation or overlap. |
| AX5 | defect | Prompt and row subtitle truncate (`How would you like to d…`) (no clipping). [FU13](#fu13) |

### card-funding / handoff

| Size | Verdict | Finding |
| --- | --- | --- |
| Large | ok | No clipping, truncation or overlap. |
| xxLarge | ok | No clipping, truncation or overlap. |
| xxxLarge | ok | No clipping, truncation or overlap. |
| AX1 | ok | No clipping, truncation or overlap. |
| AX5 | ok | No clipping, truncation or overlap. |

### cash-out / ready

| Size | Verdict | Finding |
| --- | --- | --- |
| Large | ok | No clipping, truncation or overlap. |
| xxLarge | defect | The pinned keypad and Review button cover the presets and the share-value line, and at AX the amount itself (docks do not cover content). [FU1](#fu1) |
| xxxLarge | defect | The pinned keypad and Review button cover the presets and the share-value line, and at AX the amount itself (docks do not cover content). [FU1](#fu1) |
| AX1 | defect | The pinned keypad and Review button cover the presets and the share-value line, and at AX the amount itself (docks do not cover content). [FU1](#fu1) |
| AX5 | defect | The pinned keypad and Review button cover the presets and the share-value line, and at AX the amount itself (docks do not cover content). [FU1](#fu1) |

### deposit / address

| Size | Verdict | Finding |
| --- | --- | --- |
| Large | ok | No clipping, truncation or overlap. |
| xxLarge | ok | No clipping, truncation or overlap. |
| xxxLarge | ok | No clipping, truncation or overlap. |
| AX1 | ok | No clipping, truncation or overlap. |
| AX5 | defect | `Deposit` tag breaks as `De-/posit` (labels never cut). [FU10](#fu10) |

### fund-cabal / ready

| Size | Verdict | Finding |
| --- | --- | --- |
| Large | ok | No clipping, truncation or overlap. |
| xxLarge | defect | Notice, presets or the pot header sit under the pinned keypad and Review button (docks do not cover content). [FU1](#fu1) |
| xxxLarge | defect | Notice, presets or the pot header sit under the pinned keypad and Review button (docks do not cover content). [FU1](#fu1) |
| AX1 | defect | Notice, presets or the pot header sit under the pinned keypad and Review button (docks do not cover content). [FU1](#fu1) |
| AX5 | defect | Notice, presets or the pot header sit under the pinned keypad and Review button (docks do not cover content). [FU1](#fu1) |

### group-cash / ready

| Size | Verdict | Finding |
| --- | --- | --- |
| Large | ok | No clipping, truncation or overlap. |
| xxLarge | ok | No clipping, truncation or overlap. |
| xxxLarge | ok | No clipping, truncation or overlap. |
| AX1 | ok | No clipping, truncation or overlap. |
| AX5 | ok | No clipping, truncation or overlap. |

### network / ready

| Size | Verdict | Finding |
| --- | --- | --- |
| Large | ok | No clipping, truncation or overlap. |
| xxLarge | ok | No clipping, truncation or overlap. |
| xxxLarge | ok | No clipping, truncation or overlap. |
| AX1 | ok | No clipping, truncation or overlap. |
| AX5 | defect | Row subtitle truncates (`Direct USDC dep…`) (no clipping). [FU13](#fu13) |

### transaction / completed

| Size | Verdict | Finding |
| --- | --- | --- |
| Large | ok | No clipping, truncation or overlap. |
| xxLarge | ok | No clipping, truncation or overlap. |
| xxxLarge | defect | `Account activity` row subtitle truncates (`…transfer his…`) (no clipping). [FU3](#fu3) |
| AX1 | ok | No clipping, truncation or overlap. |
| AX5 | ok | No clipping, truncation or overlap. |

### withdraw / ready

| Size | Verdict | Finding |
| --- | --- | --- |
| Large | ok | No clipping, truncation or overlap. |
| xxLarge | defect | Wallet address field and Paste sit under the pinned Review button and keypad (docks do not cover content). [FU1](#fu1) |
| xxxLarge | defect | Wallet address field and Paste sit under the pinned Review button and keypad (docks do not cover content). [FU1](#fu1) |
| AX1 | defect | Wallet address field and Paste sit under the pinned Review button and keypad (docks do not cover content). [FU1](#fu1) |
| AX5 | defect | Wallet address field and Paste sit under the pinned Review button and keypad (docks do not cover content). [FU1](#fu1) |

### proposal / open

| Size | Verdict | Finding |
| --- | --- | --- |
| Large | ok | No clipping, truncation or overlap. |
| xxLarge | ok | No clipping, truncation or overlap. |
| xxxLarge | ok | No clipping, truncation or overlap. |
| AX1 | ok | No clipping, truncation or overlap. |
| AX5 | defect | Docks (Vote No, Vote Yes, comment, tab bar) take about 45% of the screen; the composer placeholder truncates (`Add a com…`) (docks). [FU12](#fu12) |

### proposals / open

| Size | Verdict | Finding |
| --- | --- | --- |
| Large | ok | No clipping, truncation or overlap. |
| xxLarge | ok | No clipping, truncation or overlap. |
| xxxLarge | ok | No clipping, truncation or overlap. |
| AX1 | ok | No clipping, truncation or overlap. |
| AX5 | ok | No clipping, truncation or overlap. |

### propose / ready

| Size | Verdict | Finding |
| --- | --- | --- |
| Large | ok | No clipping, truncation or overlap. |
| xxLarge | ok | No clipping, truncation or overlap. |
| xxxLarge | ok | No clipping, truncation or overlap. |
| AX1 | ok | No clipping, truncation or overlap. |
| AX5 | ok | No clipping, truncation or overlap. |

### review-proposal / ready

| Size | Verdict | Finding |
| --- | --- | --- |
| Large | ok | No clipping, truncation or overlap. |
| xxLarge | ok | No clipping, truncation or overlap. |
| xxxLarge | ok | No clipping, truncation or overlap. |
| AX1 | ok | No clipping, truncation or overlap. |
| AX5 | ok | No clipping, truncation or overlap. |

### select-stock / buy

| Size | Verdict | Finding |
| --- | --- | --- |
| Large | ok | No clipping, truncation or overlap. |
| xxLarge | ok | No clipping, truncation or overlap. |
| xxxLarge | ok | No clipping, truncation or overlap. |
| AX1 | ok | No clipping, truncation or overlap. |
| AX5 | ok | No clipping, truncation or overlap. |

### trade-amount / ready

| Size | Verdict | Finding |
| --- | --- | --- |
| Large | ok | No clipping, truncation or overlap. |
| xxLarge | defect | Presets sit under the Review button and keypad, and at AX5 the stock and pot header scrolled out (docks do not cover content). [FU1](#fu1) |
| xxxLarge | defect | Presets sit under the Review button and keypad, and at AX5 the stock and pot header scrolled out (docks do not cover content). [FU1](#fu1) |
| AX1 | defect | Presets sit under the Review button and keypad, and at AX5 the stock and pot header scrolled out (docks do not cover content). [FU1](#fu1) |
| AX5 | defect | Presets sit under the Review button and keypad, and at AX5 the stock and pot header scrolled out (docks do not cover content). [FU1](#fu1) |

### votes / open

| Size | Verdict | Finding |
| --- | --- | --- |
| Large | ok | No clipping, truncation or overlap. |
| xxLarge | ok | No clipping, truncation or overlap. |
| xxxLarge | ok | No clipping, truncation or overlap. |
| AX1 | ok | No clipping, truncation or overlap. |
| AX5 | ok | No clipping, truncation or overlap. |

### cabal-info / member

| Size | Verdict | Finding |
| --- | --- | --- |
| Large | ok | No clipping, truncation or overlap. |
| xxLarge | ok | No clipping, truncation or overlap. |
| xxxLarge | ok | No clipping, truncation or overlap. |
| AX1 | ok | No clipping, truncation or overlap. |
| AX5 | ok | No clipping, truncation or overlap. |

### cabal / ready

| Size | Verdict | Finding |
| --- | --- | --- |
| Large | ok | No clipping, truncation or overlap. |
| xxLarge | ok | No clipping, truncation or overlap. |
| xxxLarge | ok | No clipping, truncation or overlap. |
| AX1 | ok | No clipping, truncation or overlap. |
| AX5 | ok | No clipping, truncation or overlap. |

### cabals / ready

| Size | Verdict | Finding |
| --- | --- | --- |
| Large | ok | No clipping, truncation or overlap. |
| xxLarge | ok | Rail and list cards grow with their content but not to F7's large-text size, which starts at AX1 (cover cards). [FU5](#fu5) |
| xxxLarge | defect | Cover card footer truncates (`Return opens s…`) (cover cards). [FU5](#fu5) |
| AX1 | ok | No clipping, truncation or overlap. |
| AX5 | ok | No clipping, truncation or overlap. |

### chat / ready

| Size | Verdict | Finding |
| --- | --- | --- |
| Large | ok | No clipping, truncation or overlap. |
| xxLarge | ok | No clipping, truncation or overlap. |
| xxxLarge | ok | No clipping, truncation or overlap. |
| AX1 | ok | No clipping, truncation or overlap. |
| AX5 | defect | Composer placeholder truncates (`Message yo…`) (docks). [FU12](#fu12) |

### create-cabal / ready

| Size | Verdict | Finding |
| --- | --- | --- |
| Large | ok | No clipping, truncation or overlap. |
| xxLarge | ok | No clipping, truncation or overlap. |
| xxxLarge | ok | No clipping, truncation or overlap. |
| AX1 | ok | No clipping, truncation or overlap. |
| AX5 | defect | `Add a cover photo` tile label truncates (`Add a cover ph…`) (labels never cut). [FU9](#fu9) |

### discover / results

| Size | Verdict | Finding |
| --- | --- | --- |
| Large | ok | No clipping, truncation or overlap. |
| xxLarge | ok | Cabal names end in `…` beside the Request pill, which the rules allow for names. |
| xxxLarge | ok | Cabal names end in `…` beside the Request pill, which the rules allow for names. |
| AX1 | ok | No clipping, truncation or overlap. |
| AX5 | ok | No clipping, truncation or overlap. |

### holding / ready

| Size | Verdict | Finding |
| --- | --- | --- |
| Large | ok | No clipping, truncation or overlap. |
| xxLarge | ok | No clipping, truncation or overlap. |
| xxxLarge | ok | No clipping, truncation or overlap. |
| AX1 | ok | No clipping, truncation or overlap. |
| AX5 | defect | Position P&L truncates (`+$525.…`) (money never truncates). [FU7](#fu7) |

### invite / share

| Size | Verdict | Finding |
| --- | --- | --- |
| Large | ok | No clipping, truncation or overlap. |
| xxLarge | ok | `Copy link` sits under the bottom edge of the `.votes` sheet until expanded (sheets). |
| xxxLarge | ok | `Copy link` sits under the bottom edge of the `.votes` sheet until expanded (sheets). |
| AX1 | ok | No clipping, truncation or overlap. |
| AX5 | ok | No clipping, truncation or overlap. |

### members / members

| Size | Verdict | Finding |
| --- | --- | --- |
| Large | ok | No clipping, truncation or overlap. |
| xxLarge | ok | No clipping, truncation or overlap. |
| xxxLarge | ok | No clipping, truncation or overlap. |
| AX1 | ok | No clipping, truncation or overlap. |
| AX5 | defect | `Members` segment broke as `Mem-/bers` (segmented). Fixed here. |

### public-cabal / preview

| Size | Verdict | Finding |
| --- | --- | --- |
| Large | ok | No clipping, truncation or overlap. |
| xxLarge | ok | No clipping, truncation or overlap. |
| xxxLarge | ok | No clipping, truncation or overlap. |
| AX1 | ok | No clipping, truncation or overlap. |
| AX5 | ok | No clipping, truncation or overlap. |

### instrument / ready

| Size | Verdict | Finding |
| --- | --- | --- |
| Large | ok | No clipping, truncation or overlap. |
| xxLarge | defect | Range chip `ALL` truncated (`A…`) and the chart time-axis labels overlap (segmented controls). Chips fixed here: they now scroll horizontally, so `ALL` starts at the edge and scrolls into view. Axis stays. [FU8](#fu8) |
| xxxLarge | defect | Range chip `ALL` truncated (`A…`) and the chart time-axis labels overlap (segmented controls). Chips fixed here: they now scroll horizontally, so `ALL` starts at the edge and scrolls into view. Axis stays. [FU8](#fu8) |
| AX1 | defect | Range chips wrapped mid-word (`1/W`, `AL/L`) (segmented). Fixed here: the chips now scroll horizontally. Axis labels still overlap. [FU8](#fu8) |
| AX5 | defect | Stock name breaks mid-word (`Alpha-/bet`), the chart-style toggle is cut off at the right edge and the axis labels overlap. Chips fixed here. [FU8](#fu8) |

### markets / ready

| Size | Verdict | Finding |
| --- | --- | --- |
| Large | ok | No clipping, truncation or overlap. |
| xxLarge | ok | No clipping, truncation or overlap. |
| xxxLarge | ok | No clipping, truncation or overlap. |
| AX1 | ok | No clipping, truncation or overlap. |
| AX5 | ok | No clipping, truncation or overlap. |

### watchlist / ready

| Size | Verdict | Finding |
| --- | --- | --- |
| Large | ok | No clipping, truncation or overlap. |
| xxLarge | ok | No clipping, truncation or overlap. |
| xxxLarge | ok | No clipping, truncation or overlap. |
| AX1 | ok | No clipping, truncation or overlap. |
| AX5 | ok | No clipping, truncation or overlap. |

### feed / all

| Size | Verdict | Finding |
| --- | --- | --- |
| Large | ok | No clipping, truncation or overlap. |
| xxLarge | ok | Post card thesis preview ends in `…` after three lines (preview, not money or name). |
| xxxLarge | ok | Post card thesis preview ends in `…` after three lines (preview, not money or name). |
| AX1 | ok | No clipping, truncation or overlap. |
| AX5 | ok | No clipping, truncation or overlap. |

### post / ready

| Size | Verdict | Finding |
| --- | --- | --- |
| Large | ok | No clipping, truncation or overlap. |
| xxLarge | ok | No clipping, truncation or overlap. |
| xxxLarge | ok | No clipping, truncation or overlap. |
| AX1 | ok | No clipping, truncation or overlap. |
| AX5 | defect | Composer placeholder truncates (`Add a com…`) (docks). [FU12](#fu12) |

### leaderboard / cabals

| Size | Verdict | Finding |
| --- | --- | --- |
| Large | ok | No clipping, truncation or overlap. |
| xxLarge | ok | Cabal names end in `…`, which the rules allow; signed money stays whole. |
| xxxLarge | ok | Cabal names end in `…`, which the rules allow; signed money stays whole. |
| AX1 | ok | No clipping, truncation or overlap. |
| AX5 | ok | No clipping, truncation or overlap. |

### edit-profile / ready

| Size | Verdict | Finding |
| --- | --- | --- |
| Large | ok | No clipping, truncation or overlap. |
| xxLarge | ok | No clipping, truncation or overlap. |
| xxxLarge | ok | No clipping, truncation or overlap. |
| AX1 | ok | No clipping, truncation or overlap. |
| AX5 | ok | No clipping, truncation or overlap. |

### people / results

| Size | Verdict | Finding |
| --- | --- | --- |
| Large | ok | No clipping, truncation or overlap. |
| xxLarge | ok | No clipping, truncation or overlap. |
| xxxLarge | ok | No clipping, truncation or overlap. |
| AX1 | ok | No clipping, truncation or overlap. |
| AX5 | defect | `Discover` and `Following` broke mid-word (segmented). Fixed here. |

### person / public

| Size | Verdict | Finding |
| --- | --- | --- |
| Large | ok | No clipping, truncation or overlap. |
| xxLarge | defect | `Public activity` row subtitle truncates (`…discuss…`) (no clipping). [FU3](#fu3) |
| xxxLarge | defect | `Public activity` row subtitle truncates (`…discuss…`) (no clipping). [FU3](#fu3) |
| AX1 | ok | No clipping, truncation or overlap. |
| AX5 | ok | No clipping, truncation or overlap. |

### profile / ready

| Size | Verdict | Finding |
| --- | --- | --- |
| Large | ok | No clipping, truncation or overlap. |
| xxLarge | defect | Settings-list row subtitles truncate (`Deposits, withdrawals and re…`) (no clipping). [FU3](#fu3) |
| xxxLarge | ok | No clipping, truncation or overlap. |
| AX1 | ok | No clipping, truncation or overlap. |
| AX5 | defect | Settings-list row subtitles truncate (`Deposits, withdrawals and re…`) (no clipping). [FU3](#fu3) |

### settings / ready

| Size | Verdict | Finding |
| --- | --- | --- |
| Large | ok | No clipping, truncation or overlap. |
| xxLarge | defect | Row subtitles truncate (`Send cash to an external wall…`) (no clipping). [FU3](#fu3) |
| xxxLarge | defect | Row subtitles truncate (`Send cash to an external wall…`) (no clipping). [FU3](#fu3) |
| AX1 | ok | No clipping, truncation or overlap. |
| AX5 | defect | Row title breaks mid-word (`Notificatio-/ns`) and the subtitle truncates (labels never cut). [FU11](#fu11) |

## Rules that held everywhere

- Every `Typo` role scaled with the text size and nothing drew under the 15pt floor.
- Money figures stayed whole outside the defects listed above. Hero figures scaled and wrapped without shrinking.
- Rows with a leading avatar and a trailing chevron stack their trailing content under the title at AX1 and AX5 on every list screen except where [FU4](#fu4) and [FU11](#fu11) say otherwise.
- `StatGrid` and the two-stat rule stack at AX1 and AX5 and are side by side below that; no cell is narrower than its figure.
- Full-width buttons, the Deposit capsule and the compact buttons kept a 44pt hit frame at every size (`DesignLayoutTests`).
- Every sheet in the set (propose, review, votes, share) opened `.large` at AX1 and AX5 with its pinned footer visible. Below AX1 each opened at its design detent.
- Toolbars: the title stayed on one line beside the 44pt round buttons at every size; large root titles wrapped.
- The system tab bar followed iOS and nothing custom was drawn over it.

## Follow-ups

Filed as issues when the lanes pick them up; each names the lane that owns the file. Lane A owns S1–S7 (#5558–#5564), Lane B owns S8–S14 (#5565–#5571), and `Design/*` goes back to its F ticket's lane.

### FU1

Amount-entry screens: the pinned keypad and Review button leave too little room at xxLarge and above, so presets, notices and headers sit under it until scrolled. Owner: Lane A (#5561, #5563).

### FU2

`CircleAction` tiles stay three across at AX5 and `Withdraw` breaks as `With-/draw`; they should stack at AX sizes. Owner: F lane (`MonacoButtons.swift`), balances #5560, cabal #5566.

### FU3

Single-line row subtitles truncate (`…`) at xxLarge and xxxLarge, and at AX5 in some rows (`MonacoRow` subtitle `lineLimit`). Owner: F lane (`MonacoList.swift`).

### FU4

Rows with a trailing money figure keep their trailing column until AX1, so the title and subtitle truncate at xxLarge and xxxLarge; stack from xxxLarge when the title would truncate. Owner: F lane (`MonacoRowLayout`), Account activity #5562.

### FU5

`CabalCoverCard`: the signed P&L figure truncates (`−$300.…`) in the 300pt rail card at xxLarge and AX5, the footer truncates at xxxLarge, and the rail and list cards do not reach F7's 330×260 / 260 size until AX1. Owner: F lane (`CabalCoverCard.swift`, F7).

### FU6

Section heads truncate their title and link at AX5 (`Market upda…`, `Mark…`, `View…`); they should wrap, align to the top and cap the title at 65% width. Owner: Lane A (Home #5559) and F lane (section head).

### FU7

Holding: the position P&L truncates (`+$525.…`) at AX5 and the holdings row subtitle truncates a money figure (`$34…`). Owner: Lane B (#5567, #5566).

### FU8

Instrument: chart time-axis labels overlap from xxLarge up, the stock name breaks mid-word (`Alpha-/bet`) beside the star at AX5, and the chart-style toggle is cut off at the right edge at AX5. Owner: Lane B (#5568), F lane (candle chart).

### FU9

Start a cabal: the `Add a cover photo` tile label truncates at AX5. Owner: Lane B (#5565).

### FU10

Receive USDC: the `Deposit` tag breaks as `De-/posit` beside the title at AX5; stack the tag under the title. Owner: Lane A (#5561).

### FU11

Settings: the `Notifications` row title breaks mid-word at AX5 and subtitles truncate. Owner: Lane B (#5571), F lane (`MonacoRow`).

### FU12

Proposal: the Vote No, Vote Yes, comment and tab-bar docks take about 45% of the screen at AX5; composer placeholders truncate (`Add a com…`) in chat, post and proposal. Owner: Lane A (#5564), Lane B (#5567, #5569).

### FU13

Copy truncation at AX5: the activity body (`…cash wo…`) and the Deposit prompt (`How would you like to d…`) and the network row subtitle. Owner: Lane A (#5560, #5561).

### FU14

Money strip: Net worth, Cash and Deposit stay on one line at xxLarge and xxxLarge and wrap from AX1; the design wraps at its 1.22 text size. Owner: Lane A (#5559) and Lane B (#5571), decision.

### FU15

`MonacoSheetDetents.detents(fittedHeight:accessibilitySize:)` is `fileprivate`, so `DesignLayoutTests` cannot pin that sheets return only `.large` at AX sizes; the shots show every sheet opening `.large`. Owner: F lane (`MonacoAppearance.swift`).

## Audit and unit tests

- `V3DynamicTypeAuditUITests` launches the 43 default states at `xxxLarge` and `AX5`, runs `performAccessibilityAudit(for: [.dynamicType, .textClipped])` and swipes up twice per screen, auditing at each position. It is split into four tests (two sizes, two halves of the list), because one size over 43 screens takes about 25 minutes and the machine's build slot is held for at most 30. It runs locally only; the CI test plan runs `MonacoTests`.
- A raw run reported 215 `.dynamicType` issues and 119 `.textClipped` issues at AX5 alone. Every `.dynamicType` report is allowed, for one reason: `Typo.scaled` returns `Font.system(size:)` already scaled for the launch size, so the audit's runtime font check reads it as fixed (`MonacoTheme.swift`). The text did scale: the shots show it. A `.textClipped` report is allowed when the element sits partly under the navigation bar, under the tab bar or outside the window while the screen is scrolled, or has no element. What remains is an allow-list of 34 rows, each with a reason: single-line text fields (their text is the accessibility value), the real defects in this doc, text inside sheets that the shots show whole, and text that a line limit cuts on purpose.
- `V3LargeTextLayoutTests` in `DesignLayoutTests.swift` pins the rail card size at xxLarge and above, the `MoneyStrip` wrap, `StatGrid` and `StatPair` stacking at AX1, the 44pt Deposit capsule at every size and the `MonacoSegmented` stacking. The sheet detent rule has no seam ([FU15](#fu15)).
