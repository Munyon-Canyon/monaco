# Dynamic Type audit, F launch (phase 1)

This page is the result of #6586's large-text pass over the 95 `screen` launch frames of #6583's inventory (`scripts/qa/parity.py frames --kind screen`), at five text sizes, in Dark. Each frame was shot from its `f__<frame>` line in `scripts/qa/sample-screens.txt` and judged against the rules in #6586's Implement step 2. Parity at the default size is #6583's (`parity.md`), Light is #6585's and VoiceOver is #6587's.

## How it was run

Build: Debug `Monaco.app` from `149c981ab` (`staging` at `0e9545061` plus #7078's test files; no app source differs), lane simulator `Monaco 6586` (iPhone 17 Pro class, iOS 27.0, stock: SimSlim is not installed on this Mac). One simulator, booted with `xcrun simctl boot` under `scripts/qa/xcode-lock.sh sim` and shut down at the end of each run.

    udid="$(scripts/gold-sim-udid.sh)"
    xcrun simctl boot "$udid"
    xcrun simctl ui "$udid" appearance dark
    for size in large extra-extra-large extra-extra-extra-large accessibility-medium accessibility-extra-extra-extra-large; do
      xcrun simctl ui "$udid" content_size "$size"
      MONACO_QA_SCREEN_ONLY="$(scripts/qa/parity.py frames --kind screen --regex)" scripts/qa/screens.sh "$udid" <Debug Monaco.app> "$out/$size"
    done
    xcrun simctl ui "$udid" content_size large
    xcrun simctl shutdown "$udid"

All 475 pairs were shot. The sim lock's 30-minute hold stopped the first run partway through `extra-extra-extra-large`; the second run shot the six frames it had missed, then AX1 and AX5. Nine frames were shot again at all five sizes with `MONACO_QA_SCREEN_SETTLE=8` because their first shots showed the wrong screen. Even then, `15-passed` draws an empty page at every size, and the `16-catch-up*` lines sometimes stop on Home, so 20 pairs are recorded as `no shot` (#7127). Each frame's five shots were laid side by side and judged by eye against the rules.

`FDynamicTypeAuditUITests` (#7078) runs `.dynamicType` and `.textClipped` on the same 95 frames at xxxLarge and AX5, one test method per frame group: `scripts/qa/xcode-lock.sh xcode xcodebuild test-without-building ... -only-testing:MonacoUITests/FDynamicTypeAuditUITests/<group>`. Its allow-list holds only reports with no element and the members-all request row's false positive (#7201) and the review sheet's backdrop 'Propose buy' text (#7206), so while the defects below are open it fails on their frames; it was not run to completion in #6586 (see [Next steps](#next-steps)).

Sizes: `large` is the default, `xxLarge` is the prototype's "Larger text" step, `xxxLarge` is the largest non-accessibility size, `AX1` is `accessibility-medium` and `AX5` is `accessibility-extra-extra-extra-large`.

Shots stay outside the repo; the AX5 sheets of the dense frames are attached to the PR.

## Reading the verdicts

- `ok`: in the first viewport, no money, ticker, count, button, tab or period label is clipped or truncated, and the frame's rows, tally, stat row, keypad, tabs, sheet, toolbar, dock and toast follow Implement step 2 at that size. A single-line `…` on a cabal or person name in a row counts as `ok`.
- `defect`: the row names the rule it breaks and the follow-up issue that owns the fix.

Long screens are judged on the first viewport; the `-scrolled` frames carry the scrolled state, and `FDynamicTypeAuditUITests` audits two swipes further down.

## Summary

95 launch screen frames at five sizes: 475 frame-size pairs.

| Verdict | Pairs |
| --- | ---: |
| ok | 297 |
| defect | 158 |
| no shot | 20 |

Defects by size: large 4, xxLarge 16, xxxLarge 26, AX1 42, AX5 70.

## Frames 02

| Frame | Size | Verdict | Rule broken | Follow-up |
| --- | --- | --- | --- | --- |
| [`02-home-dark`](../screens/02-home-dark.png) | large | ok | — | — |
| [`02-home-dark`](../screens/02-home-dark.png) | xxLarge | ok | — | — |
| [`02-home-dark`](../screens/02-home-dark.png) | xxxLarge | ok | — | — |
| [`02-home-dark`](../screens/02-home-dark.png) | AX1 | defect | 2 rows stack: Cash - USDC row keeps $500.00 beside the label instead of stacking under it | #7118 |
| [`02-home-dark`](../screens/02-home-dark.png) | AX5 | defect | 1 no truncation: 'All proposals' link ends in 'All pro...'; 'Your vote' wraps beside it and the '+$241.10 past - 2.0% month' line breaks into fragments | #7118 |
| [`02-home-light`](../screens/02-home-light.png) | large | ok | — | — |
| [`02-home-light`](../screens/02-home-light.png) | xxLarge | ok | — | — |
| [`02-home-light`](../screens/02-home-light.png) | xxxLarge | ok | — | — |
| [`02-home-light`](../screens/02-home-light.png) | AX1 | defect | 2 rows stack: Cash - USDC row keeps $500.00 beside the label instead of stacking under it | #7118 |
| [`02-home-light`](../screens/02-home-light.png) | AX5 | defect | 1 no truncation: 'All proposals' link ends in 'All pro...'; 'Your vote' wraps beside it and the '+$241.10 past - 2.0% month' line breaks into fragments | #7118 |
| [`02-home-scrolled`](../screens/02-home-scrolled.png) | large | ok | — | — |
| [`02-home-scrolled`](../screens/02-home-scrolled.png) | xxLarge | ok | — | — |
| [`02-home-scrolled`](../screens/02-home-scrolled.png) | xxxLarge | ok | — | — |
| [`02-home-scrolled`](../screens/02-home-scrolled.png) | AX1 | ok | — | — |
| [`02-home-scrolled`](../screens/02-home-scrolled.png) | AX5 | defect | 1 nothing truncated: Section header 'Your cabals 3 · 1 invited' breaks mid-word ('You/r/ca-/bals', 'in-/vit-/ed') in a narrow column beside 'See all' | #7118 |
| [`02-home-empty`](../screens/02-home-empty.png) | large | ok | — | — |
| [`02-home-empty`](../screens/02-home-empty.png) | xxLarge | ok | — | — |
| [`02-home-empty`](../screens/02-home-empty.png) | xxxLarge | ok | — | — |
| [`02-home-empty`](../screens/02-home-empty.png) | AX1 | defect | 2 rows stack: feed row 'Buy AMZN $250' has the vote tally 3 - 0 beside the title, not stacked; Cash row Deposit ok | #7118 |
| [`02-home-empty`](../screens/02-home-empty.png) | AX5 | defect | 12 overlap: card icon drawn over '3.5' in 'Cash - USDC 3.5 % APY'; $500.00 beside label, not stacked | #7118 |

`FDynamicTypeAuditUITests/test02Home` passes at xxxLarge and AX5 (#7196). From `xxLarge` the Home first-use actions stack full width, and the empty-state copy, the standing line, the leaderboard row and the feed preview rows wrap instead of ending in `…`. The feed preview meta line draws its reply count as its own icon and number, so the audit reads the visible text. A Home slot that is hidden draws no content, so the audit no longer reads text clipped to zero height.

The Leaderboard period line wraps and grows at accessibility sizes instead of clipping (#7212).

## Frames 04

| Frame | Size | Verdict | Rule broken | Follow-up |
| --- | --- | --- | --- | --- |
| [`04b-hold-1-pick`](../screens/04b-hold-1-pick.png) | large | ok | — | — |
| [`04b-hold-1-pick`](../screens/04b-hold-1-pick.png) | xxLarge | ok | — | — |
| [`04b-hold-1-pick`](../screens/04b-hold-1-pick.png) | xxxLarge | ok | — | — |
| [`04b-hold-1-pick`](../screens/04b-hold-1-pick.png) | AX1 | ok | — | — |
| [`04b-hold-1-pick`](../screens/04b-hold-1-pick.png) | AX5 | defect | 2 rows stack / 12 overlap: Cash · USDC row does not stack: card icon drawn over '3.5', '3.5%' split across lines ('3.5 / % / APY'), $500.00 beside it; Big Ballers '1' badge overlaps '1 vote open' | #7118 |
| [`04b-hold-2-holding`](../screens/04b-hold-2-holding.png) | large | ok | — | — |
| [`04b-hold-2-holding`](../screens/04b-hold-2-holding.png) | xxLarge | ok | — | — |
| [`04b-hold-2-holding`](../screens/04b-hold-2-holding.png) | xxxLarge | ok | — | — |
| [`04b-hold-2-holding`](../screens/04b-hold-2-holding.png) | AX1 | ok | — | — |
| [`04b-hold-2-holding`](../screens/04b-hold-2-holding.png) | AX5 | no shot | capture: Home scrolled past the vote card; the holding state ('Keep holding') is not on screen | #7127 |
| [`04b-hold-3-committed`](../screens/04b-hold-3-committed.png) | large | ok | — | — |
| [`04b-hold-3-committed`](../screens/04b-hold-3-committed.png) | xxLarge | ok | — | — |
| [`04b-hold-3-committed`](../screens/04b-hold-3-committed.png) | xxxLarge | ok | — | — |
| [`04b-hold-3-committed`](../screens/04b-hold-3-committed.png) | AX1 | ok | — | — |
| [`04b-hold-3-committed`](../screens/04b-hold-3-committed.png) | AX5 | defect | 2 rows stack / 12 overlap: Cash · USDC row does not stack: card icon drawn over '3.5', '3.5%' split across lines ('3.5 / % / APY'), $500.00 beside it | #7118 |
| [`04-proposal-dark`](../screens/04-proposal-dark.png) | large | ok | — | — |
| [`04-proposal-dark`](../screens/04-proposal-dark.png) | xxLarge | ok | — | — |
| [`04-proposal-dark`](../screens/04-proposal-dark.png) | xxxLarge | ok | — | — |
| [`04-proposal-dark`](../screens/04-proposal-dark.png) | AX1 | ok | — | — |
| [`04-proposal-dark`](../screens/04-proposal-dark.png) | AX5 | ok | — | — |
| [`04-proposal-light`](../screens/04-proposal-light.png) | large | ok | — | — |
| [`04-proposal-light`](../screens/04-proposal-light.png) | xxLarge | ok | — | — |
| [`04-proposal-light`](../screens/04-proposal-light.png) | xxxLarge | ok | — | — |
| [`04-proposal-light`](../screens/04-proposal-light.png) | AX1 | ok | — | — |
| [`04-proposal-light`](../screens/04-proposal-light.png) | AX5 | ok | — | — |
| [`04-proposal-scrolled`](../screens/04-proposal-scrolled.png) | large | ok | — | — |
| [`04-proposal-scrolled`](../screens/04-proposal-scrolled.png) | xxLarge | ok | — | — |
| [`04-proposal-scrolled`](../screens/04-proposal-scrolled.png) | xxxLarge | ok | — | — |
| [`04-proposal-scrolled`](../screens/04-proposal-scrolled.png) | AX1 | ok | — | — |
| [`04-proposal-scrolled`](../screens/04-proposal-scrolled.png) | AX5 | defect | 1 clipped: Reply quote 'Logan 44%...' ends in '...' (not a name) | #7120 |
| [`04-vote-idle`](../screens/04-vote-idle.png) | large | ok | — | — |
| [`04-vote-idle`](../screens/04-vote-idle.png) | xxLarge | ok | — | — |
| [`04-vote-idle`](../screens/04-vote-idle.png) | xxxLarge | ok | — | — |
| [`04-vote-idle`](../screens/04-vote-idle.png) | AX1 | ok | — | — |
| [`04-vote-idle`](../screens/04-vote-idle.png) | AX5 | ok | — | — |
| [`04-vote-picked`](../screens/04-vote-picked.png) | large | ok | — | — |
| [`04-vote-picked`](../screens/04-vote-picked.png) | xxLarge | ok | — | — |
| [`04-vote-picked`](../screens/04-vote-picked.png) | xxxLarge | ok | — | — |
| [`04-vote-picked`](../screens/04-vote-picked.png) | AX1 | ok | — | — |
| [`04-vote-picked`](../screens/04-vote-picked.png) | AX5 | ok | — | — |
| [`04-vote-holding`](../screens/04-vote-holding.png) | large | ok | — | — |
| [`04-vote-holding`](../screens/04-vote-holding.png) | xxLarge | ok | — | — |
| [`04-vote-holding`](../screens/04-vote-holding.png) | xxxLarge | ok | — | — |
| [`04-vote-holding`](../screens/04-vote-holding.png) | AX1 | ok | — | — |
| [`04-vote-holding`](../screens/04-vote-holding.png) | AX5 | ok | — | — |
| [`04-vote-committed`](../screens/04-vote-committed.png) | large | ok | — | — |
| [`04-vote-committed`](../screens/04-vote-committed.png) | xxLarge | ok | — | — |
| [`04-vote-committed`](../screens/04-vote-committed.png) | xxxLarge | ok | — | — |
| [`04-vote-committed`](../screens/04-vote-committed.png) | AX1 | ok | — | — |
| [`04-vote-committed`](../screens/04-vote-committed.png) | AX5 | ok | — | — |
| [`04-vote-closed`](../screens/04-vote-closed.png) | large | ok | — | — |
| [`04-vote-closed`](../screens/04-vote-closed.png) | xxLarge | ok | — | — |
| [`04-vote-closed`](../screens/04-vote-closed.png) | xxxLarge | ok | — | — |
| [`04-vote-closed`](../screens/04-vote-closed.png) | AX1 | ok | — | — |
| [`04-vote-closed`](../screens/04-vote-closed.png) | AX5 | ok | — | — |
| [`04-proposal-passed`](../screens/04-proposal-passed.png) | large | ok | — | — |
| [`04-proposal-passed`](../screens/04-proposal-passed.png) | xxLarge | ok | — | — |
| [`04-proposal-passed`](../screens/04-proposal-passed.png) | xxxLarge | ok | — | — |
| [`04-proposal-passed`](../screens/04-proposal-passed.png) | AX1 | ok | — | — |
| [`04-proposal-passed`](../screens/04-proposal-passed.png) | AX5 | ok | — | — |
| [`04-proposal-sides`](../screens/04-proposal-sides.png) | large | ok | — | — |
| [`04-proposal-sides`](../screens/04-proposal-sides.png) | xxLarge | ok | — | — |
| [`04-proposal-sides`](../screens/04-proposal-sides.png) | xxxLarge | ok | — | — |
| [`04-proposal-sides`](../screens/04-proposal-sides.png) | AX1 | ok | — | — |
| [`04-proposal-sides`](../screens/04-proposal-sides.png) | AX5 | ok | — | — |
| [`04-proposal-failed`](../screens/04-proposal-failed.png) | large | ok | — | — |
| [`04-proposal-failed`](../screens/04-proposal-failed.png) | xxLarge | ok | — | — |
| [`04-proposal-failed`](../screens/04-proposal-failed.png) | xxxLarge | ok | — | — |
| [`04-proposal-failed`](../screens/04-proposal-failed.png) | AX1 | ok | — | — |
| [`04-proposal-failed`](../screens/04-proposal-failed.png) | AX5 | ok | — | — |
| [`04-proposal-preipo`](../screens/04-proposal-preipo.png) | large | defect | 1 clipped: Stat cell 'Closes' value ends in '...' ('Fri 11:06...') | #7120 |
| [`04-proposal-preipo`](../screens/04-proposal-preipo.png) | xxLarge | ok | — | — |
| [`04-proposal-preipo`](../screens/04-proposal-preipo.png) | xxxLarge | ok | — | — |
| [`04-proposal-preipo`](../screens/04-proposal-preipo.png) | AX1 | ok | — | — |
| [`04-proposal-preipo`](../screens/04-proposal-preipo.png) | AX5 | defect | 1 clipped: Title breaks mid-word 'Buy / Space / X' and 'sec-/ondary' in the 'Pre-IPO · priced at the secondary estimate' subtitle, squeezed beside the $500 figure | #7120 |
| [`04-reply-composer`](../screens/04-reply-composer.png) | large | ok | — | — |
| [`04-reply-composer`](../screens/04-reply-composer.png) | xxLarge | ok | — | — |
| [`04-reply-composer`](../screens/04-reply-composer.png) | xxxLarge | ok | — | — |
| [`04-reply-composer`](../screens/04-reply-composer.png) | AX1 | ok | — | — |
| [`04-reply-composer`](../screens/04-reply-composer.png) | AX5 | defect | 1 clipped: Quoted reply line 'Mia What would our NVI...' ends in '...' | #7120 |
| [`04-composer-menu`](../screens/04-composer-menu.png) | large | ok | — | — |
| [`04-composer-menu`](../screens/04-composer-menu.png) | xxLarge | ok | — | — |
| [`04-composer-menu`](../screens/04-composer-menu.png) | xxxLarge | ok | — | — |
| [`04-composer-menu`](../screens/04-composer-menu.png) | AX1 | defect | 12 overlap: Upper 'Say something' composer is cut off by the second composer drawn over it; two composers stacked | #7120 |
| [`04-composer-menu`](../screens/04-composer-menu.png) | AX5 | defect | 12 overlap: Two composers overlap: 'Say so...' placeholder and GIF capsule drawn over the other composer | #7120 |
| [`04-gif-sheet`](../screens/04-gif-sheet.png) | large | defect | 12 overlap: Stray 'posts=0' label drawn over/under the 'Proposal' nav title | #7120 |
| [`04-gif-sheet`](../screens/04-gif-sheet.png) | xxLarge | defect | 12 overlap: Stray 'posts=0' label drawn over/under the 'Proposal' nav title | #7120 |
| [`04-gif-sheet`](../screens/04-gif-sheet.png) | xxxLarge | defect | 12 overlap: Stray 'posts=0' label drawn over/under the 'Proposal' nav title | #7120 |
| [`04-gif-sheet`](../screens/04-gif-sheet.png) | AX1 | defect | 12 overlap: Stray 'posts=0' label drawn over/under the 'Proposal' nav title | #7120 |
| [`04-gif-sheet`](../screens/04-gif-sheet.png) | AX5 | defect | 12 overlap: Stray 'posts=0' label overlaps the nav title and 'QA pot' line; sheet header breaks words mid-word ('Trend-ing GIFs', 'Pow-ered by GIPHY') | #7120 |
| [`04-gif-reply`](../screens/04-gif-reply.png) | large | ok | — | — |
| [`04-gif-reply`](../screens/04-gif-reply.png) | xxLarge | ok | — | — |
| [`04-gif-reply`](../screens/04-gif-reply.png) | xxxLarge | ok | — | — |
| [`04-gif-reply`](../screens/04-gif-reply.png) | AX1 | ok | — | — |
| [`04-gif-reply`](../screens/04-gif-reply.png) | AX5 | defect | 12 overlap: Stray 'posts=0' label drawn over the 'Replies 2' header text | #7120 |

`FDynamicTypeAuditUITests/test04Proposal`'s clipped-text findings at xxxLarge and AX5 are fixed by #7202, apart from the `04b-hold-*` Home rows, which #7196 fixed. From `xxxLarge` the GIF sheet header stacks, so "Trending GIFs" and "Powered by GIPHY" wrap, and the reply composer's quote wraps instead of ending in `…`. A sheet title such as "Reply to Mia" wraps at every size. The vote status line ("3 of 7 voted · ends in 23h") wraps (#7203). The test was not rerun in #7202; the next group-by-group run confirms these rows.

## Frames 12

#7211 stacks the info sheet rows from xxxLarge, lets sheet titles, info rows, the Home feed row subtitle and every Monaco button wrap to full height from xxxLarge, so the 19 `FDynamicTypeAuditUITests/test12Sheets` clipped-text findings at xxxLarge and AX5 are fixed in code; the audit rerun is pending.

| Frame | Size | Verdict | Rule broken | Follow-up |
| --- | --- | --- | --- | --- |
| [`12-toast`](../screens/12-toast.png) | large | ok | — | — |
| [`12-toast`](../screens/12-toast.png) | xxLarge | ok | — | — |
| [`12-toast`](../screens/12-toast.png) | xxxLarge | ok | — | — |
| [`12-toast`](../screens/12-toast.png) | AX1 | defect | 11 toast / 12 overlap: Toast 'Voted Yes. Passed 5–1.' stays one line and is drawn over the bell button (right edge covers it); should wrap | #7126 |
| [`12-toast`](../screens/12-toast.png) | AX5 | defect | 1 truncation / 12 overlap: Two-line toast overlays the home hero figure and top bar; 'All proposals' link truncated to 'All pro…'; 'Your vote' header wraps beside it | #7126 |
| [`12-info-sheet`](../screens/12-info-sheet.png) | large | ok | — | — |
| [`12-info-sheet`](../screens/12-info-sheet.png) | xxLarge | ok | — | — |
| [`12-info-sheet`](../screens/12-info-sheet.png) | xxxLarge | ok | — | — |
| [`12-info-sheet`](../screens/12-info-sheet.png) | AX1 | ok | — | — |
| [`12-info-sheet`](../screens/12-info-sheet.png) | AX5 | defect | 8 sheets: Sheet grows to fill screen; pinned 'OK' button not visible and Source/Updated/Delay rows pushed off | #7126 |

## Frames 03

| Frame | Size | Verdict | Rule broken | Follow-up |
| --- | --- | --- | --- | --- |
| [`03-cabals-tab`](../screens/03-cabals-tab.png) | large | ok | — | — |
| [`03-cabals-tab`](../screens/03-cabals-tab.png) | xxLarge | defect | 1 no truncation: '$20,000 treasury - 35...' subtitle truncated in 'Your share in each' rows | #7119 |
| [`03-cabals-tab`](../screens/03-cabals-tab.png) | xxxLarge | defect | 1 no truncation: 'New cabal' label cut to 'Ne...cabal'; '$20,000 treasury -...' subtitle truncated | #7119 |
| [`03-cabals-tab`](../screens/03-cabals-tab.png) | AX1 | ok | — | — |
| [`03-cabals-tab`](../screens/03-cabals-tab.png) | AX5 | defect | 12 overlap: tile badge '1' drawn over 'AC' letters of the first avatar | #7119 |
| [`03-cabals-tab-menu`](../screens/03-cabals-tab-menu.png) | large | ok | — | — |
| [`03-cabals-tab-menu`](../screens/03-cabals-tab-menu.png) | xxLarge | ok | — | — |
| [`03-cabals-tab-menu`](../screens/03-cabals-tab-menu.png) | xxxLarge | ok | — | — |
| [`03-cabals-tab-menu`](../screens/03-cabals-tab-menu.png) | AX1 | ok | — | — |
| [`03-cabals-tab-menu`](../screens/03-cabals-tab-menu.png) | AX5 | defect | 1 no truncation: menu header '+4.7...' return truncated and 'y...' ('you are admin') ends in '...'; badge '1' covers the 'AC' avatar | #7119 |
| [`03-cabal-treasury`](../screens/03-cabal-treasury.png) | large | ok | — | — |
| [`03-cabal-treasury`](../screens/03-cabal-treasury.png) | xxLarge | ok | — | — |
| [`03-cabal-treasury`](../screens/03-cabal-treasury.png) | xxxLarge | ok | — | — |
| [`03-cabal-treasury`](../screens/03-cabal-treasury.png) | AX1 | ok | — | — |
| [`03-cabal-treasury`](../screens/03-cabal-treasury.png) | AX5 | ok | — | — |
| [`03-cabal-treasury-light`](../screens/03-cabal-treasury-light.png) | large | ok | — | — |
| [`03-cabal-treasury-light`](../screens/03-cabal-treasury-light.png) | xxLarge | ok | — | — |
| [`03-cabal-treasury-light`](../screens/03-cabal-treasury-light.png) | xxxLarge | ok | — | — |
| [`03-cabal-treasury-light`](../screens/03-cabal-treasury-light.png) | AX1 | ok | — | — |
| [`03-cabal-treasury-light`](../screens/03-cabal-treasury-light.png) | AX5 | ok | — | — |
| [`03-cabal-treasury-scrolled`](../screens/03-cabal-treasury-scrolled.png) | large | ok | — | — |
| [`03-cabal-treasury-scrolled`](../screens/03-cabal-treasury-scrolled.png) | xxLarge | ok | — | — |
| [`03-cabal-treasury-scrolled`](../screens/03-cabal-treasury-scrolled.png) | xxxLarge | defect | 1 no truncation: 'Treasury hist...' button label truncated | #7119 |
| [`03-cabal-treasury-scrolled`](../screens/03-cabal-treasury-scrolled.png) | AX1 | ok | — | — |
| [`03-cabal-treasury-scrolled`](../screens/03-cabal-treasury-scrolled.png) | AX5 | ok | — | — |
| [`03-cabal-rules`](../screens/03-cabal-rules.png) | large | ok | — | — |
| [`03-cabal-rules`](../screens/03-cabal-rules.png) | xxLarge | ok | — | — |
| [`03-cabal-rules`](../screens/03-cabal-rules.png) | xxxLarge | ok | — | — |
| [`03-cabal-rules`](../screens/03-cabal-rules.png) | AX1 | ok | — | — |
| [`03-cabal-rules`](../screens/03-cabal-rules.png) | AX5 | ok | — | — |
| [`03-cabal-rules-scrolled`](../screens/03-cabal-rules-scrolled.png) | large | ok | — | — |
| [`03-cabal-rules-scrolled`](../screens/03-cabal-rules-scrolled.png) | xxLarge | ok | — | — |
| [`03-cabal-rules-scrolled`](../screens/03-cabal-rules-scrolled.png) | xxxLarge | ok | — | — |
| [`03-cabal-rules-scrolled`](../screens/03-cabal-rules-scrolled.png) | AX1 | ok | — | — |
| [`03-cabal-rules-scrolled`](../screens/03-cabal-rules-scrolled.png) | AX5 | ok | — | — |
| [`03-cabal-rules-sides`](../screens/03-cabal-rules-sides.png) | large | defect | 1 no truncation: 'After a vote closes - on since...' ends in '...' | #7119 |
| [`03-cabal-rules-sides`](../screens/03-cabal-rules-sides.png) | xxLarge | defect | 1 no truncation: 'After a vote closes - on...' ends in '...' | #7119 |
| [`03-cabal-rules-sides`](../screens/03-cabal-rules-sides.png) | xxxLarge | defect | 1 no truncation: 'After a vote closes -...' ends in '...' | #7119 |
| [`03-cabal-rules-sides`](../screens/03-cabal-rules-sides.png) | AX1 | ok | — | — |
| [`03-cabal-rules-sides`](../screens/03-cabal-rules-sides.png) | AX5 | defect | 1 no truncation: toggle row subtitle 'closes - on...' ends in '...' | #7119 |
| [`03-cabal-members`](../screens/03-cabal-members.png) | large | ok | — | — |
| [`03-cabal-members`](../screens/03-cabal-members.png) | xxLarge | ok | — | — |
| [`03-cabal-members`](../screens/03-cabal-members.png) | xxxLarge | ok | — | — |
| [`03-cabal-members`](../screens/03-cabal-members.png) | AX1 | ok | — | — |
| [`03-cabal-members`](../screens/03-cabal-members.png) | AX5 | ok | — | — |
| [`03-cabal-members-scrolled`](../screens/03-cabal-members-scrolled.png) | large | ok | — | — |
| [`03-cabal-members-scrolled`](../screens/03-cabal-members-scrolled.png) | xxLarge | defect | 1 no truncation: 'Admin - 7 of 9 right - voted 11...' record ends in '...' on every member row | #7119 |
| [`03-cabal-members-scrolled`](../screens/03-cabal-members-scrolled.png) | xxxLarge | defect | 1 no truncation: 'Admin - 7 of 9 right - voted...' and 'Voter - 10 of 11 right - voted...' end in '...' | #7119 |
| [`03-cabal-members-scrolled`](../screens/03-cabal-members-scrolled.png) | AX1 | defect | 2 rows stack: record line sits above the rate (10%, 12%); spec wants record under the rate | #7119 |
| [`03-cabal-members-scrolled`](../screens/03-cabal-members-scrolled.png) | AX5 | ok | — | — |
| [`03-cabal-members-all`](../screens/03-cabal-members-all.png) | large | ok | — | — |
| [`03-cabal-members-all`](../screens/03-cabal-members-all.png) | xxLarge | defect | 1 no truncation: 'Asked to join - Jordan's lin...' and Christelle 'voted 14 of...' end in '...' | #7119 |
| [`03-cabal-members-all`](../screens/03-cabal-members-all.png) | xxxLarge | ok | — | — |
| [`03-cabal-members-all`](../screens/03-cabal-members-all.png) | AX1 | defect | 2 rows stack: record line ('7 of 9 right - voted 11 of 11') sits above the 35% rate; spec wants record under the rate | #7119 |
| [`03-cabal-members-all`](../screens/03-cabal-members-all.png) | AX5 | ok | — (the request row's 'Leo' and sub-line draw whole; the audit's clipped flag on them is allow-listed as a false positive) | #7201 |
| [`03-cabal-empty`](../screens/03-cabal-empty.png) | large | ok | — | — |
| [`03-cabal-empty`](../screens/03-cabal-empty.png) | xxLarge | ok | — | — |
| [`03-cabal-empty`](../screens/03-cabal-empty.png) | xxxLarge | ok | — | — |
| [`03-cabal-empty`](../screens/03-cabal-empty.png) | AX1 | ok | — | — |
| [`03-cabal-empty`](../screens/03-cabal-empty.png) | AX5 | ok | — | — |

## Frames 15

| Frame | Size | Verdict | Rule broken | Follow-up |
| --- | --- | --- | --- | --- |
| [`15-first-fund`](../screens/15-first-fund.png) | large | ok | — | — |
| [`15-first-fund`](../screens/15-first-fund.png) | xxLarge | ok | — | — |
| [`15-first-fund`](../screens/15-first-fund.png) | xxxLarge | ok | — | — |
| [`15-first-fund`](../screens/15-first-fund.png) | AX1 | ok | — | — |
| [`15-first-fund`](../screens/15-first-fund.png) | AX5 | ok | — | — |
| [`15-passed`](../screens/15-passed.png) | large | no shot | capture: blank page with only a back button, no content drawn (original shot the same) | #7127 |
| [`15-passed`](../screens/15-passed.png) | xxLarge | no shot | capture: blank page with only a back button, no content drawn (original shot the same) | #7127 |
| [`15-passed`](../screens/15-passed.png) | xxxLarge | no shot | capture: blank page with only a back button, no content drawn (original shot the same) | #7127 |
| [`15-passed`](../screens/15-passed.png) | AX1 | no shot | capture: blank page with only a back button, no content drawn (original shot the same) | #7127 |
| [`15-passed`](../screens/15-passed.png) | AX5 | no shot | capture: blank page with only a back button, no content drawn (original shot the same) | #7127 |
| [`15-leaderboard-moment`](../screens/15-leaderboard-moment.png) | large | ok | — | — |
| [`15-leaderboard-moment`](../screens/15-leaderboard-moment.png) | xxLarge | ok | — | — |
| [`15-leaderboard-moment`](../screens/15-leaderboard-moment.png) | xxxLarge | defect | 7 period labels: Period label 'Month' breaks mid-word as 'Mont/h' (All time wraps) in the Week/Month/All time row | #7125 |
| [`15-leaderboard-moment`](../screens/15-leaderboard-moment.png) | AX1 | defect | 1 truncation / 2 ranked rows: Pinned row 'Andr…' 'Voted…' '7 of 9…' truncated; period labels 'Wee/k' 'Mon/th' broken; pinned card covers Top three avatars | #7125 |
| [`15-leaderboard-moment`](../screens/15-leaderboard-moment.png) | AX5 | defect | 2 ranked rows stack: Ranked rows break char-by-char ('An/dre/s', '78/%', '7 of 9 righ/t'), period labels shredded, name and figure share one line | #7125 |

## Frames 19

| Frame | Size | Verdict | Rule broken | Follow-up |
| --- | --- | --- | --- | --- |
| [`19-permission-sheet`](../screens/19-permission-sheet.png) | large | ok | — | — |
| [`19-permission-sheet`](../screens/19-permission-sheet.png) | xxLarge | ok | — | — |
| [`19-permission-sheet`](../screens/19-permission-sheet.png) | xxxLarge | ok | — | — |
| [`19-permission-sheet`](../screens/19-permission-sheet.png) | AX1 | defect | 8 sheets: 'Not now' button not visible; sheet content runs to the screen bottom, 'Deliver quietly' at the home indicator edge | #7126 |
| [`19-permission-sheet`](../screens/19-permission-sheet.png) | AX5 | defect | 8 sheets: Body text 'vote.' clipped at bottom; 'Turn on notifications', 'Deliver quietly' and 'Not now' are not visible | #7126 |

`FDynamicTypeAuditUITests/test19Permission`'s clipped-text findings at xxxLarge and AX5 are fixed by the PR for #7217: the push pre-prompt sheet's title, body and sample card title and body wrap to their full height, and from `xxxLarge` the sample card stacks its "now" caption under the text. The test was not rerun in this PR; the next group-by-group run confirms these rows.

## Frames 14

| Frame | Size | Verdict | Rule broken | Follow-up |
| --- | --- | --- | --- | --- |
| [`14-hold-sheet`](../screens/14-hold-sheet.png) | large | ok | — | — |
| [`14-hold-sheet`](../screens/14-hold-sheet.png) | xxLarge | ok | — | — |
| [`14-hold-sheet`](../screens/14-hold-sheet.png) | xxxLarge | ok | — | — |
| [`14-hold-sheet`](../screens/14-hold-sheet.png) | AX1 | ok | — | — |
| [`14-hold-sheet`](../screens/14-hold-sheet.png) | AX5 | defect | 1 button label: 'All proposals' link truncated to 'All propo…' on the home screen behind the sheet | #7126 |
| [`14-wrap-sheet`](../screens/14-wrap-sheet.png) | large | ok | — | — |
| [`14-wrap-sheet`](../screens/14-wrap-sheet.png) | xxLarge | defect | 1 truncation: 'your stake that week · 3 cab…' and 'Every call, the miss and the to…' end in '…' | #7124 |
| [`14-wrap-sheet`](../screens/14-wrap-sheet.png) | xxxLarge | defect | 1 truncation: 'your stake that week · 3…' and 'Every call, the miss and the…' truncated | #7124 |
| [`14-wrap-sheet`](../screens/14-wrap-sheet.png) | AX1 | defect | 1 truncation / 4 stat row: 'your stake that wee…' and 'Every call, the miss and t…' truncated; Voted/Right calls/Sharpest stay in one row instead of one per row | #7124 |
| [`14-wrap-sheet`](../screens/14-wrap-sheet.png) | AX5 | defect | 1 truncation / 12 overlap: Stat labels 'Vot…' 'Rig…' 'Sha…' and 'Chr…' truncated; '+4.2%' overlaps 'Full Ledger', 'Best call' overlaps 'Degens · you…'; 'Share the Ledger' clipped at bottom | #7124 |
| [`14-ledger-story`](../screens/14-ledger-story.png) | large | ok | — | — |
| [`14-ledger-story`](../screens/14-ledger-story.png) | xxLarge | ok | — | — |
| [`14-ledger-story`](../screens/14-ledger-story.png) | xxxLarge | ok | — | — |
| [`14-ledger-story`](../screens/14-ledger-story.png) | AX1 | defect | 1 nothing truncated: 'Replay the we…' button label ends in an ellipsis | #7124 |
| [`14-ledger-story`](../screens/14-ledger-story.png) | AX5 | defect | 1 nothing truncated; 10 docks: 'Repla…' truncated; Return/Calls/Code chips show only icon + '…'; 'Square' and 'Sticke' clipped at edges; 'Shar/e' and 'Cop/y/ima/ge' break mid-word in the buttons | #7124 |

## Frames 05

The Feed and post page wrap instead of clipping at large text (#7203). The feed post header verb, the comment byline name and time, the quiet-week vote tally and the post object place line wrap or stack from xxxLarge, and the byline stacks name over time when it does not fit one line. The emoji picker section headers (`Features/Reactions`) and the vote status line (`Design/VoteTally.swift`) sit outside this ticket's paths and stay clipped.

| Frame | Size | Verdict | Rule broken | Follow-up |
| --- | --- | --- | --- | --- |
| [`05-feed-A`](../screens/05-feed-A.png) | large | ok | — | — |
| [`05-feed-A`](../screens/05-feed-A.png) | xxLarge | ok | — | — |
| [`05-feed-A`](../screens/05-feed-A.png) | xxxLarge | ok | — | — |
| [`05-feed-A`](../screens/05-feed-A.png) | AX1 | defect | 1 truncation: 'Christelle calle…' ('Christelle called it' callout) truncated | #7121 |
| [`05-feed-A`](../screens/05-feed-A.png) | AX5 | defect | 1 truncation: 'Mia propos…' and 'Degens pas…' verb truncated in row header | #7121 |
| [`05-feed-A-scrolled`](../screens/05-feed-A-scrolled.png) | large | ok | — | — |
| [`05-feed-A-scrolled`](../screens/05-feed-A-scrolled.png) | xxLarge | ok | — | — |
| [`05-feed-A-scrolled`](../screens/05-feed-A-scrolled.png) | xxxLarge | ok | — | — |
| [`05-feed-A-scrolled`](../screens/05-feed-A-scrolled.png) | AX1 | ok | — | — |
| [`05-feed-A-scrolled`](../screens/05-feed-A-scrolled.png) | AX5 | defect | 1 truncation: 'Degens p…' and 'Degens pas…' verb truncated in row header | #7121 |
| [`05-feed-light`](../screens/05-feed-light.png) | large | ok | — | — |
| [`05-feed-light`](../screens/05-feed-light.png) | xxLarge | ok | — | — |
| [`05-feed-light`](../screens/05-feed-light.png) | xxxLarge | ok | — | — |
| [`05-feed-light`](../screens/05-feed-light.png) | AX1 | defect | 1 truncation: 'Christelle calle…' ('Christelle called it' callout) truncated | #7121 |
| [`05-feed-light`](../screens/05-feed-light.png) | AX5 | defect | 1 truncation: 'Mia propos…' and 'Degens pas…' verb truncated in row header | #7121 |
| [`05-reactions`](../screens/05-reactions.png) | large | ok | — | — |
| [`05-reactions`](../screens/05-reactions.png) | xxLarge | ok | — | — |
| [`05-reactions`](../screens/05-reactions.png) | xxxLarge | ok | — | — |
| [`05-reactions`](../screens/05-reactions.png) | AX1 | ok | — | — |
| [`05-reactions`](../screens/05-reactions.png) | AX5 | defect | 1 truncation; 12 overlap: 'Alex propos…' verb truncated; reaction emoji row overlaps and the eyes emoji is cut | #7121 |
| [`05-emoji-picker`](../screens/05-emoji-picker.png) | large | ok | — | — |
| [`05-emoji-picker`](../screens/05-emoji-picker.png) | xxLarge | ok | — | — |
| [`05-emoji-picker`](../screens/05-emoji-picker.png) | xxxLarge | ok | — | — |
| [`05-emoji-picker`](../screens/05-emoji-picker.png) | AX1 | ok | — | — |
| [`05-emoji-picker`](../screens/05-emoji-picker.png) | AX5 | defect | 2 overlap: 'Search emoji' field overlaps 'Frequently used'; 'Smileys and peo…' truncated and emoji drawn over headings; the post header verb wraps now (#7203) | #7121 |
| [`05-post`](../screens/05-post.png) | large | ok | — | — |
| [`05-post`](../screens/05-post.png) | xxLarge | defect | 1 truncation: reply quote 'Logan Took the gain while it was…' ends in an ellipsis (not a name) | #7121 |
| [`05-post`](../screens/05-post.png) | xxxLarge | defect | 1 truncation: reply quote 'Logan Took the gain while it was…' ends in an ellipsis (not a name) | #7121 |
| [`05-post`](../screens/05-post.png) | AX1 | defect | 1 truncation: reply quote 'Logan Took the gain while it was…' ends in an ellipsis (not a name) | #7121 |
| [`05-post`](../screens/05-post.png) | AX5 | defect | 1 clipped: buy 'B' marker on chart is cut off | #7121 |
| [`05-post-scrolled`](../screens/05-post-scrolled.png) | large | ok | — | — |
| [`05-post-scrolled`](../screens/05-post-scrolled.png) | xxLarge | defect | 1 truncation: reply quote 'Logan Took the gain while it was…' ends in an ellipsis (not a name) | #7121 |
| [`05-post-scrolled`](../screens/05-post-scrolled.png) | xxxLarge | ok | — | #7203 |
| [`05-post-scrolled`](../screens/05-post-scrolled.png) | AX1 | defect | 1 truncation: reply quotes 'Logan Took the gain while it…' and 'Andres Good timing on this…' truncated | #7121 |
| [`05-post-scrolled`](../screens/05-post-scrolled.png) | AX5 | ok | — | #7203 |
| [`05-post-reply`](../screens/05-post-reply.png) | large | ok | — | — |
| [`05-post-reply`](../screens/05-post-reply.png) | xxLarge | ok | — | — |
| [`05-post-reply`](../screens/05-post-reply.png) | xxxLarge | ok | — | — |
| [`05-post-reply`](../screens/05-post-reply.png) | AX1 | defect | 1 truncation: 'AAPL $…' money in quoted reply header truncated | #7121 |
| [`05-post-reply`](../screens/05-post-reply.png) | AX5 | ok | — | #7203 |
| [`05-comment-actions`](../screens/05-comment-actions.png) | large | ok | — | — |
| [`05-comment-actions`](../screens/05-comment-actions.png) | xxLarge | ok | — | — |
| [`05-comment-actions`](../screens/05-comment-actions.png) | xxxLarge | ok | — | — |
| [`05-comment-actions`](../screens/05-comment-actions.png) | AX1 | ok | — | — |
| [`05-comment-actions`](../screens/05-comment-actions.png) | AX5 | defect | 12 overlap: reaction bar emojis overlap each other and the eyes emoji is cut; 'Why sell here?' is cut and overlapped by the Reply menu; heart chip hidden | #7121 |
| [`05-post-public`](../screens/05-post-public.png) | large | defect | 4 stat row; 1 truncation: Closes cell 'Fri 2:08…' ends in an ellipsis | #7121 |
| [`05-post-public`](../screens/05-post-public.png) | xxLarge | ok | — | — |
| [`05-post-public`](../screens/05-post-public.png) | xxxLarge | ok | — | — |
| [`05-post-public`](../screens/05-post-public.png) | AX1 | ok | — | — |
| [`05-post-public`](../screens/05-post-public.png) | AX5 | ok | — | — |
| [`05-feed-empty`](../screens/05-feed-empty.png) | large | ok | — | — |
| [`05-feed-empty`](../screens/05-feed-empty.png) | xxLarge | ok | — | — |
| [`05-feed-empty`](../screens/05-feed-empty.png) | xxxLarge | ok | — | — |
| [`05-feed-empty`](../screens/05-feed-empty.png) | AX1 | ok | — | — |
| [`05-feed-empty`](../screens/05-feed-empty.png) | AX5 | defect | 1 truncation: '3 of 7 voted · ends in 23h' (`VoteStatusLine` in `Design/VoteTally.swift`) clipped; the '3 yes, 0 no' tally wraps now (#7203) | #7203 |

## Frames 06

| Frame | Size | Verdict | Rule broken | Follow-up |
| --- | --- | --- | --- | --- |
| [`06-markets-A`](../screens/06-markets-A.png) | large | ok | — | — |
| [`06-markets-A`](../screens/06-markets-A.png) | xxLarge | ok | — | — |
| [`06-markets-A`](../screens/06-markets-A.png) | xxxLarge | defect | 1 truncation: 'Secondary estimate · Oct…' date cut on Pre-IPO rows; search placeholder clipped, #7205 shortens it to "Search" at xxxLarge and above | #7122 |
| [`06-markets-A`](../screens/06-markets-A.png) | AX1 | defect | 1 truncation; 2 stack: 'Secondary estimate…' truncated; rows not stacked at AX1 | #7122 |
| [`06-markets-A`](../screens/06-markets-A.png) | AX5 | defect | 1 truncation; 2 stack: 'Vote o…', '24 sh…', '2.2 sh…' truncated; rows not stacked; search placeholder clipped, #7205 shortens it to "Search" at xxxLarge and above | #7122 |
| [`06-markets-A-scrolled`](../screens/06-markets-A-scrolled.png) | large | ok | — | — |
| [`06-markets-A-scrolled`](../screens/06-markets-A-scrolled.png) | xxLarge | defect | 1 truncation: 'Secondary estimate · O…' subtitle truncated on Anthropic and Databricks rows | #7122 |
| [`06-markets-A-scrolled`](../screens/06-markets-A-scrolled.png) | xxxLarge | defect | 1 truncation: 'Secondary estimate · Oct…' date cut on SpaceX, OpenAI, Anthropic, Databricks rows; search placeholder clipped, #7205 shortens it to "Search" at xxxLarge and above | #7122 |
| [`06-markets-A-scrolled`](../screens/06-markets-A-scrolled.png) | AX1 | defect | 1 truncation; 2 stack: subtitles 'Secondary estimate…' truncated; rows not stacked, figure beside name | #7122 |
| [`06-markets-A-scrolled`](../screens/06-markets-A-scrolled.png) | AX5 | defect | 1 truncation; 2 stack: 'Anthr…', 'Datab…', 'Secon…', 'Resea…' truncated; Moving today cards show 'A…', 'N…', 'T…', '+7.9…'; rows not stacked; search placeholder clipped, #7205 shortens it to "Search" at xxxLarge and above | #7122 |
| [`06-markets-search`](../screens/06-markets-search.png) | large | ok | — | — |
| [`06-markets-search`](../screens/06-markets-search.png) | xxLarge | defect | 1 truncation: 'Advanced Micro Devi…' stock name truncated | #7122 |
| [`06-markets-search`](../screens/06-markets-search.png) | xxxLarge | defect | 1 truncation: 'Advanced Micro D…' stock name truncated; search placeholder clipped, #7205 shortens it to "Search" at xxxLarge and above | #7122 |
| [`06-markets-search`](../screens/06-markets-search.png) | AX1 | defect | 1 truncation; 2 stack: 'Advanced Micro Dev…' truncated; rows not stacked | #7122 |
| [`06-markets-search`](../screens/06-markets-search.png) | AX5 | defect | 1 truncation; 2 stack: 'Micro…', 'Advan…', 'Super…', 'Microc…' truncated; rows not stacked; search placeholder clipped, #7205 shortens it to "Search" at xxxLarge and above | #7122 |
| [`06-markets-held`](../screens/06-markets-held.png) | large | ok | — | — |
| [`06-markets-held`](../screens/06-markets-held.png) | xxLarge | ok | — | — |
| [`06-markets-held`](../screens/06-markets-held.png) | xxxLarge | defect | 1 truncation: 'AC Capital · 12 sh · Pre-I…' SpaceX subtitle truncated; search placeholder clipped, #7205 shortens it to "Search" at xxxLarge and above | #7122 |
| [`06-markets-held`](../screens/06-markets-held.png) | AX1 | defect | 1 truncation; 2 stack: SpaceX subtitle 'AC Capital · 12 sh ·…' truncated; rows not stacked | #7122 |
| [`06-markets-held`](../screens/06-markets-held.png) | AX5 | defect | 1 money broken; 2 stack: '$8,360.' / '00' and '$4,320.' / '00' money wraps mid-figure; rows not stacked; search placeholder clipped, #7205 shortens it to "Search" at xxxLarge and above | #7122 |

## Frames 07

| Frame | Size | Verdict | Rule broken | Follow-up |
| --- | --- | --- | --- | --- |
| [`07-stock-dark`](../screens/07-stock-dark.png) | large | ok | — | — |
| [`07-stock-dark`](../screens/07-stock-dark.png) | xxLarge | defect | 1 truncation: proposal row '5 of 8 voted · 6 replies · end…' truncated | #7122 |
| [`07-stock-dark`](../screens/07-stock-dark.png) | xxxLarge | defect | 1 truncation: proposal row '5 of 8 voted · 6 replies · e…' truncated | #7122 |
| [`07-stock-dark`](../screens/07-stock-dark.png) | AX1 | ok | — | — |
| [`07-stock-dark`](../screens/07-stock-dark.png) | AX5 | defect | 7 period labels: period labels run together as '1H1D1W1M3M6MAll' with no spacing and 'All' cut at right edge | #7122 |
| [`07-stock-light`](../screens/07-stock-light.png) | large | ok | — | — |
| [`07-stock-light`](../screens/07-stock-light.png) | xxLarge | defect | 1 truncation: proposal row '5 of 8 voted · 6 replies · end…' truncated | #7122 |
| [`07-stock-light`](../screens/07-stock-light.png) | xxxLarge | defect | 1 truncation: proposal row '5 of 8 voted · 6 replies · e…' truncated | #7122 |
| [`07-stock-light`](../screens/07-stock-light.png) | AX1 | ok | — | — |
| [`07-stock-light`](../screens/07-stock-light.png) | AX5 | defect | 7 period labels: period labels run together as '1H1D1W1M3M6MAll' with no spacing and 'All' cut at right edge | #7122 |
| [`07-stock-preipo`](../screens/07-stock-preipo.png) | large | ok | — | — |
| [`07-stock-preipo`](../screens/07-stock-preipo.png) | xxLarge | ok | — | — |
| [`07-stock-preipo`](../screens/07-stock-preipo.png) | xxxLarge | ok | — | — |
| [`07-stock-preipo`](../screens/07-stock-preipo.png) | AX1 | defect | 2 stack: 'Secondary estimate · Oct 20' wraps beside '$185.00' instead of stacking | #7122 |
| [`07-stock-preipo`](../screens/07-stock-preipo.png) | AX5 | defect | 2 stack: 'Last round · Jun '26' wraps to three lines beside '$165.00' instead of stacking | #7122 |
| [`07-stock-preipo-scrolled`](../screens/07-stock-preipo-scrolled.png) | large | ok | — | — |
| [`07-stock-preipo-scrolled`](../screens/07-stock-preipo-scrolled.png) | xxLarge | defect | 1 truncation: '2 of 6 voted · 3 replies · ends…' truncated | #7122 |
| [`07-stock-preipo-scrolled`](../screens/07-stock-preipo-scrolled.png) | xxxLarge | defect | 1 truncation: '2 of 6 voted · 3 replies · e…' truncated | #7122 |
| [`07-stock-preipo-scrolled`](../screens/07-stock-preipo-scrolled.png) | AX1 | ok | — | — |
| [`07-stock-preipo-scrolled`](../screens/07-stock-preipo-scrolled.png) | AX5 | ok | — | — |

## Frames 08

| Frame | Size | Verdict | Rule broken | Follow-up |
| --- | --- | --- | --- | --- |
| [`08-review-sheet`](../screens/08-review-sheet.png) | large | ok | — | — |
| [`08-review-sheet`](../screens/08-review-sheet.png) | xxLarge | defect | 8 sheet: Shares/Price/Of treasury/To pass grid: '2%' value under 'Of treasury' and 'To pass 3 of 3' cut by pinned Send to vote button | #7120 |
| [`08-review-sheet`](../screens/08-review-sheet.png) | xxxLarge | ok | — (the 'Propose buy' text behind the sheet is allow-listed; the asset buttons stack from xxxLarge) | #7206 |
| [`08-review-sheet`](../screens/08-review-sheet.png) | AX1 | defect | 8 sheet: Stat grid cut: Shares value '≈ 1.82' and Price row hidden behind Send to vote; Of treasury and To pass not visible | #7120 |
| [`08-review-sheet`](../screens/08-review-sheet.png) | AX5 | ok | — (the 'Propose buy' text behind the sheet is allow-listed; the asset buttons stack from xxxLarge) | #7206 |
| [`08-review-sheet-preipo`](../screens/08-review-sheet-preipo.png) | large | ok | — | — |
| [`08-review-sheet-preipo`](../screens/08-review-sheet-preipo.png) | xxLarge | ok | — | — |
| [`08-review-sheet-preipo`](../screens/08-review-sheet-preipo.png) | xxxLarge | ok | — (the 'Propose buy' text behind the sheet is allow-listed; the asset buttons stack from xxxLarge) | #7206 |
| [`08-review-sheet-preipo`](../screens/08-review-sheet-preipo.png) | AX1 | defect | 8 sheet: Last 'Secondary estimate · Oct 20' row wraps and its second line is cut off behind the pinned Send to vote button | #7120 |
| [`08-review-sheet-preipo`](../screens/08-review-sheet-preipo.png) | AX5 | ok | — (the 'Propose buy' text behind the sheet is allow-listed; the asset buttons stack from xxxLarge) | #7206 |

`FDynamicTypeAuditUITests/test08Review` reports only the allow-listed 'Propose buy' backdrop text at xxxLarge and AX5 (#7206). From xxxLarge the asset detail's propose buttons stack full width.

## Frames 09

#7207 makes the keypad source pill stack from xxxLarge, lets the amount line, slot, limit line and settled button wrap to full height, so the 31 `FDynamicTypeAuditUITests/test09Money` clipped-text findings at xxxLarge and AX5 are fixed in code; the audit rerun is pending.

| Frame | Size | Verdict | Rule broken | Follow-up |
| --- | --- | --- | --- | --- |
| [`09-fund-cabal-empty`](../screens/09-fund-cabal-empty.png) | large | ok | — | — |
| [`09-fund-cabal-empty`](../screens/09-fund-cabal-empty.png) | xxLarge | ok | — | — |
| [`09-fund-cabal-empty`](../screens/09-fund-cabal-empty.png) | xxxLarge | ok | — | — |
| [`09-fund-cabal-empty`](../screens/09-fund-cabal-empty.png) | AX1 | defect | 1 truncation: Cabal picker money truncated 'Cash $50…' | #7123 |
| [`09-fund-cabal-empty`](../screens/09-fund-cabal-empty.png) | AX5 | defect | 1 truncation: Cabal picker shows 'AC…' and 'Cas…'; money label truncated | #7123 |
| [`09-fund-cabal`](../screens/09-fund-cabal.png) | large | ok | — | — |
| [`09-fund-cabal`](../screens/09-fund-cabal.png) | xxLarge | ok | — | — |
| [`09-fund-cabal`](../screens/09-fund-cabal.png) | xxxLarge | ok | — | — |
| [`09-fund-cabal`](../screens/09-fund-cabal.png) | AX1 | defect | 1 truncation: Cabal picker money truncated 'Cash $50…' | #7123 |
| [`09-fund-cabal`](../screens/09-fund-cabal.png) | AX5 | defect | 1 truncation: Cabal picker shows 'AC… Cas…' (money label truncated); '→ 36%' line sits flush against the chips | #7123 |
| [`09-fund-cabal-invalid`](../screens/09-fund-cabal-invalid.png) | large | ok | — | — |
| [`09-fund-cabal-invalid`](../screens/09-fund-cabal-invalid.png) | xxLarge | ok | — | — |
| [`09-fund-cabal-invalid`](../screens/09-fund-cabal-invalid.png) | xxxLarge | ok | — | — |
| [`09-fund-cabal-invalid`](../screens/09-fund-cabal-invalid.png) | AX1 | defect | 1 truncation: Cabal picker money truncated 'Cash $50…' | #7123 |
| [`09-fund-cabal-invalid`](../screens/09-fund-cabal-invalid.png) | AX5 | defect | 1 truncation: '$500.00 available · $150….' limit line ends in '…' and picker shows 'AC… Cas…' | #7123 |
| [`09-fund-cabal-settled`](../screens/09-fund-cabal-settled.png) | large | ok | — | — |
| [`09-fund-cabal-settled`](../screens/09-fund-cabal-settled.png) | xxLarge | ok | — | — |
| [`09-fund-cabal-settled`](../screens/09-fund-cabal-settled.png) | xxxLarge | ok | — | — |
| [`09-fund-cabal-settled`](../screens/09-fund-cabal-settled.png) | AX1 | defect | 1 truncation: Cabal picker money truncated 'Cash $25…' | #7123 |
| [`09-fund-cabal-settled`](../screens/09-fund-cabal-settled.png) | AX5 | defect | 12 overlap: Picker 'AC… Cas…' truncated; 'your share is now 36%' line is cut off and overlapped by faded +$1/+$10 chips | #7123 |
| [`09-withdraw-empty`](../screens/09-withdraw-empty.png) | large | ok | — | — |
| [`09-withdraw-empty`](../screens/09-withdraw-empty.png) | xxLarge | ok | — | — |
| [`09-withdraw-empty`](../screens/09-withdraw-empty.png) | xxxLarge | ok | — | — |
| [`09-withdraw-empty`](../screens/09-withdraw-empty.png) | AX1 | ok | — | — |
| [`09-withdraw-empty`](../screens/09-withdraw-empty.png) | AX5 | defect | 5 keypad: AX5: Picker 'Cash…' truncated and overlaps status bar clock; address wraps to 6 huge mono lines and pushes the $ amount and line under it off screen; chain chips cut ('Arb') and touch +$1 chips; 'Enter an amount' button cut at bottom | #7123 |
| [`09-withdraw`](../screens/09-withdraw.png) | large | ok | — | — |
| [`09-withdraw`](../screens/09-withdraw.png) | xxLarge | ok | — | — |
| [`09-withdraw`](../screens/09-withdraw.png) | xxxLarge | ok | — | — |
| [`09-withdraw`](../screens/09-withdraw.png) | AX1 | ok | — | — |
| [`09-withdraw`](../screens/09-withdraw.png) | AX5 | defect | 5 keypad: Picker 'Cash…' truncated; address six huge lines pushes the $120 amount and '≈ 120.00 USDC' line off screen; Withdraw $120 button cut at bottom | #7123 |
| [`09-withdraw-invalid`](../screens/09-withdraw-invalid.png) | large | ok | — | — |
| [`09-withdraw-invalid`](../screens/09-withdraw-invalid.png) | xxLarge | ok | — | — |
| [`09-withdraw-invalid`](../screens/09-withdraw-invalid.png) | xxxLarge | ok | — | — |
| [`09-withdraw-invalid`](../screens/09-withdraw-invalid.png) | AX1 | ok | — | — |
| [`09-withdraw-invalid`](../screens/09-withdraw-invalid.png) | AX5 | defect | 5 keypad: Picker 'Cash…' truncated; address takes six huge lines so amount '$650' is off screen; limit line 'You can send $4…' truncated; 'Up to $499.50' cut at bottom | #7123 |
| [`09-withdraw-settled`](../screens/09-withdraw-settled.png) | large | ok | — | — |
| [`09-withdraw-settled`](../screens/09-withdraw-settled.png) | xxLarge | ok | — | — |
| [`09-withdraw-settled`](../screens/09-withdraw-settled.png) | xxxLarge | ok | — | — |
| [`09-withdraw-settled`](../screens/09-withdraw-settled.png) | AX1 | ok | — | — |
| [`09-withdraw-settled`](../screens/09-withdraw-settled.png) | AX5 | defect | 5 keypad: Picker 'Cash…' truncated; address six huge lines pushes the $120 amount and status line off screen; button '$120 to 9xQe….…' truncated and cut at bottom | #7123 |

## Frames 10

| Frame | Size | Verdict | Rule broken | Follow-up |
| --- | --- | --- | --- | --- |
| [`10-profile-own`](../screens/10-profile-own.png) | large | ok | — | — |
| [`10-profile-own`](../screens/10-profile-own.png) | xxLarge | ok | — | — |
| [`10-profile-own`](../screens/10-profile-own.png) | xxxLarge | ok | — | — |
| [`10-profile-own`](../screens/10-profile-own.png) | AX1 | defect | 2 rows stack: Cash · USDC row keeps $248.50 trailing instead of stacking under the label | #7118 |
| [`10-profile-own`](../screens/10-profile-own.png) | AX5 | defect | 1 truncation: Handle line '@andres · joined S…' truncated | #7118 |
| [`10-profile-own-scrolled`](../screens/10-profile-own-scrolled.png) | large | ok | — | — |
| [`10-profile-own-scrolled`](../screens/10-profile-own-scrolled.png) | xxLarge | ok | — | — |
| [`10-profile-own-scrolled`](../screens/10-profile-own-scrolled.png) | xxxLarge | ok | — | — |
| [`10-profile-own-scrolled`](../screens/10-profile-own-scrolled.png) | AX1 | ok | — | — |
| [`10-profile-own-scrolled`](../screens/10-profile-own-scrolled.png) | AX5 | defect | 12 overlap: Cash · USDC row: card icon overlaps '3.5', '3.5 % APY' breaks into three lines, $248.50 stays trailing instead of stacked; pending $50.00 and Cabals rows pushed down | #7118 |
| [`10-profile-other`](../screens/10-profile-other.png) | large | ok | — | — |
| [`10-profile-other`](../screens/10-profile-other.png) | xxLarge | defect | 1 truncation: Counts line ends '2 in comm…' | #7118 |
| [`10-profile-other`](../screens/10-profile-other.png) | xxxLarge | defect | 1 truncation: Counts line truncated '12 follow…', '2 in com…' | #7118 |
| [`10-profile-other`](../screens/10-profile-other.png) | AX1 | defect | 2 rows stack: Counts line truncated '12 follo… · 8 follo… · 2 in com…'; activity rows keep time (20m, 5h) trailing instead of stacking | #7118 |
| [`10-profile-other`](../screens/10-profile-other.png) | AX5 | defect | 4 stat rows: Counts line '…f… · 8 f… · 3 c… · 2 in…' truncated; handle '#8 this mon…' truncated; P&L and Right calls remain two columns with wrapped labels ('P&L / · 1M', 'Right / calls · / 1M') | #7118 |
| [`10-profile-other-cabals`](../screens/10-profile-other-cabals.png) | large | ok | — | — |
| [`10-profile-other-cabals`](../screens/10-profile-other-cabals.png) | xxLarge | defect | 1 truncation: Counts line '48 follow… · 21 following' truncates 'followers' | #7118 |
| [`10-profile-other-cabals`](../screens/10-profile-other-cabals.png) | xxxLarge | defect | 1 truncation: Counts line truncated: '48 follo…', '21 follow…' | #7118 |
| [`10-profile-other-cabals`](../screens/10-profile-other-cabals.png) | AX1 | defect | 1 truncation: Counts line truncated '48 foll… · 21 follo… · 2 in com…'; cabal rows keep 20%/15%/8% trailing instead of stacking under name | #7118 |
| [`10-profile-other-cabals`](../screens/10-profile-other-cabals.png) | AX5 | defect | 4 stat rows: Counts line '…fo… · …f… · 3 c… · 2 in…' mostly cut; handle '#1 this…' truncated; P&L and Right calls stay two columns with labels wrapping ('P&L / · 1M', 'Right / calls ·') | #7118 |

`FDynamicTypeAuditUITests/test10Profile` passes at xxxLarge and AX5 (#7209). The other-member counts line now picks a horizontal or stacked layout with `ViewThatFits`, as the own-profile counts do, so each label gets the width and height it needs.

## Frames 11

#7210 lets the share dock chip labels wrap and stacks the chips from xxxLarge at full width, and lets the invite sheet heading and failed-state title and message wrap to full height with the heading icons stacked below the title from xxxLarge, so the 17 `FDynamicTypeAuditUITests/test11Share` clipped-text findings at xxxLarge and AX5 are fixed in code; the audit rerun is pending.

| Frame | Size | Verdict | Rule broken | Follow-up |
| --- | --- | --- | --- | --- |
| [`11-invite-sheet`](../screens/11-invite-sheet.png) | large | ok | — | — |
| [`11-invite-sheet`](../screens/11-invite-sheet.png) | xxLarge | ok | — | — |
| [`11-invite-sheet`](../screens/11-invite-sheet.png) | xxxLarge | ok | — | — |
| [`11-invite-sheet`](../screens/11-invite-sheet.png) | AX1 | ok | — | — |
| [`11-invite-sheet`](../screens/11-invite-sheet.png) | AX5 | defect | 10 dock: Title 'Couldn't load…' cut off, message and Try again hidden; Share and Copy link labels break ('Shar/e', 'Cop/y link') inside buttons; header icons crowd title | #7126 |
| [`11-share-A`](../screens/11-share-A.png) | large | ok | — | — |
| [`11-share-A`](../screens/11-share-A.png) | xxLarge | ok | — | — |
| [`11-share-A`](../screens/11-share-A.png) | xxxLarge | defect | 1 truncation: 'Replay the positi…' truncated | #7124 |
| [`11-share-A`](../screens/11-share-A.png) | AX1 | defect | 1 truncation: 'Replay the po…' and chip 'Buy /…' truncated | #7124 |
| [`11-share-A`](../screens/11-share-A.png) | AX5 | defect | 1 truncation: AX5: Replay label cut 'Repla…'; P&L/Buy-Sell/Code chips show only icon + '…'; tab row cut ('Card 4:5 Square St'); Share/Copy image labels break letter by letter ('Shar/e', 'Cop/y/ima/ge') | #7124 |
| [`11-share-B`](../screens/11-share-B.png) | large | ok | — | — |
| [`11-share-B`](../screens/11-share-B.png) | xxLarge | ok | — | — |
| [`11-share-B`](../screens/11-share-B.png) | xxxLarge | ok | — | — |
| [`11-share-B`](../screens/11-share-B.png) | AX1 | defect | 1 truncation: Chip label 'Buy /…' truncated | #7124 |
| [`11-share-B`](../screens/11-share-B.png) | AX5 | defect | 1 truncation: AX5: Replay label cut 'Repla…'; P&L/Buy-Sell/Code chips show only icon + '…'; tab row cut ('Card 4:5 Square St'); Share/Copy image labels break letter by letter ('Shar/e', 'Cop/y/ima/ge') | #7124 |
| [`11-share-C`](../screens/11-share-C.png) | large | ok | — | — |
| [`11-share-C`](../screens/11-share-C.png) | xxLarge | ok | — | — |
| [`11-share-C`](../screens/11-share-C.png) | xxxLarge | ok | — | — |
| [`11-share-C`](../screens/11-share-C.png) | AX1 | defect | 1 truncation: 'Replay your s…' and chip 'Buy /…' truncated | #7124 |
| [`11-share-C`](../screens/11-share-C.png) | AX5 | defect | 1 truncation: AX5: Replay label cut 'Repla…'; P&L/Buy-Sell/Code chips show only icon + '…'; tab row cut ('Card 4:5 Square St'); Share/Copy image labels break letter by letter ('Shar/e', 'Cop/y/ima/ge') | #7124 |
| [`11-share-card-vote`](../screens/11-share-card-vote.png) | large | ok | — | — |
| [`11-share-card-vote`](../screens/11-share-card-vote.png) | xxLarge | ok | — | — |
| [`11-share-card-vote`](../screens/11-share-card-vote.png) | xxxLarge | ok | — | — |
| [`11-share-card-vote`](../screens/11-share-card-vote.png) | AX1 | defect | 1 truncation: 'Replay the pri…' and chip 'Buy /…' truncated | #7124 |
| [`11-share-card-vote`](../screens/11-share-card-vote.png) | AX5 | defect | 1 truncation: AX5: Replay label cut 'Repla…'; P&L/Buy-Sell/Code chips show only icon + '…'; tab row cut ('Card 4:5 Square St'); Share/Copy image labels break letter by letter ('Shar/e', 'Cop/y/ima/ge') | #7124 |
| [`11-share-sticker`](../screens/11-share-sticker.png) | large | ok | — | — |
| [`11-share-sticker`](../screens/11-share-sticker.png) | xxLarge | ok | — | — |
| [`11-share-sticker`](../screens/11-share-sticker.png) | xxxLarge | ok | — | — |
| [`11-share-sticker`](../screens/11-share-sticker.png) | AX1 | defect | 1 truncation: Chip 'Buy /…' truncated | #7124 |
| [`11-share-sticker`](../screens/11-share-sticker.png) | AX5 | defect | 1 truncation: AX5: chips show only icon + '…' (P&L, Buy / Sell, Code truncated); tab row cut ('quare Story'); Share/Copy image labels break letter by letter ('Shar/e', 'Cop/y/ima/ge') | #7124 |
| [`11-share-sticker-on`](../screens/11-share-sticker-on.png) | large | ok | — | — |
| [`11-share-sticker-on`](../screens/11-share-sticker-on.png) | xxLarge | ok | — | — |
| [`11-share-sticker-on`](../screens/11-share-sticker-on.png) | xxxLarge | ok | — | — |
| [`11-share-sticker-on`](../screens/11-share-sticker-on.png) | AX1 | defect | 1 truncation: Chip 'Buy /…' truncated | #7124 |
| [`11-share-sticker-on`](../screens/11-share-sticker-on.png) | AX5 | defect | 1 truncation: AX5: chips show only icon + '…' (P&L, Buy / Sell, Code truncated); tab row cut ('quare Story'); Share/Copy image labels break letter by letter ('Shar/e', 'Cop/y/ima/ge') | #7124 |

## Frames 18

| Frame | Size | Verdict | Rule broken | Follow-up |
| --- | --- | --- | --- | --- |
| [`18-invite-landing`](../screens/18-invite-landing.png) | large | ok | — | — |
| [`18-invite-landing`](../screens/18-invite-landing.png) | xxLarge | ok | — | — |
| [`18-invite-landing`](../screens/18-invite-landing.png) | xxxLarge | ok | — | — |
| [`18-invite-landing`](../screens/18-invite-landing.png) | AX1 | defect | 4 stat rows: Treasury / 1M / Turnout stay in one row of three at AX1 instead of one per row (figures still fit) | #7126 |
| [`18-invite-landing`](../screens/18-invite-landing.png) | AX5 | defect | 4 stat rows / 1 truncation: Treasury/1M/Turnout row cramped: 'Trea-/sury', 'Turn/out' hyphenated or broken, '−4.1' figure cut | #7126 |

## Frames 13

| Frame | Size | Verdict | Rule broken | Follow-up |
| --- | --- | --- | --- | --- |
| [`13-leaderboard-C-people`](../screens/13-leaderboard-C-people.png) | large | ok | — | — |
| [`13-leaderboard-C-people`](../screens/13-leaderboard-C-people.png) | xxLarge | ok | — | — |
| [`13-leaderboard-C-people`](../screens/13-leaderboard-C-people.png) | xxxLarge | defect | 7 period labels: Period label 'Month' breaks mid-word as 'Mont/h' (All time wraps) in the Week/Month/All time row | #7125 |
| [`13-leaderboard-C-people`](../screens/13-leaderboard-C-people.png) | AX1 | defect | 1 truncation / 2 ranked rows: Row shows 'Andr…', 'Voted…' and '7 of 9…' truncated; rate beside name not stacked; period labels 'Wee/k' 'Mon/th' broken; pinned card covers Top three avatars | #7125 |
| [`13-leaderboard-C-people`](../screens/13-leaderboard-C-people.png) | AX5 | defect | 2 ranked rows stack: Ranked rows break char-by-char ('An/dre/s', '78/%', '7 of 9 righ/t'), period labels shredded, name and figure share one line | #7125 |
| [`13-leaderboard-C`](../screens/13-leaderboard-C.png) | large | ok | — | — |
| [`13-leaderboard-C`](../screens/13-leaderboard-C.png) | xxLarge | ok | — | — |
| [`13-leaderboard-C`](../screens/13-leaderboard-C.png) | xxxLarge | defect | 7 period labels: Period label 'Month' breaks mid-word as 'Mont/h' (All time wraps) in the Week/Month/All time row | #7125 |
| [`13-leaderboard-C`](../screens/13-leaderboard-C.png) | AX1 | defect | 1 truncation / 2 ranked rows: Pinned row 'Andr…' 'Voted…' '7 of 9…' truncated; period labels 'Wee/k' 'Mon/th' broken; pinned card covers Top three avatars | #7125 |
| [`13-leaderboard-C`](../screens/13-leaderboard-C.png) | AX5 | defect | 2 ranked rows stack: Ranked rows break char-by-char ('An/dre/s', '78/%', '7 of 9 righ/t'), period labels shredded, name and figure share one line | #7125 |
| [`13-leaderboard-C-scrolled`](../screens/13-leaderboard-C-scrolled.png) | large | ok | — | — |
| [`13-leaderboard-C-scrolled`](../screens/13-leaderboard-C-scrolled.png) | xxLarge | ok | — | — |
| [`13-leaderboard-C-scrolled`](../screens/13-leaderboard-C-scrolled.png) | xxxLarge | ok | — | — |
| [`13-leaderboard-C-scrolled`](../screens/13-leaderboard-C-scrolled.png) | AX1 | ok | — | — |
| [`13-leaderboard-C-scrolled`](../screens/13-leaderboard-C-scrolled.png) | AX5 | defect | 7 tabs / 2 ranked rows: Underline tabs cut ('People Cabals Fr') and overlapped; row text broken char-by-char ('An/dre/s', '78/%', 'righ/t') | #7125 |
| [`13-leaderboard-C-cabals`](../screens/13-leaderboard-C-cabals.png) | large | ok | — | — |
| [`13-leaderboard-C-cabals`](../screens/13-leaderboard-C-cabals.png) | xxLarge | ok | — | — |
| [`13-leaderboard-C-cabals`](../screens/13-leaderboard-C-cabals.png) | xxxLarge | defect | 7 period labels: Period label 'Month' breaks mid-word as 'Mont/h' (All time wraps) in the Week/Month/All time row | #7125 |
| [`13-leaderboard-C-cabals`](../screens/13-leaderboard-C-cabals.png) | AX1 | defect | 2 ranked rows stack: Ranked/your row 'Tech Titans Turnout 92%' keeps rank, logo, name and '89% 8 of 9 right' side by side with name wrapping 'Tech/Titans'; stat card name truncated 'Tech Tita…'; period labels 'Wee/k' 'Mon/th' broken | #7125 |
| [`13-leaderboard-C-cabals`](../screens/13-leaderboard-C-cabals.png) | AX5 | defect | 2 ranked rows stack: Ranked rows break char-by-char ('An/dre/s', '78/%', '7 of 9 righ/t'), period labels shredded, name and figure share one line | #7125 |
| [`13-leaderboard-C-unranked`](../screens/13-leaderboard-C-unranked.png) | large | ok | — | — |
| [`13-leaderboard-C-unranked`](../screens/13-leaderboard-C-unranked.png) | xxLarge | ok | — | — |
| [`13-leaderboard-C-unranked`](../screens/13-leaderboard-C-unranked.png) | xxxLarge | defect | 7 period labels: Period label 'Month' breaks mid-word as 'Mont/h' (All time wraps) in the Week/Month/All time row | #7125 |
| [`13-leaderboard-C-unranked`](../screens/13-leaderboard-C-unranked.png) | AX1 | defect | 7 period labels: Period labels broken mid-word: 'Wee/k', 'Mon/th'; 'This week Oct 19 – 25' wraps | #7125 |
| [`13-leaderboard-C-unranked`](../screens/13-leaderboard-C-unranked.png) | AX5 | defect | 7 period labels / 12 overlap: Period labels shredded ('W/ee/k', 'M/o/nth', 'All/ti/m/e'), tabs cut ('People Cabals Fr') and overlapped by card | #7125 |

## Frames 16

| Frame | Size | Verdict | Rule broken | Follow-up |
| --- | --- | --- | --- | --- |
| [`16-catch-up`](../screens/16-catch-up.png) | large | no shot | capture: Home screen with 'Couldn't load your portfolio.' toast drawn instead of Catch up | #7127 |
| [`16-catch-up`](../screens/16-catch-up.png) | xxLarge | no shot | capture: Home screen with 'Couldn't load your portfolio.' toast drawn instead of Catch up | #7127 |
| [`16-catch-up`](../screens/16-catch-up.png) | xxxLarge | ok | — | — |
| [`16-catch-up`](../screens/16-catch-up.png) | AX1 | no shot | capture: Home screen with 'Couldn't load your portfolio.' toast drawn instead of Catch up (original the same) | #7127 |
| [`16-catch-up`](../screens/16-catch-up.png) | AX5 | defect | 2 rows stack at AX: 'Buy NVDA · $400 / AC Capital · 5 of 8 voted' squeezed into a narrow column beside the 'Vote' button (one word per line) instead of stacking over it | #7124 |
| [`16-catch-up-open`](../screens/16-catch-up-open.png) | large | no shot | capture: Home screen with 'Couldn't load your portfolio.' toast drawn instead of Catch up | #7127 |
| [`16-catch-up-open`](../screens/16-catch-up-open.png) | xxLarge | no shot | capture: Home screen with 'Couldn't load your portfolio.' toast drawn instead of Catch up | #7127 |
| [`16-catch-up-open`](../screens/16-catch-up-open.png) | xxxLarge | ok | — | — |
| [`16-catch-up-open`](../screens/16-catch-up-open.png) | AX1 | no shot | capture: Home screen with 'Couldn't load your portfolio.' toast drawn instead of Catch up (original the same) | #7127 |
| [`16-catch-up-open`](../screens/16-catch-up-open.png) | AX5 | defect | 10 docks: Clear button label breaks mid-word ('Clea/r'); judged from first capture | #7124 |
| [`16-catch-up-open-scrolled`](../screens/16-catch-up-open-scrolled.png) | large | no shot | capture: Home screen with 'Couldn't load your portfolio.' toast drawn instead of Catch up | #7127 |
| [`16-catch-up-open-scrolled`](../screens/16-catch-up-open-scrolled.png) | xxLarge | no shot | capture: Home screen with 'Couldn't load your portfolio.' toast drawn instead of Catch up | #7127 |
| [`16-catch-up-open-scrolled`](../screens/16-catch-up-open-scrolled.png) | xxxLarge | ok | — | — |
| [`16-catch-up-open-scrolled`](../screens/16-catch-up-open-scrolled.png) | AX1 | no shot | capture: Home screen with 'Couldn't load your portfolio.' toast drawn instead of Catch up (original the same) | #7127 |
| [`16-catch-up-open-scrolled`](../screens/16-catch-up-open-scrolled.png) | AX5 | defect | 10 docks: Clear button label breaks mid-word ('Clea/r'); judged from first capture | #7124 |
| [`16-catch-up-empty`](../screens/16-catch-up-empty.png) | large | no shot | capture: Home screen with 'Couldn't load your portfolio.' toast drawn instead of Catch up | #7127 |
| [`16-catch-up-empty`](../screens/16-catch-up-empty.png) | xxLarge | no shot | capture: Home screen with 'Couldn't load your portfolio.' toast drawn instead of Catch up | #7127 |
| [`16-catch-up-empty`](../screens/16-catch-up-empty.png) | xxxLarge | ok | — | — |
| [`16-catch-up-empty`](../screens/16-catch-up-empty.png) | AX1 | no shot | capture: Home screen with 'Couldn't load your portfolio.' toast drawn instead of Catch up (original the same) | #7127 |
| [`16-catch-up-empty`](../screens/16-catch-up-empty.png) | AX5 | ok | — | — |
| [`16-catch-up-empty-scrolled`](../screens/16-catch-up-empty-scrolled.png) | large | no shot | capture: Home screen with 'Couldn't load your portfolio.' toast drawn instead of Catch up | #7127 |
| [`16-catch-up-empty-scrolled`](../screens/16-catch-up-empty-scrolled.png) | xxLarge | no shot | capture: Home screen with 'Couldn't load your portfolio.' toast drawn instead of Catch up | #7127 |
| [`16-catch-up-empty-scrolled`](../screens/16-catch-up-empty-scrolled.png) | xxxLarge | ok | — | — |
| [`16-catch-up-empty-scrolled`](../screens/16-catch-up-empty-scrolled.png) | AX1 | ok | — | — |
| [`16-catch-up-empty-scrolled`](../screens/16-catch-up-empty-scrolled.png) | AX5 | ok | — | — |

## Rules that held everywhere

- The vote tally (rule 3): the bar keeps full width, Yes and No sit side by side up to xxxLarge and stack at AX sizes without overlapping, and no count is cut. `DesignLayoutTests` pins the stacking (`VoteHoldRow`).
- The Home figure and the money hero (rule 6) scale with their role and are never drawn smaller than at `large`.
- Toolbars (rule 9): no inline title pushed a 44 pt round button off screen, and the large root titles wrap.
- The system tab bar keeps iOS's own labels at every size, with nothing custom drawn over it.
- At `large` only four pairs are defects: the `Closes` stat cell ends in `…` on `04-proposal-preipo` and `05-post-public`, the `03-cabal-rules-sides` toggle subtitle ends in `…`, and a stray `posts=0` label sits under the nav title on `04-gif-sheet`. Every other defect appears only above the default size.

## Follow-ups

Each issue lists its frames, sizes and the rule each breaks, and names the owning tickets.

| Issue | Area | Pairs | Owning tickets |
| --- | --- | ---: | --- |
| #7118 | Fix Home and Profile at large text sizes: cash rows, section headers and profile counts | 21 | #6480, #6496, #6505, #6488, #6530, #6508 |
| #7119 | Fix the Cabals tab and cabal page at large text sizes: member, rules and treasury rows | 16 | #6522, #6481, #6497, #6523 |
| #7120 | Fix the proposal, vote and review sheet at large text sizes: stat cells, quotes, composer and the pinned Send to vote | 18 | #6460, #6461, #6506, #6567, #6548, #6541 |
| #7121 | Fix Feed and comments at large text sizes: post headers, reply quotes, reactions and the emoji picker | 20 | #6486, #6500, #6541, #6548, #6469, #6552 |
| #7122 | Stack Markets and stock rows at accessibility sizes and keep stock names, money and period labels whole | 24 | #6487, #6501, #6516, #6529, #6528, #6449 |
| #7123 | Keep Fund a cabal and Withdraw's amount, picker, limit line and button reachable at AX sizes | 12 | #6459, #6490, #6504, #6476 |
| #7124 | Fix the share screen, Ledger and Catch up at large text sizes: Replay, chips, the share dock and Keep / Clear | 22 | #6519, #6503, #6525, #6513 |
| #7125 | Keep Leaderboard C's period labels, tabs and ranked rows whole at large text sizes | 16 | #6484, #6499, #6514, #6526, #6449, #6461 |
| #7126 | Keep sheet buttons, the toast and the invite surfaces reachable at accessibility sizes | 9 | #6470, #6458, #6489, #6502 |
| #7127 | Draw 15-passed, Catch up and the holding vote card on every sample launch so they can be judged at large text sizes | 20 | #6545, #6513, #6508 |

## Next steps

- Run `FDynamicTypeAuditUITests` group by group after the follow-ups above land, one `-only-testing:MonacoUITests/FDynamicTypeAuditUITests/<group>` call per group under `scripts/qa/xcode-lock.sh xcode`, on one simulator. #6586 stopped its run during the first group to free the simulator for the follow-up lanes; at about 40 s per frame and size the full run takes about two hours.
- Reshoot the 20 `no shot` pairs once #7127 makes their launch lines draw, and replace each row with a verdict.
