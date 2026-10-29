# F accessibility audit (#6587)

This page records the F accessibility pass: the automated audit that is now in the repo, the code reads that could be done without a device, and the follow-ups. The audit run and the on-device VoiceOver check have not happened yet.

## How it was run

- The audit file is in place: `apps/mobile/MonacoUITests/FAccessibilityAuditUITests.swift`. It holds the 95 launch frames of kind `screen` (`scripts/qa/parity.py frames --kind screen`) with the launch arguments from the `# F parity` block of `scripts/qa/sample-screens.txt`, grouped into 17 tests by the two-digit frame prefix (`testGroup02` … `testGroup19`; `04b-*` runs in group 04).
- Each frame launches with `-MonacoSampleAct`, waits for a labelled element, runs `performAccessibilityAudit(for: [.hitRegion, .sufficientElementDescription, .elementDetection, .trait])`, then swipes up and audits again until the screen stops changing (at most 8 swipes).
- The inline allow-list `FAuditScreens.allowed` starts empty. Every entry added later needs a reason.
- It prints `a11y|` (one line per labelled or interactive element), `audit|` (one per issue) and `small|` (one per interactive element under 44 × 44) lines.
- Local only. Nothing here runs in CI.
- **Pending for the operator, UI run:** `just build mobile`, then run `FAccessibilityAuditUITests` on the gold sim, keep the printed lines, fill the "Audit result" and per-group tables below, and file one follow-up per finding.
- **Pending for the operator, on-device VoiceOver:** on a phone with a Debug build and VoiceOver on, on the proposal, Home's card and the hold sheet, focus the picked Yes, swipe down to "Vote Yes" and double-tap: the vote commits with no hold. Then double-tap and hold on No: the long press passes through. Record the device, iOS version and date here.
- XCUITest cannot perform an accessibility custom action, so the named action is covered by the unit test and the code read below, and by the manual check.

### Frames not in the list

None. `f__05-comment-actions` has the harness line `-commentThreadHarness actions`, so it needs no drive-to-state step. The four `f__01-*` gallery lines are not kind `screen` and stay out.

### Deviation from the ticket

`f__12-toast` is the settling vote's toast (`-MonacoParitySample toast-settled`), which has no Undo by design. `testUndoToastIsReachable` therefore asserts on `f__12-toast` that the banner shows and no Undo exists, then drives an ordinary vote on `f__04-vote-idle` (tap, then press for 0.8 s) and asserts `monaco-toast-action` is labelled "Undo", hittable and at least 44 × 44.

## Audit result

Not run yet. Fill in per audit type (hit region, element description, element detection, trait) after the first run.

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

Fill from the `a11y|` and `small|` lines of the UI run: identifier, label, trait, size, verdict, one table per group (`02`, `03`, `04` with `04b`, `05`, `06`, `07`, `08`, `09`, `10`, `11`, `12`, `13`, `14`, `15`, `16`, `18`, `19`). Include the element order for the five Feed comment frames (`05-post`, `05-post-scrolled`, `05-post-reply`, `05-comment-actions`, `05-post-public`).

Code read on the Feed comment actions: `CommentRow.swift:230-237` gives a reply the custom actions Reply, React, go to the quoted reply, Copy, Report and Delete, so a reply's actions are reachable without the long-press. A `GifBubble` has a default action that opens the viewer.

## Follow-ups

Each is written to be filed as is.

### #6460 (hold to vote, `VotePair`)

- **V1: one haptic at the end of a hold.** `VoteHaptics.live.reached` is `Haptics.tap` and `landed` is `Haptics.success`, so a committed hold plays a light impact and then a success. Per `docs/design.md` the commit plays `.success` only. Drop `reached` to a no-op or make it the `.success` and drop `landed` for the hold path. Acceptance: a unit test on `VoteHaptics` shows one call at commit.
- **V3: "Vote No" in the unit test.** `VoteTallyTests` asserts only "Vote Yes" on the proposal. Add "Vote No", and assert the same on `HomeVoteCard` and `VoteHoldSheet`.

### #6508 (Home vote card)

- **H1: named actions survive a hold render.** While `model.phase` is `.holding`, `HomeVoteCard.buttons` draws `VoteHoldRow` of `VoteHoldButton`s with no "Vote Yes" / "Vote No". Keep `VotePair` mounted through the hold, or add the same actions to `frozen(...)`.

### #6458 (toast)

- **T1: links and actionless toasts hold under focus.** The countdown holds under VoiceOver focus only when `current.action != nil`. A toast with a link (`monaco-toast-link`) and no action dismisses under focus. Hold it for links too.
- **T2: confirm on a device.** Under VoiceOver an Undo toast stays up until `undo_until` (15 s cap) and the announcement reads "Undo available". Record the result in this page.

### #6421 / the FN ticket for shared haptics

- **V2: duplicate error haptic.** `MonacoToastModifier` plays `Haptics.warning()` for any error toast; `HomeVoteCard:153`, `VoteHoldSheet:183`, `WithdrawScreen:125,148` also call it by hand. If those also show an error toast, the warning plays twice. Remove the hand calls where a toast follows.
- **V4: `Haptics.tap()` behind prominent buttons.** Check `MonacoButtons.swift:355`, `GroupChatParts.swift:330` (chat send) and `CatchUpCardView.swift:176` against the prominent style's own light impact. Remove the double where the style already fires.

### The Home area ticket

- **R1: balance count rolls under Reduce Motion.** `Features/Home/BalancesView.swift:108` uses `.contentTransition(.numericText())` with no Reduce Motion check. Use `reduceMotion ? .identity : .numericText()`.

### The leaderboard area ticket

- **R2: crown and climb bar under Reduce Motion.** Confirm in the `f__15-leaderboard-moment` run that neither animates under `-UIAccessibilityReduceMotionEnabled YES`. If one does, gate it on `accessibilityReduceMotion`.

### The operator

- Run `FAccessibilityAuditUITests`, keep the output, and fill "Audit result" and the per-group tables.
- Do the on-device VoiceOver check for "Vote Yes" and the double-tap-and-hold on all three surfaces.
- Check skeletons for Reduce Motion (not read here).
