# v3 parity review

This page is the result of the screenshot parity review of the 43 UI v3 templates and their 255 states: every app screen was shot on one Debug build of `staging` and set beside its reference at `docs/design/v3/screens/`.

## How it was run

Build: `just build mobile` on `staging` at `1cf2165` (Debug, iPhone 17 Pro simulator, iOS 27.0, headless SimSlim). One simulator, booted with `xcrun simctl boot`, shut down at the end.

    udid="$(scripts/gold-sim-udid.sh)"
    xcrun simctl ui "$udid" appearance light
    MONACO_QA_SCREEN_ONLY='^[a-z]+__[a-z-]+__[a-z-]+$' scripts/qa/screens.sh "$udid" <Debug Monaco.app> .logs/qa/parity/<run>/light
    xcrun simctl ui "$udid" appearance dark
    MONACO_QA_SCREEN_ONLY="$(scripts/qa/parity.py defaults --regex)" scripts/qa/screens.sh "$udid" <Debug Monaco.app> .logs/qa/parity/<run>/dark
    scripts/qa/parity.py sheet --shots .logs/qa/parity/<run> --out .logs/qa/parity/<run>/sheet --png

The light run shot 243 of the 255 states with no failure, no crash and no timeout; the other 12 are `not built` or `no harness` below. Ten of the 243 lines launch the new `ParitySampleHarness` (`-MonacoParitySample`), which draws the real views from fixtures for states no S harness covered; the other 233 launch the S tickets' own harnesses. The dark run shot all 43 defaults. `parity.py sheet` exits 0: every default state has a light and a dark shot. Shots and sheets stay in `.logs/qa/parity/` and are not committed. The three hand spot-checks (home / loss, cabals / cabal / ready, proposals / review-proposal / ready, light and dark) were made against the committed reference renders, which are the prototype's own output.

## Reading the verdicts

