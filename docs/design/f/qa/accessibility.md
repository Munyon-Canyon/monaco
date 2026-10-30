# F accessibility audit (#6587)

This page records the F accessibility pass: the automated audit and its first run, the code reads that could be done without a device, and the follow-ups. The on-device VoiceOver check has not happened yet.

## How it was run

- The audit file is in place: `apps/mobile/MonacoUITests/FAccessibilityAuditUITests.swift`. It holds the 95 launch frames of kind `screen` (`scripts/qa/parity.py frames --kind screen`) with the launch arguments from the `# F parity` block of `scripts/qa/sample-screens.txt`, grouped into 17 tests by the two-digit frame prefix (`testGroup02` … `testGroup19`; `04b-*` runs in group 04).
- Each frame launches with `-MonacoSampleAct`, waits for a labelled element, runs `performAccessibilityAudit(for: [.hitRegion, .sufficientElementDescription, .elementDetection, .trait])`, then swipes up and audits again until the screen stops changing (at most 8 swipes).
- The inline allow-list `FAuditScreens.allowed` starts empty. Every entry added later needs a reason.
- It prints `a11y|` (one line per labelled or interactive element), `audit|` (one per issue) and `small|` (one per interactive element under 44 × 44) lines.
- Local only. Nothing here runs in CI.
- Run on 2026-10-29 on the lane simulator "Monaco 6587" (iPhone 18 Pro Max, iOS 27.0, stock, headless `simctl boot`), Xcode 27.0, through `scripts/qa/xcode-lock.sh xcode xcodebuild test ... -skipPackagePluginValidation -only-testing:MonacoUITests/FAccessibilityAuditUITests`. All 17 group tests ran in 1,560 s; the run then hit `xcode-lock`'s 1,800 s hold cap during `testVoteButtonsAreLabelled`, so the four hold and toast tests were rerun on their own after the fixes below.
- **Pending for the operator, on-device VoiceOver:** on a phone with a Debug build and VoiceOver on, on the proposal, Home's card and the hold sheet, focus the picked Yes, swipe down to "Vote Yes" and double-tap: the vote commits with no hold. Then double-tap and hold on No: the long press passes through. Record the device, iOS version and date here.
- XCUITest cannot perform an accessibility custom action, so the named action is covered by the unit test and the code read below, and by the manual check.

### Frames not in the list

None. `f__05-comment-actions` has the harness line `-commentThreadHarness actions`, so it needs no drive-to-state step. The four `f__01-*` gallery lines are not kind `screen` and stay out.

### Deviation from the ticket

`f__12-toast` is the settling vote's toast (`-MonacoParitySample toast-settled`), which has no Undo by design. `testUndoToastIsReachable` therefore asserts on `f__12-toast` that the banner shows and no Undo exists, then drives an ordinary vote on `f__04-vote-idle` (tap, then press for 0.8 s) and asserts `monaco-toast-action` is labelled "Undo", hittable and at least 44 × 44.

## Audit result

Distinct issues (frame, element, type), with swipe repeats removed: 142.

| Audit type | Issues | What they are |
| --- | --- | --- |
| Hit region | 126 | Mostly static text the audit treats as tappable: comment bodies, relative times ("30m", "3m"), `proposal-vote-status`, `proposal-result-tally`, `cabal-pot-value`, `platform-balance-value`, `home-portfolio-chip`. 65 are in group 04, where each comment body and time repeats across 15 frames. The buttons under 44 pt are in the per-group tables |
| Element detection | 12 | "Potentially inaccessible text", no element named, on 03-cabal-empty, 04-reply-composer, 05-post-reply, 08-review-sheet(-preipo), 10-profile-other, 11-invite-sheet, 12-info-sheet, 14-hold-sheet, 14-wrap-sheet, 18-invite-landing, 19-permission-sheet. Most are sheets, so the sheet's title or grabber is the likely element; confirm in the follow-up |
| Element description | 3 | "$1,000.00" not human-readable (10-profile-own, -scrolled); one element with no description on 05-post-reply |
| Trait | 1 | `withdraw-paste` lacks the not-enabled trait on 09-withdraw-settled |

Per group: 02 hit 6 · 03 hit 9, detect 1 · 04 hit 65, detect 1 · 05 hit 28, desc 1, detect 1 · 06 hit 1 · 07 hit 3 · 08 detect 2 · 09 trait 1 · 10 hit 2, desc 2, detect 1 · 11 detect 1 · 12 hit 2, detect 1 · 14 detect 2 · 15 hit 5 · 16 hit 4 · 18 hit 1, detect 1 · 19 detect 1. Group 13 has none.

Fixed by #7197: the comment composer field now has a 44 pt minimum height, and a comment row is one 44 pt tappable (double-tap to react, long press for actions) with the relative time hidden from accessibility because the row label already speaks it. This resolves the `comment-composer-field`, `comment-body-*` and relative-time hit findings below.

Fixed by #7198: `proposal-vote-status`, `proposal-result-tally`, `platform-balance-value`, `home-portfolio-chip` and `cabal-pot-value` now have a 44 pt minimum height (`MonacoButtonMetrics.hitTarget`), with a full-row content shape on the result tally's button. This resolves their hit findings in groups 02, 03 and 04 below; the audit groups were not rerun on this change. The Treasury line's clipping in the parity frame (#7188) is likely `CabalPotBand`'s `.padding(.top, -pull)`, which leaves the band only `Space.xs` of layout height; that layout fix is out of scope here.

Test results: `testGroup13`, `testReduceMotionScreensDraw`, `testTheListHoldsTheNinetyFiveScreens` and `testEveryAllowedFindingHasAReason` pass. The other 16 group tests fail on the issues above; the allow-list stays empty because each issue has a follow-up instead.