- `match`: the same order, tokens, components, copy and states as the reference.
- `match (15pt floor)`: matches, and is taller than the reference only because the app floors captions at 15pt, or differs only by a Context decision (native chart, glass tab bar and status bar, "opens soon" captions, cover gradients, SF Pro Bold wordmark).
- `difference`: the app and the reference disagree in order, tokens, components, copy or states. The row says what each shows. Differences are listed per owning ticket under [Follow-ups](#follow-ups).
- `not built`: the app has no such state. The row says why.
- `no harness`: the state exists in the app but no sample harness can draw it from fixtures.

Dark rows are the 43 default states. Where a dark row says "as light", the dark shot shows the same difference as the light shot and uses the dark tokens correctly (cream and forest swap to the dark canvas, the mint brand fill and the dark identity card); no dark-only difference was found.

Across every screen the app draws initials circles where the reference draws photos (the sample fixtures carry no photo URLs) and the tab bar is the system glass bar. Neither is counted as a difference per row below unless the screen is about the photo.

## Summary

298 rows: the 255 light states and the 43 dark defaults.

| Verdict | Rows |
| --- | ---: |
| match | 0 |
| match (15pt floor) | 110 |
| difference | 176 |
| not built | 8 |
| no harness | 4 |

Template sign-off by the founder (`founder: approved` or `founder: changes`), from the PNG sheets of the 43 defaults:

| Template | Owner | Founder |
| --- | --- | --- |
| Sign in | #5558 (A) | founder: pending |
| Verify code | #5558 (A) | founder: pending |
| Your money | #5560 (A) | founder: pending |
| Home | #5559 (A) | founder: pending |
| Activity | #5560 (A) | founder: pending |
| Account activity | #5562 (A) | founder: pending |
| Deposit | #5561 (A) | founder: pending |
| Card / Apple Pay | #5561 (A) | founder: pending |
| Cash out your share | #5561 (A) | founder: pending |
| Receive USDC | #5561 (A) | founder: pending |
| Deposit to cabal | #5561 (A) | founder: pending |
| Cabal cash | #5562 (A) | founder: pending |
| Choose a network | #5561 (A) | founder: pending |
| Transaction receipt | #5562 (A) | founder: pending |
| Withdraw to wallet | #5561 (A) | founder: pending |
| Proposal & discussion | #5564 (A) | founder: pending |
| All proposals | #5564 (A) | founder: pending |
| Propose · action chooser | #5563 (A) | founder: pending |
| Review proposal · sheet | #5563 (A) | founder: pending |
| Choose stock / holding | #5563 (A) | founder: pending |
| Proposal amount & thesis | #5563 (A) | founder: pending |
| Votes · sheet | #5564 (A) | founder: pending |
| Cabal overview | #5566 (B) | founder: pending |
| Cabal information | #5566 (B) | founder: pending |
| Your cabals | #5565 (B) | founder: pending |
| Cabal chat | #5567 (B) | founder: pending |
| Start a cabal | #5565 (B) | founder: pending |
| Discover cabals | #5565 (B) | founder: pending |
| Cabal asset holding | #5567 (B) | founder: pending |
| Share / open invitation | #5566 (B) | founder: pending |
| Members & voters | #5566 (B) | founder: pending |
| Public cabal | #5566 (B) | founder: pending |
| Stock · token market | #5568 (B) | founder: pending |
| Markets | #5568 (B) | founder: pending |
| Manage watchlist | #5568 (B) | founder: pending |
| Feed | #5569 (B) | founder: pending |
| Post & comments | #5569 (B) | founder: pending |
| Leaderboard | #5570 (B) | founder: pending |
| Edit profile | #5571 (B) | founder: pending |
| Find friends | #5571 (B) | founder: pending |
| Public user profile | #5571 (B) | founder: pending |
| Your profile | #5571 (B) | founder: pending |
| Settings | #5571 (B) | founder: pending |

## Sign in

Template `signin` · group `access` · #5558 (Lane A) · founder: pending

| State | Appearance | Verdict | What differs |
| --- | --- | --- | --- |
| `phone` | light | difference | Send code is drawn disabled on the empty field, the reference draws it enabled; the invite card says "Join a cabal", the reference says "Join a group" Follow-up: signin (#5558). |
| `phone` | dark | difference | as light |
| `email` | light | difference | Send code is drawn disabled on the empty field, the reference draws it enabled; invite card copy "Join a cabal" against "Join a group" Follow-up: signin (#5558). |
| `invalid` | light | difference | Message reads "Enter your full mobile number." where the reference reads "Enter a valid phone number."; Send code drawn disabled, reference enabled Follow-up: signin (#5558). |
| `sending` | light | difference | The segmented control is dimmed while sending, the reference keeps the selected segment solid; the button adds a spinner Follow-up: signin (#5558). |
| `error` | light | difference | The app keeps the headline and the form heading above the "Couldn't load this view" block, the reference replaces the whole screen with it Follow-up: signin (#5558). |

## Verify code

Template `verify` · group `access` · #5558 (Lane A) · founder: pending

| State | Appearance | Verdict | What differs |
| --- | --- | --- | --- |
| `ready` | light | difference | Resend link reads "Send a new code" where the reference reads "Resend code"; Continue is drawn disabled on the empty code, the reference draws it enabled Follow-up: verify (#5558). |
| `ready` | dark | difference | as light |
| `invalid` | light | difference | Message reads "That code didn't work. Check it, or send a new one." where the reference reads "That code didn't match. Check it and try again."; link reads "Send a new code" against "Resend code" Follow-up: verify (#5558). |
| `expired` | light | not built | The app has no separate expired-code state: Privy reports an expired code as the same rejection as a wrong one (LoginFailure.codeRejected), so no scenario can draw "That code expired. Request a new one." |
| `verifying` | light | match (15pt floor) |  |
| `error` | light | match (15pt floor) |  |

## Your money

Template `balances` · group `home` · #5560 (Lane A) · founder: pending

| State | Appearance | Verdict | What differs |
| --- | --- | --- | --- |
| `ready` | light | match (15pt floor) | The harness draws the screen as a root: no back chevron and no tab bar |
| `ready` | dark | match (15pt floor) | as light |
| `cash-only` | light | match (15pt floor) |  |
| `loading` | light | match (15pt floor) |  |
| `error` | light | match (15pt floor) |  |

## Home

Template `home` · group `home` · #5559 (Lane A) · founder: pending

| State | Appearance | Verdict | What differs |
| --- | --- | --- | --- |
| `loss` | light | difference | The proposal card header names the proposer (Jordan) where the reference names the cabal (Big Ballers); the card line says "cabal", the reference "group" Follow-up: home (#5559). |
| `loss` | dark | difference | as light |
| `gain` | light | difference | The proposal card header names the proposer (Jordan) where the reference names the cabal (Big Ballers) Follow-up: home (#5559). |
| `empty` | light | match (15pt floor) |  |
| `cash-only` | light | match (15pt floor) | Cabal cards draw the tint gradient in place of the stock photo (Context decision) |
| `multiple-proposals` | light | difference | The proposal card header names the proposer (Jordan) where the reference names the cabal (Big Ballers) Follow-up: home (#5559). |
| `voted` | light | difference | The proposal card header names the proposer (Jordan) where the reference names the cabal (Big Ballers) Follow-up: home (#5559). |
| `executing` | light | difference | The proposal card header names the proposer (Jordan) where the reference names the cabal (Big Ballers); the card line says "cabal", the reference "group" Follow-up: home (#5559). |
| `completed` | light | difference | The proposal card header names the proposer (Jordan) where the reference names the cabal (Big Ballers); the card line says "cabal", the reference "group" Follow-up: home (#5559). |
| `expired` | light | difference | The proposal card header names the proposer (Jordan) where the reference names the cabal (Big Ballers) Follow-up: home (#5559). |
| `blocked` | light | difference | The proposal card header names the proposer (Jordan) where the reference names the cabal (Big Ballers); the card line says "cabal", the reference "group" Follow-up: home (#5559). |
| `no-proposals` | light | match (15pt floor) |  |
| `proposals-loading` | light | match (15pt floor) |  |
| `proposals-error` | light | match (15pt floor) |  |
| `loading` | light | match (15pt floor) |  |
| `stale` | light | difference | The proposal card header names the proposer (Jordan) where the reference names the cabal (Big Ballers) Follow-up: home (#5559). |
| `error` | light | match (15pt floor) |  |

## Activity

Template `notifications` · group `home` · #5560 (Lane A) · founder: pending

| State | Appearance | Verdict | What differs |
| --- | --- | --- | --- |
| `unread` | light | match (15pt floor) | Avatars are initials circles in place of the fixture photos |
| `unread` | dark | match (15pt floor) | as light |
| `read` | light | match (15pt floor) | Avatars are initials circles in place of the fixture photos |
| `empty` | light | match (15pt floor) |  |
| `loading` | light | match (15pt floor) | The skeleton draws rows only, the reference draws a header block above the rows |
| `error` | light | match (15pt floor) |  |

## Account activity

Template `activity` · group `money` · #5562 (Lane A) · founder: pending

| State | Appearance | Verdict | What differs |
| --- | --- | --- | --- |
| `ready` | light | difference | Rows show absolute dates ("Oct 2 at 23:00") where the reference shows relative ones ("Today", "Yesterday"); cash outflows carry a minus sign the reference leaves off; the cabal row subtitle truncates at 15pt; the note says "Cabal trades" where the reference says "Group trades" Follow-up: activity (#5562). |
| `ready` | dark | difference | as light |
| `pending` | light | difference | Row shows an absolute date where the reference shows "Today"; the note says "Cabal trades" where the reference says "Group trades" Follow-up: activity (#5562). |
| `empty` | light | match (15pt floor) |  |
| `loading` | light | match (15pt floor) |  |
| `error` | light | match (15pt floor) |  |

## Deposit

Template `add-money` · group `money` · #5561 (Lane A) · founder: pending

| State | Appearance | Verdict | What differs |
| --- | --- | --- | --- |
| `ready` | light | match (15pt floor) |  |
| `ready` | dark | match (15pt floor) | as light |
| `unavailable` | light | match (15pt floor) |  |
| `error` | light | difference | The app shows an inline "Couldn't load your balance. Try again" notice above the options, the reference replaces the screen with the "Couldn't load this view" block Follow-up: add-money (#5561). |

## Card / Apple Pay

Template `card-funding` · group `money` · #5561 (Lane A) · founder: pending

| State | Appearance | Verdict | What differs |
| --- | --- | --- | --- |
| `handoff` | light | match (15pt floor) | The button reads "Continue to provider" where the reference placeholder reads "Open provider preview" |
| `handoff` | dark | match (15pt floor) | as light |
| `loading` | light | difference | The app keeps the handoff screen with an "Opening..." button, the reference draws a skeleton Follow-up: card-funding (#5561). |
| `returned` | light | match (15pt floor) |  |
| `cancelled` | light | difference | The body reads "Card purchase didn't go through." where the reference reads "You can return to your account or choose another funding method." Follow-up: card-funding (#5561). |
| `error` | light | not built | CardFundingScreen has handoff, checking, received and cancelled cases only; a card-start failure shows the notice on the Deposit chooser (add-money / unavailable), so no card-funding error screen exists |

## Cash out your share

Template `cash-out` · group `money` · #5561 (Lane A) · founder: pending

| State | Appearance | Verdict | What differs |
| --- | --- | --- | --- |
| `ready` | light | difference | Quick chips read 25% / 50% / All where the reference reads $25 / $50 / $100 / Max; the harness opens on $0 where the reference shows $250.00 typed Follow-up: cash-out (#5561). |
| `ready` | dark | difference | as light |
| `insufficient` | light | difference | The app shows "$8,000" in the error red with no "current share value" overage message and quick chips 25% / 50% / All; the reference shows "$8,000.00", the message "The request exceeds your current share value." and chips $25 / $50 / $100 / Max Follow-up: cash-out (#5561). |
| `review` | light | difference | The harness draws "Requested amount $0.00" with Request cash-out disabled where the reference shows $250.00 and an enabled button (the amount argument 250 is not applied) Follow-up: cash-out (#5561). |
| `processing` | light | match (15pt floor) | The note says "the cabal's holdings" where the reference says "the group's" |
| `completed` | light | match (15pt floor) |  |
| `error` | light | match (15pt floor) |  |

## Receive USDC

Template `deposit` · group `money` · #5561 (Lane A) · founder: pending

| State | Appearance | Verdict | What differs |
| --- | --- | --- | --- |
| `address` | light | difference | "How it works" lists two titles ("Send USDC on Solana", "It lands in your balance") where the reference lists three steps with descriptions; the notice says "Transfers on other networks can't be recovered" where the reference placeholder says "The live app supplies the exact address"; the app draws the real QR and address, the reference a preview Follow-up: deposit (#5561). |
| `address` | dark | difference | as light |
| `copied` | light | no harness | The copy confirmation is the "Address copied." toast (DepositView.copy), which sits over the screen for a moment and has no scenario that holds it; the deposit-address shot is the underlying screen |
| `waiting` | light | difference | "How it works" lists two titles where the reference lists three steps with descriptions; the app draws the real QR and address Follow-up: deposit (#5561). |
| `received` | light | match (15pt floor) |  |
| `error` | light | difference | The app keeps the address screen and adds a "Couldn't load your balance. Try again" notice, the reference replaces the screen with the "Couldn't load this view" block Follow-up: deposit (#5561). |

## Deposit to cabal

Template `fund-cabal` · group `money` · #5561 (Lane A) · founder: pending

| State | Appearance | Verdict | What differs |
| --- | --- | --- | --- |
| `ready` | light | difference | The amount reads "$0" where the reference shows "$250.00" typed; the app adds a "Your cabal earns yield on its USDC" notice and the button reads "Enter an amount" Follow-up: fund-cabal (#5561). |
| `ready` | dark | difference | as light |
| `insufficient` | light | difference | The amount reads "$750" in red where the reference reads "$750.00" in ink; the app adds a "Deposit to account" secondary button and the Deposit button is disabled with its amount Follow-up: fund-cabal (#5561). |
| `submitting` | light | difference | The amount reads "$250" where the reference reads "$250.00"; the app adds the yield notice under the "Processing your deposit..." notice Follow-up: fund-cabal (#5561). |
| `completed` | light | difference | The confirmation sheet adds an expand arrow beside the close button; the amount reads "$250" where the reference reads "$250.00"; the app adds a "Your cabal earns yield on its USDC" notice Follow-up: fund-cabal (#5561). |
| `error` | light | match (15pt floor) |  |

## Cabal cash

Template `group-cash` · group `money` · #5562 (Lane A) · founder: pending

| State | Appearance | Verdict | What differs |
| --- | --- | --- | --- |
| `ready` | light | match (15pt floor) |  |
| `ready` | dark | match (15pt floor) | as light |
| `unavailable` | light | match (15pt floor) |  |
| `loading` | light | match (15pt floor) |  |
| `error` | light | match (15pt floor) |  |

## Choose a network

Template `network` · group `money` · #5561 (Lane A) · founder: pending

| State | Appearance | Verdict | What differs |
| --- | --- | --- | --- |
| `ready` | light | difference | Network marks are the real chain logos where the reference draws letter tiles; the subtitles read "Arrives in a few minutes" where the reference reads "Route and timing confirmed by provider" Follow-up: network (#5561). |
| `ready` | dark | difference | as light |
| `selected` | light | not built | DepositNetworksView has no selected-row state; choosing a network pushes its address screen |
| `loading` | light | not built | DepositNetworksView takes a ready list of networks and has no loading state |
| `unavailable` | light | not built | DepositNetworksView has no unavailable notice or state |

## Transaction receipt

Template `transaction` · group `money` · #5562 (Lane A) · founder: pending

| State | Appearance | Verdict | What differs |
| --- | --- | --- | --- |
| `completed` | light | match (15pt floor) | The harness draws a deposit receipt, the reference a buy receipt; the layout is the same |
| `completed` | dark | match (15pt floor) | as light |
| `pending` | light | match (15pt floor) | The harness draws a deposit receipt, the reference a buy receipt; the layout is the same |
| `failed` | light | match (15pt floor) | The harness draws a deposit receipt, the reference a buy receipt; the layout is the same |
| `unknown` | light | match (15pt floor) | The harness draws a deposit receipt, the reference a buy receipt; the layout is the same |
| `error` | light | match (15pt floor) |  |

## Withdraw to wallet

Template `withdraw` · group `money` · #5561 (Lane A) · founder: pending

| State | Appearance | Verdict | What differs |
| --- | --- | --- | --- |
| `ready` | light | difference | The amount field is focused with the system keyboard and a Paste accessory covering the form; the amount reads "$0" without decimals and the quick chips are a single "Max" Follow-up: withdraw (#5561). |
| `ready` | dark | difference | as light |
| `invalid` | light | difference | The amount field is focused and the system keyboard covers the form; the amount reads "$100" without decimals and the quick chips are a single "Max"; the destination error is not visible above the keyboard Follow-up: withdraw (#5561). |
| `insufficient` | light | difference | The amount field is focused and the system keyboard covers the form; the amount reads "$700" without decimals and the quick chips are a single "Max" where the reference shows $25 / $50 / $100 / Max Follow-up: withdraw (#5561). |
| `review` | light | difference | The destination shows the full address where the reference shows a masked example wallet; the app adds "Arrives" and "Network costs $0.12" rows and its button reads "Withdraw" where the reference placeholder reads "Simulate withdrawal request" Follow-up: withdraw (#5561). |
| `processing` | light | difference | The destination row shows the full address where the reference shows a masked example wallet Follow-up: withdraw (#5561). |
| `completed` | light | difference | The destination row shows the full address where the reference shows a masked example wallet Follow-up: withdraw (#5561). |
| `error` | light | match (15pt floor) |  |

## Proposal & discussion

Template `proposal` · group `proposals` · #5564 (Lane A) · founder: pending

| State | Appearance | Verdict | What differs |
| --- | --- | --- | --- |
| `open` | light | difference | The symbol tile is neutral grey where the reference draws a lime tile; the vote summary is one line ("2 yes · 0 no · 1 not voted") where the reference labels the bar ends; the app adds an Expected section and the Vote No / Vote Yes bar sits over the content Follow-up: proposal (#5564). |
| `open` | dark | difference | as light |
| `sell` | light | difference | The sell headline reads "1.5 shares / About $25.00" where the reference reads "$400.00"; The symbol tile is neutral grey where the reference draws a lime tile; the vote summary is one line ("2 yes · 0 no · 1 not voted") where the reference labels the bar ends Follow-up: proposal (#5564). |
| `voted` | light | difference | The symbol tile is neutral grey where the reference draws a lime tile; the vote summary is one line ("2 yes · 0 no · 1 not voted") where the reference labels the bar ends; the Change vote action is a full-width button where the reference shows it beside the Discussion heading Follow-up: proposal (#5564). |
| `not-voter` | light | difference | The symbol tile is neutral grey where the reference draws a lime tile; the vote summary is one line ("2 yes · 0 no · 1 not voted") where the reference labels the bar ends; the app adds an Expected section Follow-up: proposal (#5564). |
| `approved` | light | difference | The symbol tile is neutral grey (GOOGL) where the reference draws a lime tile; the vote summary is one line ("2 yes · 0 no · 1 not voted") where the reference labels the bar ends Follow-up: proposal (#5564). |
| `executed` | light | difference | The symbol tile is neutral grey where the reference draws a lime tile; the vote summary is one line where the reference labels the bar ends Follow-up: proposal (#5564). |
| `expired` | light | difference | The symbol tile is neutral grey where the reference draws a lime tile; the vote summary is one line ("2 yes · 0 no · 1 not voted") where the reference labels the bar ends Follow-up: proposal (#5564). |
| `blocked` | light | difference | The symbol tile is neutral grey where the reference draws a lime tile; the vote summary is one line where the reference labels the bar ends Follow-up: proposal (#5564). |
| `rejected` | light | difference | The symbol tile is neutral grey where the reference draws a lime tile; the vote summary is one line ("2 yes · 0 no · 1 not voted") where the reference labels the bar ends Follow-up: proposal (#5564). |
| `error` | light | difference | The app keeps the Discussion section and adds "Couldn't load comments" under the error block, and its title reads "Proposal" where the reference title is the proposal's name Follow-up: proposal (#5564). |

## All proposals

Template `proposals` · group `proposals` · #5564 (Lane A) · founder: pending

| State | Appearance | Verdict | What differs |
| --- | --- | --- | --- |
| `open` | light | difference | Both rows are full cards with the proposer name above the title; the reference draws the newest as a card headed "Mia proposed NVIDIA" with a reply count and the older ones as plain rows; symbol tile is neutral grey Follow-up: proposals (#5564). |
| `open` | dark | difference | as light |
| `closed` | light | difference | Symbol tiles are neutral grey where the reference tints them (lime, sand, lilac); row subtitles truncate at 15pt Follow-up: proposals (#5564). |
| `empty` | light | match (15pt floor) |  |
| `loading` | light | match (15pt floor) |  |
| `error` | light | difference | The error block sits on a white band that cuts across the cream canvas above and below it; the reference draws it on the canvas Follow-up: proposals (#5564). |

## Propose · action chooser

Template `propose` · group `proposals` · #5563 (Lane A) · founder: pending

| State | Appearance | Verdict | What differs |
| --- | --- | --- | --- |
| `ready` | light | difference | The sheet is drawn over a blank grey screen, the reference draws it over the cabal screen Follow-up: propose (#5563). |
| `ready` | dark | difference | as light |
| `no-holdings` | light | difference | The sheet is drawn over a blank grey screen, the reference draws it over the cabal screen Follow-up: propose (#5563). |
| `not-member` | light | difference | The sheet is drawn over a blank grey screen, the reference draws it over the cabal screen Follow-up: propose (#5563). |

## Review proposal · sheet

Template `review-proposal` · group `proposals` · #5563 (Lane A) · founder: pending

| State | Appearance | Verdict | What differs |
| --- | --- | --- | --- |
| `ready` | light | difference | The sheet is drawn over a blank grey screen, the reference draws it over the stock screen; the app adds "Everyone agrees" and "Pot 25% of $100.00" rows under the key rows; the symbol tile is neutral grey Follow-up: review-proposal (#5563). |
| `ready` | dark | difference | as light |
| `sell` | light | difference | The sheet is drawn over a blank grey screen, the reference draws it over the stock screen; the app adds "Everyone agrees" and "Raises about $21.00" rows and states the amount in shares ("0.073 shares") where the reference states "$400.00" Follow-up: review-proposal (#5563). |
| `long-thesis` | light | difference | The sheet is drawn over a blank grey screen; the thesis is clipped by the footer where the reference shows the whole thesis with the keys beneath; the symbol tile is neutral grey Follow-up: review-proposal (#5563). |
| `expanded` | light | no harness | The large detent is reached with the expand button inside ProposeReviewSheet; no scenario presents the sheet expanded |
| `submitting` | light | difference | The sheet is drawn over a blank grey screen, the reference draws it over the stock screen; the app adds "Everyone agrees" and "Pot 25% of $100.00" rows Follow-up: review-proposal (#5563). |
| `sent` | light | difference | The sheet is drawn over a blank grey screen, the reference draws it over the stock screen Follow-up: review-proposal (#5563). |
| `error` | light | difference | The sheet is drawn over a blank grey screen; the error reads "Couldn't send your proposal. Your draft is saved here. Something went wrong. Try again." where the reference reads "Couldn't send your proposal. Your draft is saved here." Follow-up: review-proposal (#5563). |

## Choose stock / holding

Template `select-stock` · group `proposals` · #5563 (Lane A) · founder: pending

| State | Appearance | Verdict | What differs |
| --- | --- | --- | --- |
| `buy` | light | difference | Symbol tiles are neutral grey where the reference tints them; the list shows the fixture's three stocks Follow-up: select-stock (#5563). |
| `buy` | dark | difference | as light |
| `sell` | light | difference | Symbol tile is neutral grey; the row subtitle reads "0.73 shares" where the reference reads "30.0000 units"; the cabal heading above the list is not drawn Follow-up: select-stock (#5563). |
| `no-results` | light | match (15pt floor) |  |
| `no-holdings` | light | difference | The search field and the Volume / All controls are not drawn above the empty state, the reference draws them Follow-up: select-stock (#5563). |
| `loading` | light | match (15pt floor) | The title reads "Buy stock" where the reference reads "Choose stock / holding" |
| `error` | light | difference | The title reads "Buy stock" where the reference reads "Choose stock / holding"; the message reads "Check your connection and try again." where the reference reads "Your existing information hasn't been changed. Try again when your connection is available." Follow-up: select-stock (#5563). |

## Proposal amount & thesis

Template `trade-amount` · group `proposals` · #5563 (Lane A) · founder: pending

| State | Appearance | Verdict | What differs |
| --- | --- | --- | --- |
| `ready` | light | difference | The amount field is focused with the system keyboard covering the thesis and the error; the amount reads without decimals ("$25") where the reference reads "$400.00"; the symbol tile is neutral grey; the app adds a "The cabal can spend $100.00" line Follow-up: trade-amount (#5563). |
| `ready` | dark | difference | as light |
| `sell` | light | difference | The amount field is focused with the system keyboard covering the thesis; the amount reads "$0" where the reference shows "$400.00"; quick chips are 25% / 50% / All where the reference shows $25 / $50 / $100 / Max; the symbol tile is neutral grey Follow-up: trade-amount (#5563). |
| `insufficient` | light | difference | The amount field is focused with the system keyboard covering the thesis and the error; the amount reads without decimals ("$25") where the reference reads "$400.00"; the symbol tile is neutral grey; the app adds a "The cabal can spend $100.00" line Follow-up: trade-amount (#5563). |
| `empty-thesis` | light | difference | The amount field is focused with the system keyboard covering the thesis and the error; the amount reads without decimals ("$25") where the reference reads "$400.00"; the symbol tile is neutral grey; the app adds a "The cabal can spend $100.00" line Follow-up: trade-amount (#5563). |
| `error` | light | difference | The title reads "Buy Alphabet" where the reference reads "Proposal amount & thesis"; the message reads "Check your connection and try again." where the reference reads "Your existing information hasn't been changed. Try again when your connection is available." Follow-up: trade-amount (#5563). |

## Votes · sheet

Template `votes` · group `proposals` · #5564 (Lane A) · founder: pending

| State | Appearance | Verdict | What differs |
| --- | --- | --- | --- |
| `open` | light | difference | The sheet has only a close button where the reference also draws an expand / collapse arrow; voter avatars are initials circles Follow-up: votes (#5564). |
| `open` | dark | difference | as light |
| `recorded` | light | difference | The sheet has only a close button where the reference also draws an expand / collapse arrow; voter avatars are initials circles Follow-up: votes (#5564). |
| `closed` | light | difference | The sheet has only a close button where the reference also draws an expand / collapse arrow; voter avatars are initials circles Follow-up: votes (#5564). |
| `expired` | light | difference | The sheet has only a close button where the reference also draws an expand / collapse arrow; voter avatars are initials circles Follow-up: votes (#5564). |
| `rejected` | light | difference | The sheet has only a close button where the reference also draws an expand / collapse arrow; voter avatars are initials circles Follow-up: votes (#5564). |
| `empty` | light | difference | The sheet has only a close button where the reference also draws an expand / collapse arrow; voter avatars are initials circles Follow-up: votes (#5564). |
| `expanded` | light | difference | The sheet has only a close button where the reference also draws an expand / collapse arrow; voter avatars are initials circles Follow-up: votes (#5564). |
| `loading` | light | difference | The sheet has only a close button where the reference also draws an expand / collapse arrow; voter avatars are initials circles; the proposal beneath is a skeleton where the reference keeps the proposal visible Follow-up: votes (#5564). |
| `error` | light | difference | The sheet has only a close button where the reference also draws an expand / collapse arrow; voter avatars are initials circles; the proposal beneath shows its own full-screen error where the reference keeps the proposal visible Follow-up: votes (#5564). |

## Cabal overview

Template `cabal` · group `cabals` · #5566 (Lane B) · founder: pending

| State | Appearance | Verdict | What differs |
| --- | --- | --- | --- |
| `ready` | light | match (15pt floor) | Cover draws the tint gradient in place of the stock photo (Context decision); the fixture chart is flat where the reference chart falls |
| `ready` | dark | match (15pt floor) | as light |
| `no-share` | light | match (15pt floor) | Cover draws the tint gradient in place of the stock photo (Context decision); the fixture chart is flat |
| `cash-only` | light | match (15pt floor) | Cover draws the tint gradient in place of the stock photo (Context decision) |
| `no-proposals` | light | match (15pt floor) | Cover draws the tint gradient in place of the stock photo (Context decision); the fixture chart is flat |
| `voted` | light | match (15pt floor) | The first screen matches the ready state; the vote card sits below the fold in both |
| `executing` | light | match (15pt floor) | The harness shoots the middle of the page; the Proposals, Holdings and Member standings sections match the reference in order |
| `completed` | light | match (15pt floor) | The harness shoots the middle of the page; the Proposals, Holdings and Member standings sections match the reference in order |
| `loading` | light | match (15pt floor) | The skeleton keeps the "Your share" and "Cash" labels, the reference skeleton has none |
| `stale` | light | not built | The cabal screen has no stale-prices notice (CabalScreen only draws a staleness notice on Home and the cabal holding screen), so no scenario can draw "Prices delayed" |
| `error` | light | match (15pt floor) |  |

## Cabal information

Template `cabal-info` · group `cabals` · #5566 (Lane B) · founder: pending

| State | Appearance | Verdict | What differs |
| --- | --- | --- | --- |
| `member` | light | difference | The app adds a "Cabal treasury" section below the actions where the reference ends with a "View your share" button Follow-up: cabal-info (#5566). |
| `member` | dark | difference | as light |
| `admin` | light | difference | The Rules section carries an "Edit" link the reference does not draw Follow-up: cabal-info (#5566). |
| `saving` | light | no harness | The existing infoSaving scenario draws the same screen as admin: the "Saving display preferences..." notice is not drawn by CabalInfoView(savingPreview:), so no scenario shows the saving state |
| `error` | light | match (15pt floor) |  |

## Your cabals

Template `cabals` · group `cabals` · #5565 (Lane B) · founder: pending

| State | Appearance | Verdict | What differs |
| --- | --- | --- | --- |
| `ready` | light | match (15pt floor) | Cabal cards draw the tint gradient in place of the stock photo and "Return opens soon" is the missing-data caption (Context decisions); the card count sits beside the "Your cabals" heading where the reference right-aligns it |
| `ready` | dark | match (15pt floor) | as light |
| `no-action` | light | match (15pt floor) | Cabal cards draw the tint gradient in place of the stock photo and "Return opens soon" is the missing-data caption (Context decisions); the card count sits beside the "Your cabals" heading where the reference right-aligns it |
| `empty` | light | difference | The empty state adds an "Explore cabals" secondary button the reference does not draw Follow-up: cabals (#5565). |
| `filtered` | light | match (15pt floor) | The harness scrolls to the bottom, so the shot shows the leaderboard where the reference shows the top of the list |
| `loading` | light | difference | The app draws the Cabals title, the search field and the leaderboard header above the skeleton; the reference draws a skeleton for the whole page Follow-up: cabals (#5565). |
| `error` | light | difference | The app keeps the Cabals title, the search field and the Cabal leaderboard around the error block; the reference replaces the whole page with it Follow-up: cabals (#5565). |
| `no-share` | light | match (15pt floor) | Cabal cards draw the tint gradient in place of the stock photo and "Return opens soon" is the missing-data caption (Context decisions); the card count sits beside the "Your cabals" heading where the reference right-aligns it |
| `pnl-unavailable` | light | match (15pt floor) | Cabal cards draw the tint gradient in place of the stock photo and "Return opens soon" is the missing-data caption (Context decisions); the card count sits beside the "Your cabals" heading where the reference right-aligns it |
| `rank-loading` | light | match (15pt floor) | The harness scrolls to the bottom: three skeleton rows under the filter row; the reference top of page matches in order |
| `rank-error` | light | match (15pt floor) | The harness scrolls to the bottom: "Rankings couldn't load." with Retry rankings under the filter row; the reference top of page matches in order |
| `rank-empty` | light | match (15pt floor) | The harness scrolls to the bottom: the leaderboard shows "No public rankings for this period." under the filter row; the reference top of page matches in order |

## Cabal chat

Template `chat` · group `cabals` · #5567 (Lane B) · founder: pending

| State | Appearance | Verdict | What differs |
| --- | --- | --- | --- |
| `ready` | light | match (15pt floor) | Voter avatars are initials circles in place of the fixture photos; the proposal card sits mid-thread |
| `ready` | dark | match (15pt floor) | as light |
| `empty` | light | match (15pt floor) |  |
| `sending` | light | match (15pt floor) |  |
| `failed` | light | match (15pt floor) | The app scrolls to the newest messages and adds an offline toast over the composer |
| `offline` | light | match (15pt floor) | Voter avatars are initials circles in place of the fixture photos |

## Start a cabal

Template `create-cabal` · group `cabals` · #5565 (Lane B) · founder: pending

| State | Appearance | Verdict | What differs |
| --- | --- | --- | --- |
| `ready` | light | difference | The cover image slot starts empty ("Add a cover photo") where the reference shows a chosen cover with a check; Create cabal is pinned to the bottom where the reference draws it in the flow Follow-up: create-cabal (#5565). |
| `ready` | dark | difference | as light |
| `invalid` | light | match (15pt floor) | The harness shows the over-length message "Use 40 characters or fewer." where the reference shows "Give your cabal a name." |
| `selected-voters` | light | match (15pt floor) |  |
| `creating` | light | difference | The app keeps the form under a "Creating your cabal" header with a spinner button, the reference replaces the form content with a header and the greyed form below Follow-up: create-cabal (#5565). |
| `error` | light | no harness | The existing createCabal error scenario draws the same form as ready: the create failure is a toast that has gone by the shot, so no scenario shows the "Couldn't load this view" block |

## Discover cabals

Template `discover` · group `cabals` · #5565 (Lane B) · founder: pending

| State | Appearance | Verdict | What differs |
| --- | --- | --- | --- |
| `results` | light | difference | Rows carry a Request / Joined chip beside the chevron where the reference draws plain rows; avatars are initials squares in place of the fixture photos Follow-up: discover (#5565). |
| `results` | dark | difference | as light |
| `no-results` | light | match (15pt floor) |  |
| `request-pending` | light | difference | The app adds a "Your request to join Weekend pot is pending." notice and a "Request pending" chip; the reference draws the plain list Follow-up: discover (#5565). |
| `loading` | light | difference | The app keeps the "Find a cabal" heading and the search field above the skeleton; the reference draws a skeleton for the whole page Follow-up: discover (#5565). |
| `error` | light | difference | The app keeps the "Find a cabal" heading and the search field above the error block; the reference replaces the whole page with it Follow-up: discover (#5565). |

## Cabal asset holding

Template `holding` · group `cabals` · #5567 (Lane B) · founder: pending

| State | Appearance | Verdict | What differs |
| --- | --- | --- | --- |
| `ready` | light | difference | The symbol tile is neutral grey where the reference draws a lime tile; the chart axis dates carry a time ("Sep 19 at 10:49") where the reference shows "Sep 18" Follow-up: holding (#5567). |
| `ready` | dark | difference | as light |
| `loss` | light | difference | The symbol tile is neutral grey where the reference draws a lime tile; the chart axis dates carry a time ("Sep 19 at 10:49") where the reference shows "Sep 18" Follow-up: holding (#5567). |
| `empty` | light | difference | The symbol tile is neutral grey where the reference draws a lime tile; the P&L caption reads "Position P&L · All time" where the reference reads "Position P&L · 1M" Follow-up: holding (#5567). |
| `loading` | light | match (15pt floor) |  |
| `stale` | light | difference | The symbol tile is neutral grey where the reference draws a lime tile; the chart axis dates carry a time ("Sep 19 at 10:49") where the reference shows "Sep 18"; the stale notice wraps to two lines at 15pt Follow-up: holding (#5567). |
| `error` | light | match (15pt floor) |  |

## Share / open invitation

Template `invite` · group `cabals` · #5566 (Lane B) · founder: pending

| State | Appearance | Verdict | What differs |
| --- | --- | --- | --- |
| `share` | light | match (15pt floor) | The app adds a "Cabal links open soon" caption under the link (missing-data caption) |
| `share` | dark | match (15pt floor) | as light |
| `preview` | light | match (15pt floor) |  |
| `invalid` | light | match (15pt floor) |  |
| `expired` | light | not built | The invite sheet has four modes (share, invited, requested, unavailable); an expired link resolves to unavailable, so no scenario draws a separate expired state |
| `pending` | light | match (15pt floor) |  |

## Members & voters

Template `members` · group `cabals` · #5566 (Lane B) · founder: pending

| State | Appearance | Verdict | What differs |
| --- | --- | --- | --- |
| `members` | light | difference | The control has two segments (Members, Voters) for a member viewer where the reference draws three (Members, Voters, Requests); avatars are initials circles Follow-up: members (#5566). |
| `members` | dark | difference | as light |
| `choose-voters` | light | difference | The app draws a Voters tab with a "Voter" chip per row; the reference opens an "Eligible voters" sheet over the screen with a check per member and a Save voters button Follow-up: members (#5566). |
| `requests` | light | match (15pt floor) | Avatars are initials circles; the reference's "Administrator preview" notice is a prototype caption |
| `empty` | light | match (15pt floor) | The empty message reads "New membership requests will appear here." where the reference adds "for permitted administrators." |
| `error` | light | match (15pt floor) |  |

## Public cabal

Template `public-cabal` · group `cabals` · #5566 (Lane B) · founder: pending

| State | Appearance | Verdict | What differs |
| --- | --- | --- | --- |
| `preview` | light | difference | There is no About section between the return and Membership (the reference draws the cabal description); the return block shows the "Returns open soon" caption (missing-data caption); the app adds an "Approval required: the creator reviews each request." note above the button Follow-up: public-cabal (#5566). |
| `preview` | dark | difference | as light |
| `pending` | light | difference | There is no About section between the return and Membership (the reference draws the cabal description); the return block shows the "Returns open soon" caption (missing-data caption); the app adds a "Cancel request" link under the disabled button Follow-up: public-cabal (#5566). |
| `member` | light | difference | There is no About section between the return and Membership (the reference draws the cabal description); the return block shows the "Returns open soon" caption (missing-data caption) Follow-up: public-cabal (#5566). |
| `closed` | light | not built | PublicCabalView has no closed-to-requests state: the join mode on the wire is open or request, and CabalJoinSection has no "Not accepting requests" button |
| `private` | light | match (15pt floor) |  |
| `error` | light | match (15pt floor) |  |

## Stock · token market

Template `instrument` · group `markets` · #5568 (Lane B) · founder: pending

| State | Appearance | Verdict | What differs |
| --- | --- | --- | --- |
| `ready` | light | difference | Native candles, the interval picker and the chart-style toggle replace the reference line chart (Context decision); the symbol tile is neutral grey where the reference draws a lime tile; the app adds a "Market open" pill and a "Propose a sell" button; the watch star is empty where the reference draws it filled Follow-up: instrument (#5568). |
| `ready` | dark | difference | as light |
| `loss` | light | difference | Native candles, the interval picker and the chart-style toggle replace the reference line chart (Context decision); the symbol tile is neutral grey where the reference draws a lime tile; the app adds a "Market open" pill and a "Propose a sell" button; the watch star is empty where the reference draws it filled; the loss figures read -4.44% where the reference reads -4.18% (fixture numbers) Follow-up: instrument (#5568). |
| `no-holders` | light | difference | Native candles, the interval picker and the chart-style toggle replace the reference line chart (Context decision); the symbol tile is neutral grey where the reference draws a lime tile; the app adds a "Market open" pill and a "Propose a sell" button; the watch star is empty where the reference draws it filled Follow-up: instrument (#5568). |
| `no-proposals` | light | difference | Native candles, the interval picker and the chart-style toggle replace the reference line chart (Context decision); the symbol tile is neutral grey where the reference draws a lime tile; the app adds a "Market open" pill and a "Propose a sell" button; the watch star is empty where the reference draws it filled Follow-up: instrument (#5568). |
| `loading` | light | match (15pt floor) |  |
| `stale` | light | difference | Native candles, the interval picker and the chart-style toggle replace the reference line chart (Context decision); the symbol tile is neutral grey where the reference draws a lime tile; the app adds a "Market open" pill and a "Propose a sell" button; the watch star is empty where the reference draws it filled; the stale notice reads "Token price delayed" where the reference matches, and it sits above the price in both Follow-up: instrument (#5568). |
| `unavailable` | light | difference | Native candles, the interval picker and the chart-style toggle replace the reference line chart (Context decision); the symbol tile is neutral grey where the reference draws a lime tile; the app adds a "Market open" pill and a "Propose a sell" button; the watch star is empty where the reference draws it filled; the unavailable notice wraps to two lines at 15pt Follow-up: instrument (#5568). |

## Markets

Template `markets` · group `markets` · #5568 (Lane B) · founder: pending

| State | Appearance | Verdict | What differs |
| --- | --- | --- | --- |
| `ready` | light | difference | Symbol tiles are neutral grey where the reference tints them (lime, lilac, sand); the mover cards show "Opens soon" where the reference shows the 24h volume figure (missing-data caption) Follow-up: markets (#5568). |
| `ready` | dark | difference | as light |
| `watchlist-empty` | light | difference | Symbol tiles are neutral grey where the reference tints them (lime, lilac, sand); the mover cards show "Opens soon" where the reference shows the 24h volume figure (missing-data caption); the Popular now rows show price and change where the reference shows 24h volume and trade counts Follow-up: markets (#5568). |
| `no-results` | light | difference | The sort control reads "Default" where the reference reads "Volume" Follow-up: markets (#5568). |
| `loading` | light | difference | The app keeps the Markets title above the skeleton, the reference draws a skeleton for the whole page Follow-up: markets (#5568). |
| `stale` | light | difference | Symbol tiles are neutral grey where the reference tints them (lime, lilac, sand); the mover cards show "Opens soon" where the reference shows the 24h volume figure (missing-data caption); the notice reads "Prices delayed · just now." where the reference reads "Prices delayed · 14 minutes ago."; the Popular now rows show price and change where the reference shows 24h volume Follow-up: markets (#5568). |
| `error` | light | match (15pt floor) | The app keeps the Markets title above the error block, the reference replaces the page with it |

## Manage watchlist

Template `watchlist` · group `markets` · #5568 (Lane B) · founder: pending

| State | Appearance | Verdict | What differs |
| --- | --- | --- | --- |
| `ready` | light | difference | Symbol tiles are neutral grey where the reference tints them; the "3 stocks selected" caption is larger at the 15pt floor Follow-up: watchlist (#5568). |
| `ready` | dark | difference | as light |
| `empty` | light | difference | The app keeps all rows with empty stars below the empty state where the reference draws three filled ones; the caption reads "No stocks selected" where the reference reads "3 stocks selected"; the title reads "Your watchlist" where the reference reads the same on empty and "Manage watchlist" on error Follow-up: watchlist (#5568). |
| `no-results` | light | match (15pt floor) | The caption reads "No stocks selected" where the reference reads "3 stocks selected" |
| `loading` | light | difference | The title reads "Your watchlist" where the reference reads "Manage watchlist"; the app keeps the search field and caption above the skeleton, the reference draws a skeleton for the whole page Follow-up: watchlist (#5568). |
| `error` | light | difference | The title reads "Your watchlist" where the reference reads "Manage watchlist"; the message reads "Check your connection..." copy from the shared view, matching the reference Follow-up: watchlist (#5568). |

## Feed

Template `feed` · group `feed` · #5569 (Lane B) · founder: pending

| State | Appearance | Verdict | What differs |
| --- | --- | --- | --- |
| `all` | light | difference | Cards carry a like count where the reference shows the like icon with a count too, but the avatar is an initials circle where the reference shows a photo; the proposal card text clips mid-sentence at 15pt; the search field and the filter menu glyph differ (filter lines against the sliders icon) Follow-up: feed (#5569). |
| `all` | dark | difference | as light |
| `proposals` | light | difference | Avatars are initials circles in place of the fixture photos; the end-of-list line reads "No more matching updates" where the reference reads "You're caught up"; the filter glyph is lines where the reference draws sliders; the Propose sell card shows -2.4% where the reference shows +4.9% (fixture numbers) Follow-up: feed (#5569). |
| `trades` | light | difference | Avatars are initials circles in place of the fixture photos; the end-of-list line reads "No more matching updates" where the reference reads "You're caught up"; the filter glyph is lines where the reference draws sliders Follow-up: feed (#5569). |
| `price-moves` | light | difference | Avatars are initials circles in place of the fixture photos; the end-of-list line reads "No more matching updates" where the reference reads "You're caught up"; the filter glyph is lines where the reference draws sliders Follow-up: feed (#5569). |
| `following` | light | difference | The shot is the follows-nobody empty state where the reference shows the Following feed with posts; no scenario selects the Following scope over the fixture items Follow-up: feed (#5569). |
| `pnl-private` | light | difference | Avatars are initials circles in place of the fixture photos; the cards read the same "P&L private" tags as the reference Follow-up: feed (#5569). |
| `empty` | light | match (15pt floor) |  |
| `loading` | light | difference | The app keeps the Feed title and the search field above the skeleton, the reference draws a skeleton for the whole page Follow-up: feed (#5569). |
| `load-more-error` | light | difference | The harness scrolls to the bottom, so the shot shows "Older updates couldn't load" with a Retry older updates button; the reference shows the top of the feed; avatars are initials circles Follow-up: feed (#5569). |
| `error` | light | difference | The app keeps the Feed title, the search field and the toolbar buttons above the error block; the reference replaces the page with it Follow-up: feed (#5569). |

## Post & comments

Template `post` · group `feed` · #5569 (Lane B) · founder: pending

| State | Appearance | Verdict | What differs |
| --- | --- | --- | --- |
| `ready` | light | match (15pt floor) | Avatars are initials circles and the symbol tile is neutral grey where the reference shows photos and a lime tile |
| `ready` | dark | match (15pt floor) | as light |
| `empty` | light | match (15pt floor) | Avatars are initials circles and the symbol tile is neutral grey where the reference shows photos and a lime tile |
| `replying` | light | difference | The system dictation prompt ("Speed up your typing by sliding your finger across the letters") covers the lower half of the shot; the "Replying to Mia" bar sits above the field as the reference's reply row does not Follow-up: post (#5569). |
| `sending` | light | match (15pt floor) | The app adds a "Sending..." notice above the field and a spinner on the send button; Avatars are initials circles and the symbol tile is neutral grey where the reference shows photos and a lime tile |
| `failed` | light | difference | The offline toast covers the composer and the "Your reply wasn't sent" notice is cut off by it; Avatars are initials circles and the symbol tile is neutral grey where the reference shows photos and a lime tile Follow-up: post (#5569). |
| `unavailable` | light | match (15pt floor) |  |

## Leaderboard

Template `leaderboard` · group `explore` · #5570 (Lane B) · founder: pending

| State | Appearance | Verdict | What differs |
| --- | --- | --- | --- |
| `cabals` | light | difference | Rows draw initials tiles in place of the fixture cover and profile photos; the leaderboard is reached from the Cabals tab, so the tab bar highlights Cabals where the reference highlights Home; the caption wraps at 15pt and the board and range controls use chevrons where the reference uses arrows; the app adds a crown on rank 1 and a trailing "All 5 eligible cabals shown" line Follow-up: leaderboard (#5570). |
| `cabals` | dark | difference | as light |
| `people` | light | difference | Rows draw initials tiles in place of the fixture cover and profile photos; the leaderboard is reached from the Cabals tab, so the tab bar highlights Cabals where the reference highlights Home; the caption wraps at 15pt and the board and range controls use chevrons where the reference uses arrows; the fixture people differ from the reference (Ana Kovacs, Marcus Hale against Mia, Leo) Follow-up: leaderboard (#5570). |
| `filtered` | light | difference | Rows draw initials tiles in place of the fixture cover and profile photos; the leaderboard is reached from the Cabals tab, so the tab bar highlights Cabals where the reference highlights Home; the caption wraps at 15pt and the board and range controls use chevrons where the reference uses arrows; the shot lists three cabals under the filter where the reference lists five Follow-up: leaderboard (#5570). |
| `empty` | light | match (15pt floor) | Rows draw initials tiles in place of the fixture cover and profile photos; the leaderboard is reached from the Cabals tab, so the tab bar highlights Cabals where the reference highlights Home; the caption wraps at 15pt and the board and range controls use chevrons where the reference uses arrows |
| `loading` | light | difference | The app keeps the board controls and caption above a flat skeleton, the reference draws a skeleton for the whole page Follow-up: leaderboard (#5570). |
| `stale` | light | match (15pt floor) | Rows draw initials tiles in place of the fixture cover and profile photos; the leaderboard is reached from the Cabals tab, so the tab bar highlights Cabals where the reference highlights Home; the caption wraps at 15pt and the board and range controls use chevrons where the reference uses arrows |
| `error` | light | match (15pt floor) | The app keeps no board controls over the error block, matching the reference |

## Edit profile

Template `edit-profile` · group `profile` · #5571 (Lane B) · founder: pending

| State | Appearance | Verdict | What differs |
| --- | --- | --- | --- |
| `ready` | light | difference | The avatar is an initials circle ("LN") where the reference shows a photo; the app adds a "You can change your handle again on October 30, 2026." note under Username; Save changes is drawn disabled where the reference draws it enabled Follow-up: edit-profile (#5571). |
| `ready` | dark | difference | as light |
| `invalid` | light | difference | The avatar is an initials circle ("LN") where the reference shows a photo; the app adds a "You can change your handle again on October 30, 2026." note under Username; Save changes is drawn disabled where the reference draws it enabled; the username error reads "a!" with no red outline and no "Choose a valid username." message where the reference draws both Follow-up: edit-profile (#5571). |
| `saving` | light | difference | The avatar is an initials circle ("LN") where the reference shows a photo; the app adds a "You can change your handle again on October 30, 2026." note under Username; Save changes is drawn disabled where the reference draws it enabled Follow-up: edit-profile (#5571). |
| `saved` | light | difference | The avatar is an initials circle ("LN") where the reference shows a photo; the app adds a "You can change your handle again on October 30, 2026." note under Username; Save changes is drawn disabled where the reference draws it enabled; the "Profile saved." notice is drawn above the disabled button where the reference draws it above an enabled one Follow-up: edit-profile (#5571). |
| `error` | light | match (15pt floor) |  |

## Find friends

Template `people` · group `profile` · #5571 (Lane B) · founder: pending

| State | Appearance | Verdict | What differs |
| --- | --- | --- | --- |
| `results` | light | difference | Each row carries a Follow button beside the chevron where the reference draws plain rows; Avatars are initials circles in place of the fixture photos; the fixture people differ (Mia Chen, Noah Park, Zoe Ruiz against Mia, Leo, Sam); the info notice reads "Search is explicit. Monaco never follows anyone for you." where the reference reads "...This design does not upload contacts or automatically follow anyone." Follow-up: people (#5571). |
| `results` | dark | difference | as light |
| `following` | light | difference | Avatars are initials circles in place of the fixture photos; the fixture people differ (Mia Chen, Noah Park, Zoe Ruiz against Mia, Leo, Sam); the info notice reads "Search is explicit. Monaco never follows anyone for you." where the reference reads "...This design does not upload contacts or automatically follow anyone." Follow-up: people (#5571). |
| `no-results` | light | match (15pt floor) |  |
| `loading` | light | difference | The app keeps the search field and the Discover / Following control above the skeleton, the reference draws a skeleton for the whole page Follow-up: people (#5571). |
| `error` | light | difference | The app keeps the search field and the segmented control above the error block, the reference replaces the page with it; the message reads "Check your connection and try again." where the reference matches the shared view copy Follow-up: people (#5571). |

## Public user profile

Template `person` · group `profile` · #5571 (Lane B) · founder: pending

| State | Appearance | Verdict | What differs |
| --- | --- | --- | --- |
| `public` | light | difference | The identity card has no share button and the stats row has no "3 cabals" figure; the avatar is an initials circle in place of the photo; the title reads the person's name where the reference reads "Public user profile"; the harness draws no back chevron; the app shows a primary Follow button where the reference shows an outlined "Following" (the harness profile is not followed) Follow-up: person (#5571). |
| `public` | dark | difference | as light |
| `following` | light | difference | The identity card has no share button and the stats row has no "3 cabals" figure; the avatar is an initials circle in place of the photo; the title reads the person's name where the reference reads "Public user profile"; the harness draws no back chevron Follow-up: person (#5571). |
| `private` | light | difference | The identity card has no share button and the stats row has no "3 cabals" figure; the avatar is an initials circle in place of the photo; the title reads the person's name where the reference reads "Public user profile"; the harness draws no back chevron; the app shows Follow where the reference shows an outlined "Following" Follow-up: person (#5571). |
| `not-found` | light | match (15pt floor) | The title reads "Mia Chen" where the reference reads "Public user profile" |
| `error` | light | match (15pt floor) | The title reads "Mia Chen" where the reference reads "Public user profile" |

## Your profile

Template `profile` · group `profile` · #5571 (Lane B) · founder: pending

| State | Appearance | Verdict | What differs |
| --- | --- | --- | --- |
| `ready` | light | difference | The avatar is a person glyph or initials circle in place of the photo; the stats row (followers, following, cabals) and the Account activity, Find friends and View as others rows are not drawn; the top bar shows a gear where the reference draws a sun icon; the Net worth and Cash block carries a "$50.00 pending" line the reference does not Follow-up: profile (#5571). |
| `ready` | dark | difference | as light |
| `new` | light | difference | The shot is the populated placeholder (Logan Norman, $1,000.00 positions value) where the reference shows a new user (Alex, $0.00, 0 followers) with Account activity, Find friends and View as others rows Follow-up: profile (#5571). |
| `loading` | light | match (15pt floor) | The app keeps the Profile title and the settings gear above the skeleton |
| `error` | light | match (15pt floor) | The app keeps the Profile title and the settings gear above the error block, the reference replaces the page with it |

## Settings

Template `settings` · group `profile` · #5571 (Lane B) · founder: pending

| State | Appearance | Verdict | What differs |
| --- | --- | --- | --- |
| `ready` | light | difference | The Appearance section (System / Light / Dark), the Larger text preview and Reduce transparency toggles and the Decision requests and Replies and mentions toggles are not drawn; the app instead draws a Notifications row and the Account list with Withdraw, Blocked people, Block explorers and Delete account Follow-up: settings (#5571). |
| `ready` | dark | difference | as light |
| `saving` | light | difference | The app draws a "Saving..." notice over the list where the reference draws the Appearance and Notifications toggles; the same sections are missing as in ready Follow-up: settings (#5571). |
| `error` | light | match (15pt floor) |  |
| `sign-out` | light | difference | The sign-out sheet button reads "Sign out" where the reference reads "Sign out of preview"; the sheet is drawn over the Settings list where the reference draws it over the Settings screen Follow-up: settings (#5571). |

## Product decisions this review surfaces

- **Network chooser.** decisions.md says the `network` chooser (Base, Arbitrum, Ethereum) "stays out because the app is Solana-only", but #5561 built `DepositNetworksView` with Base, Arbitrum and Ethereum rows and chain logos (money / network / ready). Someone has to decide whether to keep it; the reference draws letter tiles and its loading, selected and unavailable states have no counterpart in the app.
- **"Cabal" or "group".** The reference still says "group" in places (Group trades, from the group pot, Join a group, Group · 1M); the app says "cabal" throughout. The rows record each, but the wording is one decision for the founder, not eleven fixes.
- **Photos.** The reference shows face photos for people and stock photos for cabals; the app draws initials and the tint gradient (decision 8 for covers). The sample fixtures carry no photo URLs, so every person row is initials in the shots. Adding fixture photos to the harnesses would remove this noise from the next review.

## Design gaps from the sign-in build

Christelle's Mac posted these on #5572 while building sign in (#5558, PR #5601). They live in shared `Design/` files that every lane uses, and each needs a decision on how far it spreads, so none is fixed in this PR:

- `MonacoCard` has no borderless style (the sign-in cover card wants one).
- `CabalCover.thumb` rounds the tile at `0.32 × size` (about 31pt on the 96pt sign-in tile) where the reference uses 16.
- `EmptyState` adds a 16pt side inset, so its "Try again" button is narrower than the field above it. It shows in every error shot above: the app's button is inset by the gutter where the reference button spans the content width.
- `OTPCodeField` boxes are 56pt; the spec says 57.
- The glass back chevron is 20pt medium, and `Typo.display` is 30pt where the sign-in headline reference reads 35.

## Follow-ups

Each difference belongs to the lane of the S ticket that owns the screen. The follow-up issues are not filed yet: this review ran under a no-GitHub-posting rule, so the root files one issue per entry below (title "v3 parity: <template>", milestone UI v3, label of the lane) and links it from the rows above. The entry text is the issue body.

### #5558 (Lane A)

- **Sign in** (`signin`):
  - phone: Send code is drawn disabled on the empty field, the reference draws it enabled; the invite card says "Join a cabal", the reference says "Join a group"
  - email: Send code is drawn disabled on the empty field, the reference draws it enabled; invite card copy "Join a cabal" against "Join a group"
  - invalid: Message reads "Enter your full mobile number." where the reference reads "Enter a valid phone number."; Send code drawn disabled, reference enabled
  - sending: The segmented control is dimmed while sending, the reference keeps the selected segment solid; the button adds a spinner
  - error: The app keeps the headline and the form heading above the "Couldn't load this view" block, the reference replaces the whole screen with it
- **Verify code** (`verify`):
  - ready: Resend link reads "Send a new code" where the reference reads "Resend code"; Continue is drawn disabled on the empty code, the reference draws it enabled
  - invalid: Message reads "That code didn't work. Check it, or send a new one." where the reference reads "That code didn't match. Check it and try again."; link reads "Send a new code" against "Resend code"

### #5559 (Lane A)

- **Home** (`home`):
  - loss: The proposal card header names the proposer (Jordan) where the reference names the cabal (Big Ballers); the card line says "cabal", the reference "group"
  - gain: The proposal card header names the proposer (Jordan) where the reference names the cabal (Big Ballers)
  - multiple-proposals: The proposal card header names the proposer (Jordan) where the reference names the cabal (Big Ballers)
  - voted: The proposal card header names the proposer (Jordan) where the reference names the cabal (Big Ballers)
  - executing: The proposal card header names the proposer (Jordan) where the reference names the cabal (Big Ballers); the card line says "cabal", the reference "group"
  - completed: The proposal card header names the proposer (Jordan) where the reference names the cabal (Big Ballers); the card line says "cabal", the reference "group"
  - expired: The proposal card header names the proposer (Jordan) where the reference names the cabal (Big Ballers)
  - blocked: The proposal card header names the proposer (Jordan) where the reference names the cabal (Big Ballers); the card line says "cabal", the reference "group"
  - stale: The proposal card header names the proposer (Jordan) where the reference names the cabal (Big Ballers)

### #5561 (Lane A)

- **Deposit** (`add-money`):
  - error: The app shows an inline "Couldn't load your balance. Try again" notice above the options, the reference replaces the screen with the "Couldn't load this view" block
- **Card / Apple Pay** (`card-funding`):
  - loading: The app keeps the handoff screen with an "Opening..." button, the reference draws a skeleton
  - cancelled: The body reads "Card purchase didn't go through." where the reference reads "You can return to your account or choose another funding method."
- **Cash out your share** (`cash-out`):
  - ready: Quick chips read 25% / 50% / All where the reference reads $25 / $50 / $100 / Max; the harness opens on $0 where the reference shows $250.00 typed
  - insufficient: The app shows "$8,000" in the error red with no "current share value" overage message and quick chips 25% / 50% / All; the reference shows "$8,000.00", the message "The request exceeds your current share value." and chips $25 / $50 / $100 / Max
  - review: The harness draws "Requested amount $0.00" with Request cash-out disabled where the reference shows $250.00 and an enabled button (the amount argument 250 is not applied)
- **Receive USDC** (`deposit`):
  - address: "How it works" lists two titles ("Send USDC on Solana", "It lands in your balance") where the reference lists three steps with descriptions; the notice says "Transfers on other networks can't be recovered" where the reference placeholder says "The live app supplies the exact address"; the app draws the real QR and address, the reference a preview
  - waiting: "How it works" lists two titles where the reference lists three steps with descriptions; the app draws the real QR and address
  - error: The app keeps the address screen and adds a "Couldn't load your balance. Try again" notice, the reference replaces the screen with the "Couldn't load this view" block
- **Deposit to cabal** (`fund-cabal`):
  - ready: The amount reads "$0" where the reference shows "$250.00" typed; the app adds a "Your cabal earns yield on its USDC" notice and the button reads "Enter an amount"
  - insufficient: The amount reads "$750" in red where the reference reads "$750.00" in ink; the app adds a "Deposit to account" secondary button and the Deposit button is disabled with its amount
  - submitting: The amount reads "$250" where the reference reads "$250.00"; the app adds the yield notice under the "Processing your deposit..." notice
  - completed: The confirmation sheet adds an expand arrow beside the close button; the amount reads "$250" where the reference reads "$250.00"; the app adds a "Your cabal earns yield on its USDC" notice
- **Choose a network** (`network`):
  - ready: Network marks are the real chain logos where the reference draws letter tiles; the subtitles read "Arrives in a few minutes" where the reference reads "Route and timing confirmed by provider"
- **Withdraw to wallet** (`withdraw`):
  - ready: The amount field is focused with the system keyboard and a Paste accessory covering the form; the amount reads "$0" without decimals and the quick chips are a single "Max"
  - invalid: The amount field is focused and the system keyboard covers the form; the amount reads "$100" without decimals and the quick chips are a single "Max"; the destination error is not visible above the keyboard
  - insufficient: The amount field is focused and the system keyboard covers the form; the amount reads "$700" without decimals and the quick chips are a single "Max" where the reference shows $25 / $50 / $100 / Max
  - review: The destination shows the full address where the reference shows a masked example wallet; the app adds "Arrives" and "Network costs $0.12" rows and its button reads "Withdraw" where the reference placeholder reads "Simulate withdrawal request"
  - processing: The destination row shows the full address where the reference shows a masked example wallet
  - completed: The destination row shows the full address where the reference shows a masked example wallet

### #5562 (Lane A)

- **Account activity** (`activity`):
  - ready: Rows show absolute dates ("Oct 2 at 23:00") where the reference shows relative ones ("Today", "Yesterday"); cash outflows carry a minus sign the reference leaves off; the cabal row subtitle truncates at 15pt; the note says "Cabal trades" where the reference says "Group trades"
  - pending: Row shows an absolute date where the reference shows "Today"; the note says "Cabal trades" where the reference says "Group trades"

### #5563 (Lane A)

- **Propose · action chooser** (`propose`):
  - ready: The sheet is drawn over a blank grey screen, the reference draws it over the cabal screen
  - no-holdings: The sheet is drawn over a blank grey screen, the reference draws it over the cabal screen
  - not-member: The sheet is drawn over a blank grey screen, the reference draws it over the cabal screen
- **Review proposal · sheet** (`review-proposal`):
  - ready: The sheet is drawn over a blank grey screen, the reference draws it over the stock screen; the app adds "Everyone agrees" and "Pot 25% of $100.00" rows under the key rows; the symbol tile is neutral grey
  - sell: The sheet is drawn over a blank grey screen, the reference draws it over the stock screen; the app adds "Everyone agrees" and "Raises about $21.00" rows and states the amount in shares ("0.073 shares") where the reference states "$400.00"
  - long-thesis: The sheet is drawn over a blank grey screen; the thesis is clipped by the footer where the reference shows the whole thesis with the keys beneath; the symbol tile is neutral grey
  - submitting: The sheet is drawn over a blank grey screen, the reference draws it over the stock screen; the app adds "Everyone agrees" and "Pot 25% of $100.00" rows
  - sent: The sheet is drawn over a blank grey screen, the reference draws it over the stock screen
  - error: The sheet is drawn over a blank grey screen; the error reads "Couldn't send your proposal. Your draft is saved here. Something went wrong. Try again." where the reference reads "Couldn't send your proposal. Your draft is saved here."
- **Choose stock / holding** (`select-stock`):
  - buy: Symbol tiles are neutral grey where the reference tints them; the list shows the fixture's three stocks
  - sell: Symbol tile is neutral grey; the row subtitle reads "0.73 shares" where the reference reads "30.0000 units"; the cabal heading above the list is not drawn
  - no-holdings: The search field and the Volume / All controls are not drawn above the empty state, the reference draws them
  - error: The title reads "Buy stock" where the reference reads "Choose stock / holding"; the message reads "Check your connection and try again." where the reference reads "Your existing information hasn't been changed. Try again when your connection is available."
- **Proposal amount & thesis** (`trade-amount`):
  - ready: The amount field is focused with the system keyboard covering the thesis and the error; the amount reads without decimals ("$25") where the reference reads "$400.00"; the symbol tile is neutral grey; the app adds a "The cabal can spend $100.00" line
  - sell: The amount field is focused with the system keyboard covering the thesis; the amount reads "$0" where the reference shows "$400.00"; quick chips are 25% / 50% / All where the reference shows $25 / $50 / $100 / Max; the symbol tile is neutral grey
  - insufficient: The amount field is focused with the system keyboard covering the thesis and the error; the amount reads without decimals ("$25") where the reference reads "$400.00"; the symbol tile is neutral grey; the app adds a "The cabal can spend $100.00" line
  - empty-thesis: The amount field is focused with the system keyboard covering the thesis and the error; the amount reads without decimals ("$25") where the reference reads "$400.00"; the symbol tile is neutral grey; the app adds a "The cabal can spend $100.00" line
  - error: The title reads "Buy Alphabet" where the reference reads "Proposal amount & thesis"; the message reads "Check your connection and try again." where the reference reads "Your existing information hasn't been changed. Try again when your connection is available."

### #5564 (Lane A)

- **Proposal & discussion** (`proposal`):
  - open: The symbol tile is neutral grey where the reference draws a lime tile; the vote summary is one line ("2 yes · 0 no · 1 not voted") where the reference labels the bar ends; the app adds an Expected section and the Vote No / Vote Yes bar sits over the content
  - sell: The sell headline reads "1.5 shares / About $25.00" where the reference reads "$400.00"; The symbol tile is neutral grey where the reference draws a lime tile; the vote summary is one line ("2 yes · 0 no · 1 not voted") where the reference labels the bar ends
  - voted: The symbol tile is neutral grey where the reference draws a lime tile; the vote summary is one line ("2 yes · 0 no · 1 not voted") where the reference labels the bar ends; the Change vote action is a full-width button where the reference shows it beside the Discussion heading
  - not-voter: The symbol tile is neutral grey where the reference draws a lime tile; the vote summary is one line ("2 yes · 0 no · 1 not voted") where the reference labels the bar ends; the app adds an Expected section
  - approved: The symbol tile is neutral grey (GOOGL) where the reference draws a lime tile; the vote summary is one line ("2 yes · 0 no · 1 not voted") where the reference labels the bar ends
  - executed: The symbol tile is neutral grey where the reference draws a lime tile; the vote summary is one line where the reference labels the bar ends
  - expired: The symbol tile is neutral grey where the reference draws a lime tile; the vote summary is one line ("2 yes · 0 no · 1 not voted") where the reference labels the bar ends
  - blocked: The symbol tile is neutral grey where the reference draws a lime tile; the vote summary is one line where the reference labels the bar ends
  - rejected: The symbol tile is neutral grey where the reference draws a lime tile; the vote summary is one line ("2 yes · 0 no · 1 not voted") where the reference labels the bar ends
  - error: The app keeps the Discussion section and adds "Couldn't load comments" under the error block, and its title reads "Proposal" where the reference title is the proposal's name
- **All proposals** (`proposals`):
  - open: Both rows are full cards with the proposer name above the title; the reference draws the newest as a card headed "Mia proposed NVIDIA" with a reply count and the older ones as plain rows; symbol tile is neutral grey
  - closed: Symbol tiles are neutral grey where the reference tints them (lime, sand, lilac); row subtitles truncate at 15pt
  - error: The error block sits on a white band that cuts across the cream canvas above and below it; the reference draws it on the canvas
- **Votes · sheet** (`votes`):
  - open: The sheet has only a close button where the reference also draws an expand / collapse arrow; voter avatars are initials circles
  - recorded: The sheet has only a close button where the reference also draws an expand / collapse arrow; voter avatars are initials circles
  - closed: The sheet has only a close button where the reference also draws an expand / collapse arrow; voter avatars are initials circles
  - expired: The sheet has only a close button where the reference also draws an expand / collapse arrow; voter avatars are initials circles
  - rejected: The sheet has only a close button where the reference also draws an expand / collapse arrow; voter avatars are initials circles
  - empty: The sheet has only a close button where the reference also draws an expand / collapse arrow; voter avatars are initials circles
  - expanded: The sheet has only a close button where the reference also draws an expand / collapse arrow; voter avatars are initials circles
  - loading: The sheet has only a close button where the reference also draws an expand / collapse arrow; voter avatars are initials circles; the proposal beneath is a skeleton where the reference keeps the proposal visible
  - error: The sheet has only a close button where the reference also draws an expand / collapse arrow; voter avatars are initials circles; the proposal beneath shows its own full-screen error where the reference keeps the proposal visible

### #5565 (Lane B)

- **Your cabals** (`cabals`):
  - empty: The empty state adds an "Explore cabals" secondary button the reference does not draw
  - loading: The app draws the Cabals title, the search field and the leaderboard header above the skeleton; the reference draws a skeleton for the whole page
  - error: The app keeps the Cabals title, the search field and the Cabal leaderboard around the error block; the reference replaces the whole page with it
- **Start a cabal** (`create-cabal`):
  - ready: The cover image slot starts empty ("Add a cover photo") where the reference shows a chosen cover with a check; Create cabal is pinned to the bottom where the reference draws it in the flow
  - creating: The app keeps the form under a "Creating your cabal" header with a spinner button, the reference replaces the form content with a header and the greyed form below
- **Discover cabals** (`discover`):
  - results: Rows carry a Request / Joined chip beside the chevron where the reference draws plain rows; avatars are initials squares in place of the fixture photos
  - request-pending: The app adds a "Your request to join Weekend pot is pending." notice and a "Request pending" chip; the reference draws the plain list
  - loading: The app keeps the "Find a cabal" heading and the search field above the skeleton; the reference draws a skeleton for the whole page
  - error: The app keeps the "Find a cabal" heading and the search field above the error block; the reference replaces the whole page with it

### #5566 (Lane B)

- **Cabal information** (`cabal-info`):
  - member: The app adds a "Cabal treasury" section below the actions where the reference ends with a "View your share" button
  - admin: The Rules section carries an "Edit" link the reference does not draw
- **Members & voters** (`members`):
  - members: The control has two segments (Members, Voters) for a member viewer where the reference draws three (Members, Voters, Requests); avatars are initials circles
  - choose-voters: The app draws a Voters tab with a "Voter" chip per row; the reference opens an "Eligible voters" sheet over the screen with a check per member and a Save voters button
- **Public cabal** (`public-cabal`):
  - preview: There is no About section between the return and Membership (the reference draws the cabal description); the return block shows the "Returns open soon" caption (missing-data caption); the app adds an "Approval required: the creator reviews each request." note above the button
  - pending: There is no About section between the return and Membership (the reference draws the cabal description); the return block shows the "Returns open soon" caption (missing-data caption); the app adds a "Cancel request" link under the disabled button
  - member: There is no About section between the return and Membership (the reference draws the cabal description); the return block shows the "Returns open soon" caption (missing-data caption)

### #5567 (Lane B)

- **Cabal asset holding** (`holding`):
  - ready: The symbol tile is neutral grey where the reference draws a lime tile; the chart axis dates carry a time ("Sep 19 at 10:49") where the reference shows "Sep 18"
  - loss: The symbol tile is neutral grey where the reference draws a lime tile; the chart axis dates carry a time ("Sep 19 at 10:49") where the reference shows "Sep 18"
  - empty: The symbol tile is neutral grey where the reference draws a lime tile; the P&L caption reads "Position P&L · All time" where the reference reads "Position P&L · 1M"
  - stale: The symbol tile is neutral grey where the reference draws a lime tile; the chart axis dates carry a time ("Sep 19 at 10:49") where the reference shows "Sep 18"; the stale notice wraps to two lines at 15pt

### #5568 (Lane B)

- **Stock · token market** (`instrument`):
  - ready: Native candles, the interval picker and the chart-style toggle replace the reference line chart (Context decision); the symbol tile is neutral grey where the reference draws a lime tile; the app adds a "Market open" pill and a "Propose a sell" button; the watch star is empty where the reference draws it filled
  - loss: Native candles, the interval picker and the chart-style toggle replace the reference line chart (Context decision); the symbol tile is neutral grey where the reference draws a lime tile; the app adds a "Market open" pill and a "Propose a sell" button; the watch star is empty where the reference draws it filled; the loss figures read -4.44% where the reference reads -4.18% (fixture numbers)
  - no-holders: Native candles, the interval picker and the chart-style toggle replace the reference line chart (Context decision); the symbol tile is neutral grey where the reference draws a lime tile; the app adds a "Market open" pill and a "Propose a sell" button; the watch star is empty where the reference draws it filled
  - no-proposals: Native candles, the interval picker and the chart-style toggle replace the reference line chart (Context decision); the symbol tile is neutral grey where the reference draws a lime tile; the app adds a "Market open" pill and a "Propose a sell" button; the watch star is empty where the reference draws it filled
  - stale: Native candles, the interval picker and the chart-style toggle replace the reference line chart (Context decision); the symbol tile is neutral grey where the reference draws a lime tile; the app adds a "Market open" pill and a "Propose a sell" button; the watch star is empty where the reference draws it filled; the stale notice reads "Token price delayed" where the reference matches, and it sits above the price in both
  - unavailable: Native candles, the interval picker and the chart-style toggle replace the reference line chart (Context decision); the symbol tile is neutral grey where the reference draws a lime tile; the app adds a "Market open" pill and a "Propose a sell" button; the watch star is empty where the reference draws it filled; the unavailable notice wraps to two lines at 15pt
- **Markets** (`markets`):
  - ready: Symbol tiles are neutral grey where the reference tints them (lime, lilac, sand); the mover cards show "Opens soon" where the reference shows the 24h volume figure (missing-data caption)
  - watchlist-empty: Symbol tiles are neutral grey where the reference tints them (lime, lilac, sand); the mover cards show "Opens soon" where the reference shows the 24h volume figure (missing-data caption); the Popular now rows show price and change where the reference shows 24h volume and trade counts
  - no-results: The sort control reads "Default" where the reference reads "Volume"
  - loading: The app keeps the Markets title above the skeleton, the reference draws a skeleton for the whole page
  - stale: Symbol tiles are neutral grey where the reference tints them (lime, lilac, sand); the mover cards show "Opens soon" where the reference shows the 24h volume figure (missing-data caption); the notice reads "Prices delayed · just now." where the reference reads "Prices delayed · 14 minutes ago."; the Popular now rows show price and change where the reference shows 24h volume
- **Manage watchlist** (`watchlist`):
  - ready: Symbol tiles are neutral grey where the reference tints them; the "3 stocks selected" caption is larger at the 15pt floor
  - empty: The app keeps all rows with empty stars below the empty state where the reference draws three filled ones; the caption reads "No stocks selected" where the reference reads "3 stocks selected"; the title reads "Your watchlist" where the reference reads the same on empty and "Manage watchlist" on error
  - loading: The title reads "Your watchlist" where the reference reads "Manage watchlist"; the app keeps the search field and caption above the skeleton, the reference draws a skeleton for the whole page
  - error: The title reads "Your watchlist" where the reference reads "Manage watchlist"; the message reads "Check your connection..." copy from the shared view, matching the reference

### #5569 (Lane B)

- **Feed** (`feed`):
  - all: Cards carry a like count where the reference shows the like icon with a count too, but the avatar is an initials circle where the reference shows a photo; the proposal card text clips mid-sentence at 15pt; the search field and the filter menu glyph differ (filter lines against the sliders icon)
  - proposals: Avatars are initials circles in place of the fixture photos; the end-of-list line reads "No more matching updates" where the reference reads "You're caught up"; the filter glyph is lines where the reference draws sliders; the Propose sell card shows -2.4% where the reference shows +4.9% (fixture numbers)
  - trades: Avatars are initials circles in place of the fixture photos; the end-of-list line reads "No more matching updates" where the reference reads "You're caught up"; the filter glyph is lines where the reference draws sliders
  - price-moves: Avatars are initials circles in place of the fixture photos; the end-of-list line reads "No more matching updates" where the reference reads "You're caught up"; the filter glyph is lines where the reference draws sliders
  - following: The shot is the follows-nobody empty state where the reference shows the Following feed with posts; no scenario selects the Following scope over the fixture items
  - pnl-private: Avatars are initials circles in place of the fixture photos; the cards read the same "P&L private" tags as the reference
  - loading: The app keeps the Feed title and the search field above the skeleton, the reference draws a skeleton for the whole page
  - load-more-error: The harness scrolls to the bottom, so the shot shows "Older updates couldn't load" with a Retry older updates button; the reference shows the top of the feed; avatars are initials circles
  - error: The app keeps the Feed title, the search field and the toolbar buttons above the error block; the reference replaces the page with it
- **Post & comments** (`post`):
  - replying: The system dictation prompt ("Speed up your typing by sliding your finger across the letters") covers the lower half of the shot; the "Replying to Mia" bar sits above the field as the reference's reply row does not
  - failed: The offline toast covers the composer and the "Your reply wasn't sent" notice is cut off by it; Avatars are initials circles and the symbol tile is neutral grey where the reference shows photos and a lime tile

### #5570 (Lane B)

- **Leaderboard** (`leaderboard`):
  - cabals: Rows draw initials tiles in place of the fixture cover and profile photos; the leaderboard is reached from the Cabals tab, so the tab bar highlights Cabals where the reference highlights Home; the caption wraps at 15pt and the board and range controls use chevrons where the reference uses arrows; the app adds a crown on rank 1 and a trailing "All 5 eligible cabals shown" line
  - people: Rows draw initials tiles in place of the fixture cover and profile photos; the leaderboard is reached from the Cabals tab, so the tab bar highlights Cabals where the reference highlights Home; the caption wraps at 15pt and the board and range controls use chevrons where the reference uses arrows; the fixture people differ from the reference (Ana Kovacs, Marcus Hale against Mia, Leo)
  - filtered: Rows draw initials tiles in place of the fixture cover and profile photos; the leaderboard is reached from the Cabals tab, so the tab bar highlights Cabals where the reference highlights Home; the caption wraps at 15pt and the board and range controls use chevrons where the reference uses arrows; the shot lists three cabals under the filter where the reference lists five
  - loading: The app keeps the board controls and caption above a flat skeleton, the reference draws a skeleton for the whole page

### #5571 (Lane B)

- **Edit profile** (`edit-profile`):
  - ready: The avatar is an initials circle ("LN") where the reference shows a photo; the app adds a "You can change your handle again on October 30, 2026." note under Username; Save changes is drawn disabled where the reference draws it enabled
  - invalid: The avatar is an initials circle ("LN") where the reference shows a photo; the app adds a "You can change your handle again on October 30, 2026." note under Username; Save changes is drawn disabled where the reference draws it enabled; the username error reads "a!" with no red outline and no "Choose a valid username." message where the reference draws both
  - saving: The avatar is an initials circle ("LN") where the reference shows a photo; the app adds a "You can change your handle again on October 30, 2026." note under Username; Save changes is drawn disabled where the reference draws it enabled
  - saved: The avatar is an initials circle ("LN") where the reference shows a photo; the app adds a "You can change your handle again on October 30, 2026." note under Username; Save changes is drawn disabled where the reference draws it enabled; the "Profile saved." notice is drawn above the disabled button where the reference draws it above an enabled one
- **Find friends** (`people`):
  - results: Each row carries a Follow button beside the chevron where the reference draws plain rows; Avatars are initials circles in place of the fixture photos; the fixture people differ (Mia Chen, Noah Park, Zoe Ruiz against Mia, Leo, Sam); the info notice reads "Search is explicit. Monaco never follows anyone for you." where the reference reads "...This design does not upload contacts or automatically follow anyone."
  - following: Avatars are initials circles in place of the fixture photos; the fixture people differ (Mia Chen, Noah Park, Zoe Ruiz against Mia, Leo, Sam); the info notice reads "Search is explicit. Monaco never follows anyone for you." where the reference reads "...This design does not upload contacts or automatically follow anyone."
  - loading: The app keeps the search field and the Discover / Following control above the skeleton, the reference draws a skeleton for the whole page
  - error: The app keeps the search field and the segmented control above the error block, the reference replaces the page with it; the message reads "Check your connection and try again." where the reference matches the shared view copy
- **Public user profile** (`person`):
  - public: The identity card has no share button and the stats row has no "3 cabals" figure; the avatar is an initials circle in place of the photo; the title reads the person's name where the reference reads "Public user profile"; the harness draws no back chevron; the app shows a primary Follow button where the reference shows an outlined "Following" (the harness profile is not followed)
  - following: The identity card has no share button and the stats row has no "3 cabals" figure; the avatar is an initials circle in place of the photo; the title reads the person's name where the reference reads "Public user profile"; the harness draws no back chevron
  - private: The identity card has no share button and the stats row has no "3 cabals" figure; the avatar is an initials circle in place of the photo; the title reads the person's name where the reference reads "Public user profile"; the harness draws no back chevron; the app shows Follow where the reference shows an outlined "Following"
- **Your profile** (`profile`):
  - ready: The avatar is a person glyph or initials circle in place of the photo; the stats row (followers, following, cabals) and the Account activity, Find friends and View as others rows are not drawn; the top bar shows a gear where the reference draws a sun icon; the Net worth and Cash block carries a "$50.00 pending" line the reference does not
  - new: The shot is the populated placeholder (Logan Norman, $1,000.00 positions value) where the reference shows a new user (Alex, $0.00, 0 followers) with Account activity, Find friends and View as others rows
- **Settings** (`settings`):
  - ready: The Appearance section (System / Light / Dark), the Larger text preview and Reduce transparency toggles and the Decision requests and Replies and mentions toggles are not drawn; the app instead draws a Notifications row and the Account list with Withdraw, Blocked people, Block explorers and Delete account
  - saving: The app draws a "Saving..." notice over the list where the reference draws the Appearance and Notifications toggles; the same sections are missing as in ready
  - sign-out: The sign-out sheet button reads "Sign out" where the reference reads "Sign out of preview"; the sheet is drawn over the Settings list where the reference draws it over the Settings screen