Fixes to the test from this run:

- Sizes were truncated (`Int(43.999…)` printed 43 and counted as small). They are now rounded. The tables below count a 43 pt reading from this run as 44.
- `testUndoToastIsReachable` failed on an Undo 43.99999999999997 pt wide. It now compares the rounded size.
- `f__14-hold-sheet` (`-MonacoProposalSample holdYes`) draws the sheet's error state ("Try again", no vote buttons), so the three hold tests now run on `f__04-vote-idle` and `f__04b-hold-1-pick` only. Follow-up #7096.

After those fixes, `testHoldCommitsOnPressAndHold` (0.8 s press commits), `testShortPressDoesNotCommit` (0.2 s does not), `testVoteButtonsAreLabelled` and `testUndoToastIsReachable` (Undo is a hittable 44 pt button while the toast is up) all pass: `** TEST SUCCEEDED **`, 4 tests, same simulator.

## Code reads

### Vote buttons: one `VotePair` on every hold surface

    rg -n 'VotePair\(|accessibilityAction\(named: "Vote' apps/mobile/Monaco/Features

    Features/Home/HomeVoteCard.swift:67:            VotePair(
    Features/Proposals/VoteHoldSheet.swift:154:        VotePair(
    Features/Proposals/ProposalVoteSection.swift:138:                VotePair(

The second pattern matches nothing: the named action is not written with `accessibilityAction(named:)`. `Design/VoteTally.swift` (`VotePair.button`) declares it inside `.accessibilityActions { if isInteractive { Button("Vote \(side.title)") { perform(side) } } }`, and `perform` calls the same `onCommit` the hold reaches (`apply` handles `.reached` and `.performed` by calling `onCommit(side)`). The plain default action (`.accessibilityAction { pickWithoutHold(side) }`) picks without committing. The hint is "Pick, then hold to vote".

| Surface | Draws `VotePair` | `onCommit` | Verdict |
| --- | --- | --- | --- |
| Proposal (`ProposalVoteSection`) | yes, prefix `proposal` | `commit` | pass |
| Home card (`HomeVoteCard`) | yes, prefix is the slot's, suffix the proposal id | `commit` | pass, with the gap below |
| Hold sheet (`VoteHoldSheet`) | yes | its own commit | pass |

Gap on Home's card: while a hold is in progress `HomeVoteCard.buttons` swaps `VotePair` for a frozen `VoteHoldRow` of bare `VoteHoldButton`s (`frozen(...)`). The frozen buttons carry the label and identifier but no actions. If the card re-renders mid-hold under VoiceOver the "Vote Yes" action is missing until the hold ends. Low risk, since VoiceOver users use the named action and never hold.

Labels, from `VoteHoldButton.spokenLabel`: "Yes, 4 votes" / "No, 1 vote"; after commit "Voted Yes, 5 votes". The selected trait is set when picked or committed. Identifiers are `<prefix>-vote-yes[-<suffix>]` and `<prefix>-vote-no[-<suffix>]`.

The ticket's Reference table wants the committed state as one element ("You voted Yes. Passed 5 to 1, 6 of 8 voted, buying 1.82 shares"). The code gives the button label "Voted Yes, N votes" plus `proposal-voted` as a separate text. Compare with the table in the UI run.

### `VoteTallyTests`

    rg -n 'Vote Yes|accessibilityCustomActions|accessibilityHint' apps/mobile/MonacoTests/VoteTallyTests.swift

    :299  #expect(yes.accessibilityLabel == "Yes, 4 votes")
    :301  #expect(no.accessibilityLabel == "No, 1 vote")
    :302  #expect(yes.accessibilityHint == "Pick, then hold to vote")
    :303  #expect(actions.contains("Vote Yes"))
    :305  let action = try #require(yes.accessibilityCustomActions?.first { $0.name == "Vote Yes" })
    :306  _ = action.actionHandler?(action)

The case asserts the label, the hint, the "Vote Yes" action, and that invoking it commits. It covers `proposal-vote-yes` only. It does not assert "Vote No", and it does not cover the Home card or the hold sheet. The passing run is for the operator's MonacoTests pass (not run here).

### Toast timing and VoiceOver focus

`Design/MonacoToastBanner.swift`, `MonacoToastTiming.dwell`:

- Without VoiceOver: reading time (2 s + 0.06 s per character), floored at 2.5 s for success and 4 s for failure, at least 4 s with an action, capped at 10 s.
- With VoiceOver and no action: dwell × 1.5, capped at 15 s.
- With VoiceOver and an action: `max(expiresAt - now, dwell)`, capped at 15 s. #6430's `undo_until` is 15 s, so an Undo toast stays up for the whole window.
- `dismiss(_:after:)` skips the countdown tick while `voiceOverEnabled && current.action != nil && bannerFocused`, so the toast holds while VoiceOver focus is on an Undo toast. A toast without an action does not hold under focus and only gets the 1.5 × dwell.
- The banner posts `AccessibilityNotification.Announcement` ("<message>. Undo available.") when it appears, and `Haptics.warning()` on an error toast.
- The Undo button is `monaco-toast-action`, minimum 44 pt (`MonacoToastLayout.minimumHeight`), focusable through `toastFocus`.
- The Undo button disappears at `expiresAt` (`refreshAction`), even if the toast stays.

Verdict: announce, reach Undo, and hold under focus all pass by the code. The toast without an action still dismisses under focus, which is fine for a message but not for a link (`monaco-toast-link` shares the banner). Confirm on a device.

### Haptics

    grep -rnE 'sensoryFeedback|Haptics\.' apps/mobile/Monaco/Features apps/mobile/Monaco/Design

About 90 hits. Checked against the house rules:

| Rule | Reading |
| --- | --- |
| `.selection` on chips, tabs, period labels, presets, keypad chips, vote pick, chart scrub | pass: `MonacoChipBar`, `MonacoUnderlineTabs`, `MonacoRangeChips`, `AmountKeypad`, `MonacoCandleChart` (scrub), `VoteHaptics.pick` |
| `.success` once at the end of a hold, never on the pick | `VoteHaptics.live` is `pick: selection, reached: tap, landed: success`. `landed` plays on the selection change (`VoteHaptics.playsLanded`), `reached` plays `Haptics.tap()` at 100% of the fill. The commit therefore plays a light impact and then a success. See follow-up V1 |
| `.success` on proposal sent and money settled | `ProposalPassedSheet` and `AmountKeypadScreen` (`newState.isSettled`) play it. Not checked for the review sheet send (`f__08-review-sheet`) |
| `.warning` on every error toast | `MonacoToastModifier` plays it for any `!isSuccess` toast. Also called by hand in `HomeVoteCard`, `VoteHoldSheet`, `WithdrawScreen`, so those errors may play twice. See V2 |
| No `Haptics.tap()` behind a prominent button | `MonacoButtons.swift:355` (the circle action button) and `Features/Notifications/CatchUpCardView.swift:176`, `Features/Groups/GroupChatParts.swift:330` (chat send), `Features/Comments/CommentRow.swift:101` (long-press on a reply) call `Haptics.tap()` in an action. Whether they sit behind a prominent button needs a check of each style. The prominent style itself fires `.impact(.light)` through `sensoryFeedback` (`MonacoButtons.swift:123`), so the chat send and the circle button are the doubles to confirm |
| Fund and withdraw invalid | `.sensoryFeedback(.error, ...)` in `FundCabalScreen:162` and `WithdrawScreen:122` instead of `.warning`. The house rule names `.warning` for error toasts only, so this is consistent, but note it |

### Reduce Motion

Read item by item (the five-frame draw test is in the file and has not been run):

| Item | Reading |
| --- | --- |
| Hold draws a fill without the ring sweep or spring | `VoteHoldButton.swift:97`: the ring stroke draws only when `!reduceMotion`. The fill sweep stays. Pass. No spring was found in the file |
| Counts do not roll | `MonacoMoney` (two places), `MonacoAmountEntry`, `AmountKeypadScreen` use `.identity` under Reduce Motion. `Features/Home/BalancesView.swift:108` uses `.numericText()` with no check. See R1. `.opacity` transitions (`MonacoAmountEntry:132`, `CabalRulesSection:84`) are fades, fine |
| Podium crown and climb bar do not animate | Not found: no `reduceMotion` in `MonacoBoardRow.swift`, `CallBoardYouCard.swift`, `CallBoardDocked.swift` or `HomeLeaderboardSlot.swift`, which are where the crown and climb live. `MonacoBoardList` gates its row reorder animation. Not verified: check for an unconditional `withAnimation` on the leaderboard moment (`f__15-leaderboard-moment`) in the UI run. See R2 |
| Chart does not draw on | `MonacoCandleChart.swift:171` and `:289` pass `nil` animation under Reduce Motion. Pass |
| Skeletons static | Not read. No skeleton view was reviewed in this pass |
| Toast keeps only its fade | `MonacoToastModifier`: the transition is `.opacity` under Reduce Motion, else slide plus fade; `transitionAnimation` is a plain 0.25 s ease. Pass |

## Per-group tables

From the `a11y|` lines of the run: every button, link, switch, slider and text field whose hit frame is under 44 × 44 pt, one row per identifier (or label when there is none) with the smallest size seen, and each group's audit issues. Every other interactive element measured passed. System elements (sheet grabbers, keyboard keys) are left out. The full output (13,062 `a11y|` lines) is kept with the run, not here.

### Group 02

| Frames | Identifier | Label | Trait | Smallest size | Verdict |
| --- | --- | --- | --- | --- | --- |
| 3 frames | `home-vote-open-home-proposal-1` | Buy NVDA · $400, AC Capital · 5 of 8 voted · 6 replies · 4h | button | 375x40 | small |

Audit issues (6):

| Frame | Identifier | Label | Type | Issue |
| --- | --- | --- | --- | --- |
| 02-home-dark | `home-portfolio-chip` | Net worth, 12,082 dollars, up 241 dollars 10 cents, 2.0 percent, past month | hit | Hit area is too small |
| 02-home-dark | `platform-balance-value` | Cash | hit | Hit area is too small |
| 02-home-empty | `platform-balance-value` | Cash | hit | Hit area is too small |
| 02-home-light | `home-portfolio-chip` | Net worth, 12,082 dollars, up 241 dollars 10 cents, 2.0 percent, past month | hit | Hit area is too small |
| 02-home-light | `platform-balance-value` | Cash | hit | Hit area is too small |
| 02-home-scrolled | `platform-balance-value` | Cash | hit | Hit area is too small |

### Group 03

| Frames | Identifier | Label | Trait | Smallest size | Verdict |
| --- | --- | --- | --- | --- | --- |
| 9 frames | `—` | — | button | 48x36 | small |
| 9 frames | `cabal-more-button` | More | button | 48x36 | small |
| 8 frames | `cabal-pot-value` | Treasury, $20,000.00, down 4.1 percent, 1M | button | 256x18 | small |
| 3 frames | `cabal-rules-show-sides-toggle` | Show who voted what | button | 61x28 | small |
| 03-cabals-tab-menu | `cabals-menu-sample` | Fund | button | 408x42 | small |
| 03-cabals-tab | `cabals-vote-open-home-proposal-1` | Buy NVDA · $400, AC Capital · 5 of 8 voted · 6 replies · 4h | button | 375x40 | small |
| 03-cabal-members-all | `members-invite-button` | Invite to this cabal | button | 28x36 | small |

Audit issues (10):

| Frame | Identifier | Label | Type | Issue |
| --- | --- | --- | --- | --- |
| 03-cabal-empty | `—` | — | detect | Potentially inaccessible text |
| 03-cabal-empty | `cabal-pot-value` | Treasury, $600.00 | hit | Hit area is too small |
| 03-cabal-members | `cabal-pot-value` | Treasury, $20,000.00, down 4.1 percent, 1M | hit | Hit area is too small |
| 03-cabal-members-all | `—` | Asked to join · Jordan's link · 2h | hit | Hit area is too small |
| 03-cabal-members-all | `—` | Leo | hit | Hit area is too small |
| 03-cabal-rules | `cabal-pot-value` | Treasury, $20,000.00, down 4.1 percent, 1M | hit | Hit area is too small |
| 03-cabal-rules-scrolled | `cabal-pot-value` | Treasury, $20,000.00, down 4.1 percent, 1M | hit | Hit area is too small |
| 03-cabal-rules-sides | `cabal-pot-value` | Treasury, $20,000.00, down 4.1 percent, 1M | hit | Hit area is too small |
| 03-cabal-treasury | `cabal-pot-value` | Treasury, $20,000.00, down 4.1 percent, 1M | hit | Hit area is too small |
| 03-cabal-treasury-light | `cabal-pot-value` | Treasury, $20,000.00, down 4.1 percent, 1M | hit | Hit area is too small |

### Group 04

| Frames | Identifier | Label | Trait | Smallest size | Verdict |
| --- | --- | --- | --- | --- | --- |
| 04-reply-composer | `—` | — | button | 42x42 | small |
| 04-reply-composer | `—` | Continue | button | 274x33 | small |
| 04-gif-reply | `comment-author-c1` | Mia | button | 41x44 | small |
| 15 frames | `comment-composer-field` | — | button | 223x19 | small (fixed by #7197) |
| 04-reply-composer | `comment-composer-gif` | Add a GIF | button | 42x42 | small |
| 04-reply-composer | `comment-composer-plus` | Add a poll, quote or stock | button | 42x42 | small |
| 04-reply-composer | `comment-composer-send` | Send reply | button | 42x42 | small |
| 04-gif-sheet | `gif-close` | Close | button | 42x42 | small |
| 3 frames | `home-vote-open-home-proposal-1` | Buy NVDA · $400, Big Ballers · 5 of 8 voted · 6 replies · 4h | button | 375x40 | small |
| 04-gif-sheet | `monaco-search-field` | Search GIFs | button | 341x21 | small |
| 5 frames | `proposal-result-tally` | ✓ 5 · 6 of 8 voted · 1 ✕ | button | 155x18 | small |
| 7 frames | `proposal-vote-status` | 5 of 8 voted, 6 replies, ends in 3h | button | 237x18 | small |
| 04-reply-composer | `reply-sheet-close` | Close | button | 42x42 | small |

Audit issues (66):

| Frame | Identifier | Label | Type | Issue |
| --- | --- | --- | --- | --- |
| 04-gif-reply | `—` | 5m | hit | Hit area is too small |
| 04-gif-reply | `comment-body-c1` | What would our NVIDIA allocation be after this? | hit | Hit area is too small |
| 04-proposal-dark | `—` | 30m | hit | Hit area is too small |
| 04-proposal-dark | `—` | 3m | hit | Hit area is too small |
| 04-proposal-dark | `comment-body-c1` | Earnings beat three quarters running. | hit | Hit area is too small |
| 04-proposal-dark | `comment-body-c4` | In. | hit | Hit area is too small |
| 04-proposal-dark | `proposal-vote-status` | 5 of 8 voted, 6 replies, ends in 3h | hit | Hit area is too small |
| 04-proposal-failed | `—` | 30m | hit | Hit area is too small |
| 04-proposal-failed | `—` | 3m | hit | Hit area is too small |
| 04-proposal-failed | `comment-body-c1` | Earnings beat three quarters running. | hit | Hit area is too small |
| 04-proposal-failed | `comment-body-c4` | In. | hit | Hit area is too small |
| 04-proposal-failed | `proposal-result-tally` | ✓ 2 · 2 of 3 voted · 0 ✕ | hit | Hit area is too small |
| 04-proposal-light | `—` | 30m | hit | Hit area is too small |
| 04-proposal-light | `—` | 3m | hit | Hit area is too small |
| 04-proposal-light | `comment-body-c1` | Earnings beat three quarters running. | hit | Hit area is too small |
| 04-proposal-light | `comment-body-c4` | In. | hit | Hit area is too small |
| 04-proposal-light | `proposal-vote-status` | 5 of 8 voted, 6 replies, ends in 3h | hit | Hit area is too small |
| 04-proposal-passed | `—` | 30m | hit | Hit area is too small |
| 04-proposal-passed | `—` | 3m | hit | Hit area is too small |
| 04-proposal-passed | `comment-body-c1` | Earnings beat three quarters running. | hit | Hit area is too small |
| 04-proposal-passed | `comment-body-c4` | In. | hit | Hit area is too small |
| 04-proposal-passed | `proposal-result-tally` | ✓ 2 · 2 of 3 voted · 0 ✕ | hit | Hit area is too small |
| 04-proposal-preipo | `—` | 30m | hit | Hit area is too small |
| 04-proposal-preipo | `—` | 3m | hit | Hit area is too small |
| 04-proposal-preipo | `comment-body-c1` | Earnings beat three quarters running. | hit | Hit area is too small |
| 04-proposal-preipo | `comment-body-c4` | In. | hit | Hit area is too small |
| 04-proposal-preipo | `proposal-vote-status` | 2 of 6 voted, 6 replies, ends in 19h | hit | Hit area is too small |
| 04-proposal-scrolled | `—` | 30m | hit | Hit area is too small |
| 04-proposal-scrolled | `—` | 3m | hit | Hit area is too small |
| 04-proposal-scrolled | `comment-body-c1` | Earnings beat three quarters running. | hit | Hit area is too small |
| 04-proposal-scrolled | `comment-body-c4` | In. | hit | Hit area is too small |
| 04-proposal-scrolled | `proposal-vote-status` | 5 of 8 voted, 6 replies, ends in 3h | hit | Hit area is too small |
| 04-proposal-sides | `—` | 30m | hit | Hit area is too small |
| 04-proposal-sides | `—` | 3m | hit | Hit area is too small |
| 04-proposal-sides | `comment-body-c1` | Earnings beat three quarters running. | hit | Hit area is too small |
| 04-proposal-sides | `comment-body-c4` | In. | hit | Hit area is too small |
| 04-proposal-sides | `proposal-result-tally` | ✓ 2 · 2 of 3 voted · 0 ✕ | hit | Hit area is too small |
| 04-reply-composer | `—` | — | detect | Potentially inaccessible text |
| 04-vote-closed | `—` | 30m | hit | Hit area is too small |
| 04-vote-closed | `—` | 3m | hit | Hit area is too small |
| 04-vote-closed | `comment-body-c1` | Earnings beat three quarters running. | hit | Hit area is too small |
| 04-vote-closed | `comment-body-c4` | In. | hit | Hit area is too small |
| 04-vote-closed | `proposal-result-tally` | ✓ 2 · 2 of 3 voted · 0 ✕ | hit | Hit area is too small |
| 04-vote-committed | `—` | 30m | hit | Hit area is too small |
| 04-vote-committed | `—` | 3m | hit | Hit area is too small |
| 04-vote-committed | `comment-body-c1` | Earnings beat three quarters running. | hit | Hit area is too small |
| 04-vote-committed | `comment-body-c4` | In. | hit | Hit area is too small |
| 04-vote-committed | `proposal-result-tally` | ✓ 5 · 6 of 8 voted · 1 ✕ | hit | Hit area is too small |
| 04-vote-holding | `—` | 30m | hit | Hit area is too small |
| 04-vote-holding | `—` | 3m | hit | Hit area is too small |
| 04-vote-holding | `comment-body-c1` | Earnings beat three quarters running. | hit | Hit area is too small |
| 04-vote-holding | `comment-body-c4` | In. | hit | Hit area is too small |
| 04-vote-holding | `proposal-vote-status` | 5 of 8 voted, 6 replies, ends in 3h | hit | Hit area is too small |
| 04-vote-idle | `—` | 30m | hit | Hit area is too small |
| 04-vote-idle | `—` | 3m | hit | Hit area is too small |
| 04-vote-idle | `comment-body-c1` | Earnings beat three quarters running. | hit | Hit area is too small |
| 04-vote-idle | `comment-body-c4` | In. | hit | Hit area is too small |
| 04-vote-idle | `proposal-vote-status` | 5 of 8 voted, 6 replies, ends in 3h | hit | Hit area is too small |
| 04-vote-picked | `—` | 30m | hit | Hit area is too small |
| 04-vote-picked | `—` | 3m | hit | Hit area is too small |
| 04-vote-picked | `comment-body-c1` | Earnings beat three quarters running. | hit | Hit area is too small |
| 04-vote-picked | `comment-body-c4` | In. | hit | Hit area is too small |
| 04-vote-picked | `proposal-vote-status` | 5 of 8 voted, 6 replies, ends in 3h | hit | Hit area is too small |
| 04b-hold-1-pick | `platform-balance-value` | Cash | hit | Hit area is too small |
| 04b-hold-2-holding | `platform-balance-value` | Cash | hit | Hit area is too small |
| 04b-hold-3-committed | `platform-balance-value` | Cash | hit | Hit area is too small |

### Group 05

| Frames | Identifier | Label | Trait | Smallest size | Verdict |
| --- | --- | --- | --- | --- | --- |
| 5 frames | `—` | — | button | 42x42 | small |
| 3 frames | `comment-composer-field` | — | button | 223x19 | small (fixed by #7197) |
| 05-post-reply | `comment-composer-gif` | Add a GIF | button | 42x42 | small |
| 05-post-reply | `comment-composer-plus` | Add a poll, quote or stock | button | 42x42 | small |
| 05-post-reply | `comment-composer-send` | Send reply | button | 42x42 | small |
| 05-emoji-picker | `emoji-category-flags` | Flags | button | 78x42 | small |
| 05-emoji-picker | `emoji-category-nature` | Animals and nature | button | 78x42 | small |
| 05-emoji-picker | `emoji-category-people` | People and body | button | 78x42 | small |
| 05-emoji-picker | `emoji-category-recent` | Frequently used | button | 78x42 | small |
| 05-emoji-picker | `emoji-category-smileys` | Smileys and people | button, selected | 78x42 | small |
| 05-emoji-picker | `emoji-cell-*` | grinning face | button | 52x42 | small |
| 05-emoji-picker | `emoji-frequent-1F389` | party popper | button | 52x42 | small |
| 05-emoji-picker | `emoji-frequent-1F440` | eyes | button | 52x42 | small |
| 05-emoji-picker | `emoji-frequent-1F44D` | thumbs up | button | 52x42 | small |
| 05-emoji-picker | `emoji-frequent-1F4AA` | flexed biceps | button | 52x42 | small |
| 05-emoji-picker | `emoji-frequent-1F602` | face with tears of joy | button | 52x42 | small |
| 05-emoji-picker | `emoji-frequent-1F64C` | raising hands | button | 52x42 | small |
| 05-emoji-picker | `emoji-frequent-1F91D` | handshake | button | 52x42 | small |
| 05-emoji-picker | `emoji-frequent-2764-FE0F` | red heart | button | 52x42 | small |
| 05-emoji-picker | `emoji-search` | — | button | 342x21 | small |
| 4 frames | `feed-filter` | Filter feed | button | 56x36 | small |
| 05-post, 05-post-scrolled | `post-share` | Share | button | 17x23 | small |
| 05-post-public | `proposal-vote-status` | 3 of 7 voted, 2 replies, ends in 22h | button | 244x18 | small |
| 05-post-reply | `reply-sheet-close` | Close | button | 42x42 | small |

Audit issues (30):

| Frame | Identifier | Label | Type | Issue |
| --- | --- | --- | --- | --- |
| 05-comment-actions | `—` | 20m | hit | Hit area is too small |
| 05-comment-actions | `—` | 30m | hit | Hit area is too small |
| 05-comment-actions | `—` | 40m | hit | Hit area is too small |
| 05-comment-actions | `—` | 50m | hit | Hit area is too small |
| 05-comment-actions | `comment-body-aapl-andres` | Good timing on this one. | hit | Hit area is too small |
| 05-comment-actions | `comment-body-aapl-nora` | Why sell here? | hit | Hit area is too small |
| 05-comment-actions | `comment-body-aapl-sam` | He called the top again. | hit | Hit area is too small |
| 05-post | `—` | 20m | hit | Hit area is too small |
| 05-post | `—` | 30m | hit | Hit area is too small |
| 05-post | `—` | 40m | hit | Hit area is too small |
| 05-post | `—` | 50m | hit | Hit area is too small |
| 05-post | `comment-body-aapl-andres` | Good timing on this one. | hit | Hit area is too small |
| 05-post | `comment-body-aapl-nora` | Why sell here? | hit | Hit area is too small |
| 05-post | `comment-body-aapl-sam` | He called the top again. | hit | Hit area is too small |
| 05-post | `post-share` | Share | hit | Hit area is too small |
| 05-post-public | `—` | 30m | hit | Hit area is too small |
| 05-post-public | `—` | 3m | hit | Hit area is too small |
| 05-post-public | `comment-body-c1` | Earnings beat three quarters running. | hit | Hit area is too small |
| 05-post-public | `comment-body-c4` | In. | hit | Hit area is too small |
| 05-post-reply | `—` | — | desc | Element has no description |
| 05-post-reply | `—` | — | detect | Potentially inaccessible text |
| 05-post-reply | `comment-composer-field` | — | hit | Hit area is too small (fixed by #7197) |
| 05-post-scrolled | `—` | 20m | hit | Hit area is too small |
| 05-post-scrolled | `—` | 30m | hit | Hit area is too small |
| 05-post-scrolled | `—` | 40m | hit | Hit area is too small |
| 05-post-scrolled | `—` | 50m | hit | Hit area is too small |
| 05-post-scrolled | `comment-body-aapl-andres` | Good timing on this one. | hit | Hit area is too small |
| 05-post-scrolled | `comment-body-aapl-nora` | Why sell here? | hit | Hit area is too small |
| 05-post-scrolled | `comment-body-aapl-sam` | He called the top again. | hit | Hit area is too small |
| 05-post-scrolled | `post-share` | Share | hit | Hit area is too small |

### Group 06

| Frames | Identifier | Label | Trait | Smallest size | Verdict |
| --- | --- | --- | --- | --- | --- |
| 4 frames | `monaco-search-field` | Stocks and Pre-IPO | button | 291x19 | small |

Audit issues (1):

| Frame | Identifier | Label | Type | Issue |
| --- | --- | --- | --- | --- |
| 06-markets-search | `monaco-search-field` | Stocks and Pre-IPO | hit | Hit area is too small |

### Group 08

| Frames | Identifier | Label | Trait | Smallest size | Verdict |
| --- | --- | --- | --- | --- | --- |
| 08-review-sheet, 08-review-sheet-preipo | `propose-review-header` | Alphabet, Buy · market order, Market order, $25 | button | 362x40 | small |
| 08-review-sheet, 08-review-sheet-preipo | `propose-review-market-order-info` | Market order | button | 18x18 | small |
| 08-review-sheet, 08-review-sheet-preipo | `xmark` | Close | button | 42x42 | small |

Audit issues (2):

| Frame | Identifier | Label | Type | Issue |
| --- | --- | --- | --- | --- |
| 08-review-sheet | `—` | — | detect | Potentially inaccessible text |
| 08-review-sheet-preipo | `—` | — | detect | Potentially inaccessible text |

### Group 09

| Frames | Identifier | Label | Trait | Smallest size | Verdict |
| --- | --- | --- | --- | --- | --- |
| 3 frames | `amount-keypad-source` | Cash $500.00 | button | 188x34 | small |
| 4 frames | `withdraw-paste` | Paste | button | 41x34 | small |

Audit issues (1):

| Frame | Identifier | Label | Type | Issue |
| --- | --- | --- | --- | --- |
| 09-withdraw-settled | `withdraw-paste` | Paste | trait | Missing trait: UIAccessibilityTraitNotEnabled |

### Group 10

| Frames | Identifier | Label | Trait | Smallest size | Verdict |
| --- | --- | --- | --- | --- | --- |
| 10-profile-other, 10-profile-other-cabals | `—` | — | button | 58x36 | small |
| 10-profile-other, 10-profile-other-cabals | `—` | More | button | 58x36 | small |

Audit issues (5):

| Frame | Identifier | Label | Type | Issue |
| --- | --- | --- | --- | --- |
| 10-profile-other | `—` | — | detect | Potentially inaccessible text |
| 10-profile-own | `—` | $1,000.00 | desc | Label not human-readable |
| 10-profile-own | `profile-balance-value` | Cash | hit | Hit area is too small |
| 10-profile-own-scrolled | `—` | $1,000.00 | desc | Label not human-readable |
| 10-profile-own-scrolled | `profile-balance-value` | Cash | hit | Hit area is too small |

### Group 11

| Frames | Identifier | Label | Trait | Smallest size | Verdict |
| --- | --- | --- | --- | --- | --- |
| 11-invite-sheet | `—` | — | button | 42x42 | small |
| 11-invite-sheet | `cabal-invite-qr-toggle` | Show QR code | button | 42x42 | small |
| 11-invite-sheet | `cabal-invite-settings` | Invite settings | button | 42x42 | small |
| 6 frames | `share-format-story` | Story | button | 36x44 | small |
| 11-invite-sheet | `xmark` | Close | button | 42x42 | small |

Audit issues (1):

| Frame | Identifier | Label | Type | Issue |
| --- | --- | --- | --- | --- |
| 11-invite-sheet | `—` | — | detect | Potentially inaccessible text |

### Group 12

| Frames | Identifier | Label | Trait | Smallest size | Verdict |
| --- | --- | --- | --- | --- | --- |
| 12-toast | `home-vote-open-home-proposal-1` | Buy NVDA · $400, Big Ballers · 5 of 8 voted · 6 replies · 7h | button | 375x40 | small |
| 12-info-sheet | `market-stats-sheet` | Close | button | 42x42 | small |

Audit issues (3):

| Frame | Identifier | Label | Type | Issue |
| --- | --- | --- | --- | --- |
| 12-info-sheet | `—` | — | detect | Potentially inaccessible text |
| 12-toast | `home-portfolio-chip` | Net worth, 11,082 dollars, down 417 dollars 68 cents, 3.5 percent, past month | hit | Hit area is too small |
| 12-toast | `platform-balance-value` | Cash | hit | Hit area is too small |

### Group 13

| Frames | Identifier | Label | Trait | Smallest size | Verdict |
| --- | --- | --- | --- | --- | --- |
| 5 frames | `leaderboard-period-week` | Week | button | 39x44 | small |

### Group 14

| Frames | Identifier | Label | Trait | Smallest size | Verdict |
| --- | --- | --- | --- | --- | --- |
| 14-wrap-sheet | `ledger-sheet` | Close | button | 42x42 | small |
| 14-ledger-story | `share-format-story` | Story | button, selected | 39x44 | small |
| 14-hold-sheet | `vote-hold-sheet` | Close | button | 25x25 | small |

Audit issues (2):

| Frame | Identifier | Label | Type | Issue |
| --- | --- | --- | --- | --- |
| 14-hold-sheet | `—` | — | detect | Potentially inaccessible text |
| 14-wrap-sheet | `—` | — | detect | Potentially inaccessible text |

### Group 15

| Frames | Identifier | Label | Trait | Smallest size | Verdict |
| --- | --- | --- | --- | --- | --- |
| 15-passed | `comment-composer-field` | — | button | 232x20 | small (fixed by #7197) |
| 15-leaderboard-moment | `leaderboard-period-week` | Week | button | 39x44 | small |
| 15-passed | `proposal-result-tally` | ✓ 2 · 2 of 3 voted · 0 ✕ | button | 157x18 | small |

Audit issues (5):

| Frame | Identifier | Label | Type | Issue |
| --- | --- | --- | --- | --- |
| 15-passed | `—` | 30m | hit | Hit area is too small |
| 15-passed | `—` | 3m | hit | Hit area is too small |
| 15-passed | `comment-body-c1` | Earnings beat three quarters running. | hit | Hit area is too small |
| 15-passed | `comment-body-c4` | In. | hit | Hit area is too small |
| 15-passed | `proposal-result-tally` | ✓ 2 · 2 of 3 voted · 0 ✕ | hit | Hit area is too small |

### Group 16

| Frames | Identifier | Label | Trait | Smallest size | Verdict |
| --- | --- | --- | --- | --- | --- |
| 16-catch-up | `catch-up-one-by-one` | One by one | button | 128x36 | small |
| 16-catch-up | `catch-up-row-invite:00000000-0000-7000-8000-00000000e001` | Sam invited you to Quant Kids, 5 members · holds OpenAI. 2 hours ago,  | button | 341x40 | small |

Audit issues (4):

| Frame | Identifier | Label | Type | Issue |
| --- | --- | --- | --- | --- |
| 16-catch-up-empty | `—` | Error: Couldn't load your portfolio. | hit | Hit area is too small |
| 16-catch-up-empty-scrolled | `—` | Error: Couldn't load your portfolio. | hit | Hit area is too small |
| 16-catch-up-open | `—` | Error: Couldn't load your portfolio. | hit | Hit area is too small |
| 16-catch-up-open-scrolled | `—` | Error: Couldn't load your portfolio. | hit | Hit area is too small |

### Group 18

| Frames | Identifier | Label | Trait | Smallest size | Verdict |
| --- | --- | --- | --- | --- | --- |
| 18-invite-landing | `invite-landing-share` | Share invite link | button | 48x36 | small |

Audit issues (2):

| Frame | Identifier | Label | Type | Issue |
| --- | --- | --- | --- | --- |
| 18-invite-landing | `—` | — | detect | Potentially inaccessible text |
| 18-invite-landing | `invite-landing-vote-ends` | 4 yes, 1 no, Join to vote | hit | Hit area is too small |

### Group 19

| Frames | Identifier | Label | Trait | Smallest size | Verdict |
| --- | --- | --- | --- | --- | --- |
| 19-permission-sheet | `push-pre-prompt-not-now` | Not now | button | 54x42 | small |
| 19-permission-sheet | `push-pre-prompt-quietly` | Deliver quietly | button | 94x42 | small |

Audit issues (1):

| Frame | Identifier | Label | Type | Issue |
| --- | --- | --- | --- | --- |
| 19-permission-sheet | `—` | — | detect | Potentially inaccessible text |

Code read on the Feed comment actions: `CommentRow.swift:230-237` gives a reply the custom actions Reply, React, go to the quoted reply, Copy, Report and Delete, so a reply's actions are reachable without the long-press. A `GifBubble` has a default action that opens the viewer.

## Follow-ups

Filed on 2026-10-29 in F launch (phase 1).

### #6460 (hold to vote, `VotePair`)

- **V1 (#7066): one haptic at the end of a hold.** `VoteHaptics.live.reached` is `Haptics.tap` and `landed` is `Haptics.success`, so a committed hold plays a light impact and then a success. Per `docs/design.md` the commit plays `.success` only. Drop `reached` to a no-op or make it the `.success` and drop `landed` for the hold path. Acceptance: a unit test on `VoteHaptics` shows one call at commit.
- **V3 (#7066): "Vote No" in the unit test.** `VoteTallyTests` asserts only "Vote Yes" on the proposal. Add "Vote No", and assert the same on `HomeVoteCard` and `VoteHoldSheet`.

### #6508 (Home vote card)

- **H1 (#7067): named actions survive a hold render.** While `model.phase` is `.holding`, `HomeVoteCard.buttons` draws `VoteHoldRow` of `VoteHoldButton`s with no "Vote Yes" / "Vote No". Keep `VotePair` mounted through the hold, or add the same actions to `frozen(...)`.

### #6458 (toast)

- **T1 (#7068): links and actionless toasts hold under focus.** The countdown holds under VoiceOver focus only when `current.action != nil`. A toast with a link (`monaco-toast-link`) and no action dismisses under focus. Hold it for links too.
- **T2: confirm on a device.** Under VoiceOver an Undo toast stays up until `undo_until` (15 s cap) and the announcement reads "Undo available". Record the result in this page.

### #6421 / the FN ticket for shared haptics

- **V2 (#7069): duplicate error haptic.** `MonacoToastModifier` plays `Haptics.warning()` for any error toast; `HomeVoteCard:153`, `VoteHoldSheet:183`, `WithdrawScreen:125,148` also call it by hand. If those also show an error toast, the warning plays twice. Remove the hand calls where a toast follows.
- **V4 (#7069): `Haptics.tap()` behind prominent buttons.** Check `MonacoButtons.swift:355`, `GroupChatParts.swift:330` (chat send) and `CatchUpCardView.swift:176` against the prominent style's own light impact. Remove the double where the style already fires.

### The Home area ticket

- **R1 (#7070): balance count rolls under Reduce Motion.** `Features/Home/BalancesView.swift:108` uses `.contentTransition(.numericText())` with no Reduce Motion check. Use `reduceMotion ? .identity : .numericText()`.

### The leaderboard area ticket

- **R2: crown and climb bar under Reduce Motion.** Confirm in the `f__15-leaderboard-moment` run that neither animates under `-UIAccessibilityReduceMotionEnabled YES`. If one does, gate it on `accessibilityReduceMotion`.

### Targets and samples, from the audit run

- **A1 (#7094): round buttons, sheet closes and composer controls.** 42 × 42 closes and composer GIF, ⊕ and send; the hold sheet's close at 25 × 25; `post-share` at 17 × 23; the review sheet's market-order i at 18 × 18.
- **A2 (#7095): short links, chips, pills and rows.** The 36 pt More, filter, Catch up and invite pills, Withdraw's paste and the keypad source pill at 34 pt, the board's Week at 39 pt wide, Story at 36 pt wide, the emoji picker and push pre-prompt at 42 pt, the 40 pt vote and Catch up rows, the profile's "$1,000.00" label and the paste button's missing not-enabled trait.
- **S1 (#7096): the hold sheet sample.** `-MonacoProposalSample holdYes` draws the error state, so the hold tests skip `f__14-hold-sheet` until it draws the picked frame.

### The operator

- Do the on-device VoiceOver check for "Vote Yes" and the double-tap-and-hold on all three surfaces.
- Check skeletons for Reduce Motion (not read here).

### Follow-up #7199

- **GIF sheet tiles.** Each tile's still is a shape with an image overlay; the tile now ignores its children and the button carries the GIF's title as its label, with "GIF" when the title is empty (`GifSheet.swift`).
- **Already labelled, left as is.** The composer's ⊕ ("plusAccessibility"), GIF, send and remove-attachment buttons, the sheet's Close, the invite settings gear and the review sheet's market-order i carry labels; the composer has no emoji button.
- **Pending.** The audit rerun of groups 04, 08, 10 and 11 has not run; the operator rule keeps UI test runs out of lanes.
