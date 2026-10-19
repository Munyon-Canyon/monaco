# v3 accessibility audit (Q4, #5575)

Audit of VoiceOver labels, 44pt targets, haptics and Reduce Motion across the 43 default v3 screens, on one build of `staging` after the parity gallery (#5685) landed. Findings only: the few fixes made inline are listed under "Fixed in this PR". Nothing here is filed as an issue (the founder's call); every follow-up below is written so it can be filed as is.

Run it locally (the CI plan runs `MonacoTests` only), on the slim lane simulator:

    xcodebuild test -project apps/mobile/Monaco.xcodeproj -scheme Monaco -destination "id=$(scripts/gold-sim-udid.sh)" -only-testing:MonacoUITests/V3AccessibilityAuditUITests

`V3AccessibilityAuditUITests` launches each default state through its sample harness (the manifest lines of `scripts/qa/sample-screens.txt`), runs `performAccessibilityAudit` for `.hitRegion`, `.sufficientElementDescription`, `.elementDetection` and `.trait`, swipes up and audits again to the bottom, and prints `a11y|`, `audit|` and `small|` lines for every element. The per-template tables below come from those `a11y|` lines (buttons only; frames are the accessibility frames, so 43pt reads as a 44pt target that the system rounded down).

## Audit result

45 tests, all 43 screens walked top to bottom. The audit found 21 distinct findings, 15 of which are allow-listed with a reason in the test; the 6 comment-avatar ones were fixed (below), and all are listed in "Follow-ups".

| Audit type | Findings | Verdict |
| --- | --- | --- |
| `.hitRegion` | 1: the chat day separator "Today" (`chat-day-s5`) | Allow-listed: a static label, not a control |
| `.sufficientElementDescription` | 8 "Label not human-readable" on money and link strings (`$750.00`, `−$417.68`, `$175.42`, `$248.50`, `+$100.00`, `$25.00`, the invite URL); 6 "Element has no description" on comment avatars | Money and link strings allow-listed (they read as typed; wording is follow-up F3). Comment avatars fixed |
| `.elementDetection` | 6 "Potentially inaccessible text" with no identifier on holding, home, instrument, person, proposal and votes | Allow-listed, follow-up F7 |
| `.trait` | none | ok |

## Fixed in this PR

- `ProposalVoteDock.swift`: `Haptics.success()` when the server accepts a vote (design rule: `.success` when a vote lands). It was missing.
- `CommentThreadView.swift`: the comment avatar link is hidden from VoiceOver. The author name beside it opens the same profile, so VoiceOver read two links, and the avatar one had no description.

## Pattern review (Proposal 2)

Where the expected label differs from what the app reads, the difference is a follow-up and not a rename: journeys find controls by these labels and identifiers, and Q5 owns the journeys.

| Control | Expected | Actual | Verdict |
| --- | --- | --- | --- |
| Glass round "+" (Cabals) | "Start a cabal", button | not in the `cabals__cabals__ready` tree (the screen draws the cover cards and rank rows; "Start a cabal" is only the Create screen title) | F1 |
| Cabal share round, cover share disc | "Share <cabal name>" | cover share disc reads "Share Big Ballers" (ok); the cabal share round and hero share read "Share cabal invitation" | F2 |
| Cabal info round | "Cabal information" | "Cabal information" on the cabal and chat screens (ok) | ok |
| Bell (Home, Feed) | "Activity", unread value "New activity" | Home "Notifications"; Feed "Activity notifications"; no "New activity" value in the default frames | F4 |
| Feed filter round | "Filter feed" | "Filter feed" (the menu and its glass round both expose it) | ok |
| Profile settings round | "Settings" | "Settings" | ok |
| Sheet close / expand | "Close" / "Expand" | "Close" / "Expand", but 42x42 | F5 |
| Star (instrument, watchlist) | "Add <ticker> to watchlist" / "Remove <ticker> from watchlist", selected when on | instrument "Remove from watchlist" (no ticker), watchlist rows "Remove Apple" / "Add AMD" (name, not ticker); the selected trait is set | F6 |
| Pencil (identity card, edit profile) | "Edit profile" / "Change photo" | "Edit profile photo" on both the profile card and edit profile | F6 |
| Feed reactions | "Like", "Reply", "Share"; Like selected when on | "Like <event summary>", "Share this update", "Reply" (comments) | F6 |
| Compact Deposit capsule | "Deposit" | "Deposit", 87x44 | ok |
| P&L capsule / pnl text | the signed value as money text reads it | "up 3.50 percent" and "up 4.5 percent, up 2 dollars" inside rank and board rows; merged into the row | ok |
| Count pill | merged into the header ("Proposals, 1") | member count reads as its own text ("3 members") | F6 |
| "Needs your vote" badge | part of the card label | "1 vote needed" is inside the cover card label | ok |
| Cover card | one element with name, members, share, ownership, all-time, vote | one element: "Big Ballers, 8 members, 1 vote needed, your share $7,000.00, ownership 35.00%, your all-time P&L down 300 dollars, …" | ok |
| Decision card | one element with action, cabal, amount, closing time, plus a Review button | one element: "Kai: Voting open. Buy NVDA, $1,200.00 from the cabal pot. 2 yes votes needed · Closes in 23h" | ok |
| Rank row | "<rank>, <name>, <signed money>, <signed percent>", button | "First, Ape Society, up 42.10 percent, up 12690 dollars" | ok |
| Feed event card | one element for the summary, reactions separate | reactions are separate 44x44 buttons | ok |
| Key rows / receipt lines | "<key>, <value>" | settings, members and activity rows read "<title>, <detail>" | ok |
| Charts | one element with the range summary and an adjustable action | the cabal pot reads "0 percent, past month"; the instrument chart is covered by `AssetChartSampleUITests` | not re-driven here |

## Targets (Proposal 3)

Measured from the accessibility frames at the default size (AX5 is Q3's audit). A frame of 43pt tall is the 44pt target rounded down by the audit snapshot.

| Control | Result |
| --- | --- |
| Compact Deposit capsule | 87x44, ok |
| Chips and periods | period segment 85x43 and chips 36pt tall visually, hit 44 (the Cabals leaderboard period "1M" is 85x43, ok). The feed filter menu label is 56x35 (F5) |
| Cover share disc, stars, pencil | 44x44, ok |
| Reactions (like, share, reply) | 44x44, ok |
| Toolbar rounds | 44x43 or 44x44, ok. Sheet close and expand are 42x42 (F5) |
| OTP boxes, keypad keys | not reached by these defaults: the verify screen draws a single code field and the keypad is not in the 43 defaults. See F8 |
| Menu selects | create-cabal "Everyone" and "People I pick" 180x43, ok |
| "See all" links | proposals "See all" 60x43, ok |
| Under 43pt | chat author names 28x18 (hidden from VoiceOver, the bubble carries the name), feed actor buttons 28-40pt wide by 44 high (F5), the withdraw Paste button 66x34, the keyboard Done button 62x36 and the system sheet grabber 72x23 (system chrome) |
| Text fields | search and composer fields report the text area (22pt) as their frame; the padded field is the target |

## Haptics (Proposal 4)

Checked with `grep -rnE 'sensoryFeedback|Haptics\.' apps/mobile/Monaco/Features apps/mobile/Monaco/Design` against `docs/design.md` "Icons and motion".

| Rule | Result |
| --- | --- |
| No `Haptics.tap()` behind a `.monacoPrimary` button | none. The primary style plays its own impact (`MonacoButtons.swift`). Two other callers play `tap()` on non-primary buttons: `CircleAction` (`MonacoButtons.swift`, the Home and cabal action tiles) and the chat send disc (`GroupChatParts.swift`). The rule says only the primary style taps, so both are follow-up F9 |
| `.success` only after the server confirms | withdraw `.accepted`, fund cabal `.accepted`, cash out `.started`: ok. Vote: was missing, fixed here after `model.vote` returns true. Proposal sent (`ProposeReviewScreen` outcome) and an inbound deposit have no `.success`: F10 (`ProposeReviewScreen.swift` is Q2's file) |
| `.warning` on error toasts | the toast banner plays it for every non-success toast (`MonacoToastBanner.swift`), and the OTP and name-editor errors that raise no toast play it themselves: ok |
| `.selection` on chips, segments, periods, chart scrub | chip bar, segmented control (also the period chips), keypad, tabs, watch star, chart scrub (`sensoryFeedback`): ok |

## Reduce Motion (Proposal 4)

`testReduceMotionScreensDraw` launches `home`, `cabal`, `review-proposal` and `fund-cabal` with `-UIAccessibilityReduceMotionEnabled YES`; all four draw. The motion code reads as follows. This is a code read: the test does not measure animation.

| Item | Result |
| --- | --- |
| Sheet and card transitions | press scale, selection springs and the keyboard reveal read `accessibilityReduceMotion` and drop the animation |
| Chart reveal | `MonacoCandleChart` skips the reveal animation under Reduce Motion |
| Success orb | `OutcomeView` has no pulse or repeating animation at all |
| Skeleton pulse | static under Reduce Motion (`SkeletonPulse`) |
| Toast fade | `MonacoToastBanner` keeps its fade; it is the one exception the rule allows |
| Numeric roll | `MonacoMoney` and `MonacoPriceTick` read the flag; no violation found |

## Follow-ups

Written to be filed as is. None is filed.

- **F1.** The Cabals tab has no "Start a cabal" round in the default state. Confirm against the design (`cabals__cabals__ready`) and add the glass "+" with the label "Start a cabal", or document that Create lives elsewhere.
- **F2.** Make the cabal share round and hero share read "Share <cabal name>" like the cover share disc. Keep the identifiers `cabal-share-button` and `cabal-hero-share`.
- **F3.** Money text flagged "Label not human-readable" (`cabal-slice-cash`, `cabal-slice-value`, `home-portfolio-total`, `asset-detail-price`, `deposit-screen-balance-value`, `account-txn-receipt-amount`, the `$25.00` proposal amount): give each an explicit spoken label ("down 417 dollars 68 cents") like the cover cards already use.
- **F4.** Unify the bell: one label "Activity" on Home and Feed, with the value "New activity" when unread. Home reads "Notifications" and Feed "Activity notifications".
- **F5.** Hit areas: sheet close and expand rounds (42x42), the feed filter menu label (56x35) and feed actor buttons (28-40pt wide) need a 44pt frame and content shape.
- **F6.** Label consistency: stars should read "Add <ticker> to watchlist" (instrument has no ticker), pencils "Edit profile" / "Change photo", the feed Like and Share buttons should read "Like" and "Share" with the summary as a hint, and the member count pill should merge into its section header.
- **F7.** `.elementDetection` "Potentially inaccessible text" on holding, home, instrument, person, proposal and votes: find the unlabelled text run each screen draws.
- **F8.** Drive the OTP boxes and keypad keys: neither is in the 43 default states. Add a keypad state to the audit once the manifest has one.
- **F9.** `CircleAction` and the chat send disc play `Haptics.tap()` on non-primary buttons. Either allow it in `docs/design.md` or drop it.
- **F10.** Add `Haptics.success()` when a proposal is sent (`ProposeReviewScreen`, after Q2 lands) and when the deposit poller credits an inbound deposit.

## Per-template control tables

Every button in each default state, from the audit run.

### `access__signin__phone`

| Control | Label read | Trait | Frame | Verdict |
| --- | --- | --- | --- | --- |
| `—` | Text message | button selected | 180x44 | ok |
| `—` | Email | button | 180x44 | ok |
| `smsSendCodeButton` | Send code | button | 353x48 | ok |

### `access__verify__ready`

| Control | Label read | Trait | Frame | Verdict |
| --- | --- | --- | --- | --- |
| `smsVerifyBackButton` | Back | button | 44x44 | ok |
| `smsVerifyButton` | Continue | button | 353x48 | ok |
| `smsResendCodeButton` | Send a new code | button | 121x44 | ok |
| `smsChangeAddressButton` | Change number | button | 114x44 | ok |

### `cabals__cabal-info__member`

| Control | Label read | Trait | Frame | Verdict |
| --- | --- | --- | --- | --- |
| `cabal-members-row` | Members & voters, 3 members | button | 393x72 | ok |
| `cabal-invite-member-row` | Invite to this cabal, Share an invitation | button | 393x72 | ok |
| `cabal-treasury-solscan` | View on Solscan | button | 393x72 | ok |
| `cabal-info-view-share` | View your share | button | 353x48 | ok |
| `cabalLeaveButton` | Leave cabal | button | 353x48 | ok |
| `—` | Home | button | 74x54 | ok |
| `—` | Cabals | button selected | 81x54 | ok |
| `—` | Markets | button | 75x54 | ok |
| `—` | Feed | button | 74x54 | ok |
| `—` | Profile | button | 74x54 | ok |

### `cabals__cabal__ready`

| Control | Label read | Trait | Frame | Verdict |
| --- | --- | --- | --- | --- |
| `cabal-share-button` | Share cabal invitation | button | 44x43 | ok |
| `cabal-details-button` | Cabal information | button | 44x43 | ok |
| `cabal-hero-share` | Share cabal invitation | button | 44x44 | ok |
| `cabal-value-chart-range-1W` | Past week | button | 122x46 | ok |
| `cabal-value-chart-range-1M` | Past month | button selected | 122x46 | ok |
| `cabal-value-chart-range-ALL` | All time | button | 122x46 | ok |
| `cabal-action-fund` | Deposit | button | 66x72 | ok |
| `cabal-action-propose` | Propose | button | 66x72 | ok |
| `cabal-action-cash-out` | Cash out | button | 66x72 | ok |
| `cabal-chat-open` | Open chat | button | 87x44 | ok |
| `cabal-action-chat` | Chat. Alex, 12m ago: What’s everyone watching this week?. 3 unread messages | button | 353x112 | ok |
| `cabal-action-chat-unread` | Alex, 12m, What’s everyone watching this week?, QA pot · 3 new messages, 3 | button | 353x112 | ok |
| `—` | See all | button | 60x44 | ok |
| `cabal-proposals-empty` | Propose the first buy. | button | 153x44 | ok |
| `cabal-holding-GOOGL` | Alphabet, GOOGL · 0.73 shares · $342.47, $250.00, up 73 cents | button | 393x72 | ok |
| `cabal-holdings-cash` | Cash, USDC, $750.00, 3.5% APY | button | 393x72 | ok |
| `cabal-member-user-1` | First, Investor 1, up 4.50 percent, up 2 dollars | button | 393x72 | ok |
| `cabal-member-user-2` | Rank 2, Investor 2, up 4.00 percent, up 2 dollars | button | 393x72 | ok |
| `cabal-member-user-3` | Rank 3, Investor 3, up 3.50 percent, up 2 dollars | button | 393x72 | ok |
| `cabal-member-board-all-shown` | All 3 shown | button | 393x48 | ok |
| `cabal-rules-row` | Rules & members, 3 members | button | 393x72 | ok |
| `—` | Home | button | 74x54 | ok |
| `—` | Cabals | button selected | 81x54 | ok |
| `—` | Markets | button | 75x54 | ok |
| `—` | Feed | button | 74x54 | ok |
| `—` | Profile | button | 74x54 | ok |

### `cabals__cabals__ready`

| Control | Label read | Trait | Frame | Verdict |
| --- | --- | --- | --- | --- |
| `cabals-new-button` | Create a cabal | button | 44x44 | ok |
| `cabal-cover-card-0f1d6a52-0001-7000-8000-00000` | Big Ballers, 8 members, 1 vote needed, your share $7,000.00, ownership 35.00%, your all-time P&L down 300 doll | button | 353x208 | ok |
| `square.and.arrow.up` | Share Big Ballers | button | 44x44 | ok |
| `cabal-cover-card-0f1d6a52-0001-7000-8000-00000` | Tech Crew, 5 members, your share $2,782.32, ownership 14.00%, your all-time P&L up 50 dollars, up 1.8 percent, | button | 353x208 | ok |
| `square.and.arrow.up` | Share Tech Crew | button | 44x44 | ok |
| `cabal-cover-card-0f1d6a52-0001-7000-8000-00000` | Underwater, 9 members, your share $1,300.00, ownership 6.50%, your all-time P&L down 167 dollars 68 cents, dow | button | 353x208 | ok |
| `square.and.arrow.up` | Share Underwater | button | 44x44 | ok |
| `cabals-board-period` | Leaderboard period, 1M | button | 85x43 | ok |
| `—` | (no label) | button | 85x43 | ok |
| `cabals-board-filter` | Filter and sort cabal leaderboard | button | 44x43 | ok |
| `cabals-board-rank-info` | Rank change | button | 112x44 | ok |
| `cabals-board-row-ape-society` | First, Ape Society, up 42.10 percent, up 12690 dollars | button | 393x72 | ok |
| `cabals-board-row-quant-kids` | Rank 2, Quant Kids, up 37.10 percent, up 7420 dollars | button | 393x72 | ok |
| `cabals-board-row-dip-buyers` | Rank 3, Dip Buyers, up 19.60 percent, up 3180 dollars | button | 393x72 | ok |

### `cabals__chat__ready`

| Control | Label read | Trait | Frame | Verdict |
| --- | --- | --- | --- | --- |
| `chat-info` | Cabal information | button | 44x43 | ok |
| `chat-avatar-s1` | Ana, open profile | button | 44x44 | ok |
| `chat-author-s1` | Ana | button | 28x18 | hit area under 44pt, see Follow-ups |
| `chat-avatar-s3` | Leo, open profile | button | 44x44 | ok |
| `chat-author-s3` | Leo | button | 26x18 | hit area under 44pt, see Follow-ups |
| `chat-avatar-s5` | Mia, open profile | button | 44x44 | ok |
| `chat-author-s5` | Mia | button | 25x18 | hit area under 44pt, see Follow-ups |
| `chat-proposal-review-s5p` | Review | button | 83x44 | ok |
| `chat-replies-s5p` | 8 replies | button | 329x44 | ok |
| `chat-avatar-s6` | Ana, open profile | button | 44x44 | ok |
| `chat-author-s6` | Ana | button | 28x18 | hit area under 44pt, see Follow-ups |
| `chat-send` | Send message | button | 44x44 | ok |

### `cabals__create-cabal__ready`

| Control | Label read | Trait | Frame | Verdict |
| --- | --- | --- | --- | --- |
| `create-cover-picker` | Cover image, none | button | 353x110 | ok |
| `—` | Open | button | 180x44 | ok |
| `—` | Approval required | button selected | 180x44 | ok |
| `—` | Everyone | button selected | 180x44 | ok |
| `—` | People I pick | button | 180x44 | ok |
| `—` | 1 hour | button | 122x44 | ok |
| `—` | 1 day | button selected | 122x44 | ok |
| `—` | 1 week | button | 122x44 | ok |
| `create-group-submit` | Create cabal | button | 353x48 | ok |

### `cabals__discover__results`

| Control | Label read | Trait | Frame | Verdict |
| --- | --- | --- | --- | --- |
| `cabals-search-field` | Clear search | button | 44x44 | ok |
| `cabals-search-result-00000000-0000-7000-8000-0` | Weekend investors, 3 members | button | 393x72 | ok |
| `cabals-search-enter-00000000-0000-7000-8000-00` | Request to join Weekend investors | button | 83x44 | ok |
| `cabals-search-result-00000000-0000-7000-8000-0` | Weekend warriors, 1 member | button | 393x72 | ok |
| `cabals-search-enter-00000000-0000-7000-8000-00` | Request to join Weekend warriors | button | 83x44 | ok |
| `cabals-search-result-00000000-0000-7000-8000-0` | Weekend club, 2 members, Joined | button | 393x72 | ok |

### `cabals__holding__ready`

| Control | Label read | Trait | Frame | Verdict |
| --- | --- | --- | --- | --- |
| `holding-period-1W` | 1W | button | 75x46 | ok |
| `holding-period-1M` | 1M | button selected | 76x46 | ok |
| `holding-period-3M` | 3M | button | 75x46 | ok |
| `holding-period-1Y` | 1Y | button | 76x46 | ok |
| `holding-period-All` | All | button | 75x46 | ok |
| `holding-trade-trade-3` | Sell, 8.0000 units, Oct 12, 2026 · 14:02 UTC, $230.00 per unit, total $1,840.00 | button | 353x96 | ok |
| `holding-trade-trade-2` | Buy, 13.5556 units, Oct 02, 2026 · 09:41 UTC, $213.00 per unit, total $2,887.39 | button | 353x96 | ok |
| `holding-trade-trade-1` | Buy, 24.4444 units, Sep 25, 2026 · 10:18 UTC, $200.00 per unit, total $4,888.88 | button | 353x96 | ok |
| `holding-older-trades` | Show older trades | button | 353x48 | ok |
| `proposal-card-00000000-0000-4000-8000-00000000` | Kai: Voting open. Buy NVDA, $1,200.00 from the cabal pot. 2 yes votes needed · Closes in 23h | button | 353x196 | ok |
| `—` | Review | button | 75x44 | ok |
| `—` | 0 yes · 0 no · 3 not voted | button | 321x44 | ok |
| `holding-details` | Position details | button | 353x48 | ok |

### `cabals__invite__share`

| Control | Label read | Trait | Frame | Verdict |
| --- | --- | --- | --- | --- |
| `—` | Sheet Grabber | button | 72x23 | hit area under 44pt, see Follow-ups |
| `arrow.up` | Expand | button | 42x42 | hit area under 44pt, see Follow-ups |
| `xmark` | Close | button | 42x42 | hit area under 44pt, see Follow-ups |
| `share-card-share` | Share link | button | 338x46 | ok |
| `share-card-copy` | Copy link | button | 338x46 | ok |
| `cabal-invite-gain-row` | Share your gain as an image | button | 377x69 | ok |

### `cabals__members__members`

| Control | Label read | Trait | Frame | Verdict |
| --- | --- | --- | --- | --- |
| `members-tab-members` | Members | button selected | 180x44 | ok |
| `members-tab-voters` | Voters | button | 180x44 | ok |
| `members-list` | Kai, Admin | button | 393x72 | ok |
| `members-list` | Jordan, Member | button | 393x72 | ok |
| `members-list` | Priya, Member | button | 393x72 | ok |

### `cabals__public-cabal__preview`

| Control | Label read | Trait | Frame | Verdict |
| --- | --- | --- | --- | --- |
| `cabal-join-button` | Request to join | button | 353x48 | ok |
| `—` | Home | button | 74x54 | ok |
| `—` | Cabals | button selected | 81x54 | ok |
| `—` | Markets | button | 75x54 | ok |
| `—` | Feed | button | 74x54 | ok |
| `—` | Profile | button | 74x54 | ok |

### `explore__leaderboard__cabals`

| Control | Label read | Trait | Frame | Verdict |
| --- | --- | --- | --- | --- |
| `leaderboard-category` | Leaderboard category, Cabals | button | 208x44 | ok |
| `—` | (no label) | button | 208x44 | ok |
| `leaderboard-period` | Leaderboard period, 1M | button | 85x44 | ok |
| `leaderboard-filter` | Filter and sort leaderboard | button | 44x44 | ok |
| `leaderboard-info` | About this ranking | button | 44x44 | ok |
| `leaderboard-row-ape-society` | Rank 1, Up 4 places from the previous snapshot, Ape Society, 12 members, up 42.30 percent, up 12690 dollars | button | 393x74 | ok |
| `—` | First, Ape Society, up 42.30 percent, up 12690 dollars, up 4 places | button | 393x74 | ok |
| `leaderboard-row-quant-kids` | Rank 2, Down 1 place from the previous snapshot, Quant Kids, 9 members, up 37.10 percent, up 7420 dollars | button | 393x72 | ok |
| `—` | Rank 2, Quant Kids, up 37.10 percent, up 7420 dollars, down 1 place | button | 393x72 | ok |
| `leaderboard-row-dip-buyers` | Rank 3, Unchanged, Dip Buyers, 7 members, up 19.60 percent, up 3180 dollars | button | 393x72 | ok |
| `—` | Rank 3, Dip Buyers, up 19.60 percent, up 3180 dollars, unchanged | button | 393x72 | ok |
| `leaderboard-row-sunday-investors` | Rank 4, Up 2 places from the previous snapshot, Sunday Investors, 4 members, up 8.80 percent, up 1240 dollars | button | 393x71 | ok |
| `—` | Rank 4, Sunday Investors, up 8.80 percent, up 1240 dollars, up 2 places | button | 393x71 | ok |
| `leaderboard-row-moon-crew` | Rank 5, Down 3 places from the previous snapshot, Moon Crew, 15 members, down 2.10 percent, down 320 dollars | button | 393x72 | ok |
| `—` | Rank 5, Moon Crew, down 2.10 percent, down 320 dollars, down 3 places | button | 393x72 | ok |

### `feed__feed__all`

| Control | Label read | Trait | Frame | Verdict |
| --- | --- | --- | --- | --- |
| `feed-bell` | Activity notifications | button | 44x43 | ok |
| `feed-filter` | Filter feed | button | 56x35 | hit area under 44pt, see Follow-ups |
| `—` | (no label) | button | 56x36 | hit area under 44pt, see Follow-ups |
| `line.3.horizontal.decrease` | Filter feed | button | 44x43 | ok |
| `feed-actor-00000000-0000-7000-8000-0000000fe10` | Open Mia profile | button | 36x44 | hit area under 44pt, see Follow-ups |
| `—` | Open Mia profile | button | 28x44 | hit area under 44pt, see Follow-ups |
| `—` | NVDA, NVIDIA, $400.00, 2.00% of Big Ballers’s pot, Voting open, A small position alongside our existing holdin | button | 321x116 | ok |
| `feed-like-00000000-0000-7000-8000-0000000fe101` | Like Mia proposed buying $400.00 of NVIDIA in Big Ballers | button | 44x44 | ok |
| `feed-comments-00000000-0000-7000-8000-0000000f` | 8 comments | button | 44x44 | ok |
| `feed-share-00000000-0000-7000-8000-0000000fe10` | Share this update | button | 44x44 | ok |
| `feed-actor-00000000-0000-7000-8000-0000000fe10` | Open Leo profile | button | 36x44 | hit area under 44pt, see Follow-ups |
| `—` | Open Leo profile | button | 28x44 | hit area under 44pt, see Follow-ups |
| `—` | AAPL, Apple, $480.00, Sold 2 units · Avg $240.00, Executed | button | 321x70 | ok |
| `feed-like-00000000-0000-7000-8000-0000000fe102` | Like Tech Crew sold $480.00 of Apple | button | 44x44 | ok |
| `feed-comments-00000000-0000-7000-8000-0000000f` | 3 comments | button | 44x44 | ok |
| `feed-share-00000000-0000-7000-8000-0000000fe10` | Share this update | button | 44x44 | ok |
| `—` | NVDA, NVIDIA, +4.18%, 24h change, 24h token volume, $9.4M | button | 321x81 | ok |
| `feed-like-00000000-0000-7000-8000-0000000fe103` | Like NVIDIA is up 4.18% today | button | 44x44 | ok |
| `feed-comments-00000000-0000-7000-8000-0000000f` | 4 comments | button | 44x44 | ok |
| `feed-share-00000000-0000-7000-8000-0000000fe10` | Share this update | button | 44x44 | ok |
| `feed-actor-00000000-0000-7000-8000-0000000fe10` | Open Priya profile | button | 36x44 | hit area under 44pt, see Follow-ups |
| `—` | Open Priya profile | button | 40x44 | hit area under 44pt, see Follow-ups |
| `—` | AAPL · Replied to your proposal, How much cash would we have left after this? | button | 321x72 | ok |
| `feed-like-00000000-0000-7000-8000-0000000fe104` | Like Priya replied to your proposal | button | 44x44 | ok |
| `feed-comments-00000000-0000-7000-8000-0000000f` | 2 comments | button | 44x44 | ok |
| `feed-share-00000000-0000-7000-8000-0000000fe10` | Share this update | button | 44x44 | ok |
| `feed-actor-00000000-0000-7000-8000-0000000fe10` | Open Sam profile | button | 36x44 | hit area under 44pt, see Follow-ups |
| `—` | Open Sam profile | button | 35x44 | hit area under 44pt, see Follow-ups |
| `—` | MSFT, Microsoft, $350.00, 1.40% of Tech Crew’s pot, Voting open, Trim the position before earnings. | button | 321x96 | ok |
| `feed-like-00000000-0000-7000-8000-0000000fe105` | Like Sam proposed selling $350.00 of Microsoft in Tech Crew | button | 44x44 | ok |
| `feed-comments-00000000-0000-7000-8000-0000000f` | 1 comment | button | 44x44 | ok |
| `feed-share-00000000-0000-7000-8000-0000000fe10` | Share this update | button | 44x44 | ok |
| `feed-actor-00000000-0000-7000-8000-0000000fe10` | Open Mia profile | button | 36x44 | hit area under 44pt, see Follow-ups |
| `—` | MSFT, Microsoft, $250.00, 1.00% of Big Ballers’s pot, Execution pending | button | 321x104 | ok |
| `feed-like-00000000-0000-7000-8000-0000000fe106` | Like Mia’s proposal to buy $250.00 of Microsoft was approved | button | 44x44 | ok |
| `feed-comments-00000000-0000-7000-8000-0000000f` | 6 comments | button | 44x44 | ok |
| `feed-share-00000000-0000-7000-8000-0000000fe10` | Share this update | button | 44x44 | ok |
| `feed-actor-00000000-0000-7000-8000-0000000fe10` | Open Jordan profile | button | 36x44 | hit area under 44pt, see Follow-ups |
| `—` | Open Jordan profile | button | 55x44 | ok |
| `—` | Café Capital, New public cabal | button | 321x62 | ok |
| `feed-like-00000000-0000-7000-8000-0000000fe107` | Like Jordan started Café Capital | button | 44x44 | ok |
| `feed-comments-00000000-0000-7000-8000-0000000f` | 0 comments | button | 44x44 | ok |
| `feed-share-00000000-0000-7000-8000-0000000fe10` | Share this update | button | 44x44 | ok |

### `feed__post__ready`

| Control | Label read | Trait | Frame | Verdict |
| --- | --- | --- | --- | --- |
| `feed-post-author` | Open Mia profile, 12m · Big Ballers | button | 161x44 | ok |
| `—` | NVIDIA, Big Ballers, $400.00 | button | 353x72 | ok |
| `comment-avatar-c1` | (no label) | button | 44x43 | ok |
| `comment-author-c1` | Mia | button | 44x44 | ok |
| `comment-reply-c1` | Reply | button | 44x44 | ok |
| `comment-avatar-c2` | (no label) | button | 44x44 | ok |
| `comment-author-c2` | Alex | button | 50x44 | ok |
| `comment-reply-c2` | Reply | button | 44x44 | ok |
| `comment-composer-send` | Send comment | button | 44x44 | ok |

### `home__balances__ready`

| Control | Label read | Trait | Frame | Verdict |
| --- | --- | --- | --- | --- |
| `balances-deposit` | Deposit | button | 66x72 | ok |
| `balances-withdraw` | Withdraw | button | 67x72 | ok |
| `balances-activity` | Activity | button | 66x72 | ok |
| `balances-cabal-0f1d6a52-0001-7000-8000-0000000` | Big Ballers, Your share, $7,000.00 | button | 393x72 | ok |
| `balances-cabal-0f1d6a52-0001-7000-8000-0000000` | Tech Crew, Your share, $2,700.00 | button | 393x72 | ok |
| `balances-cabal-0f1d6a52-0001-7000-8000-0000000` | Underwater, Your share, $1,382.32 | button | 393x72 | ok |
| `balances-cash-row` | Cash, Not allocated to a cabal, $500.00 | button | 393x72 | ok |

### `home__home__loss`

| Control | Label read | Trait | Frame | Verdict |
| --- | --- | --- | --- | --- |
| `home-notifications-button` | Notifications | button | 44x43 | ok |
| `home-profile-button` | Profile | button | 44x43 | ok |
| `home-portfolio-range-1W` | Past week | button | 122x46 | ok |
| `home-portfolio-range-1M` | Past month | button selected | 122x46 | ok |
| `home-portfolio-range-ALL` | All time | button | 122x46 | ok |
| `—` | Net worth, $11,582.32 | button | 137x44 | ok |
| `—` | Cash, $500.00 | button | 103x44 | ok |
| `home-add-money-link` | Deposit | button | 87x44 | ok |
| `—` | See all | button | 60x44 | ok |
| `proposal-card-home-proposal-1` | Jordan: Needs your vote. Buy NVIDIA, $400.00 from the cabal pot. 5 yes votes needed · Closes in 11h | button | 353x196 | ok |
| `—` | Review | button | 83x44 | ok |
| `—` | 3 yes · 1 no · 4 not voted | button | 321x44 | ok |
| `cabal-cover-card-0f1d6a52-0001-7000-8000-00000` | Big Ballers, 8 members, 1 vote needed, your share $7,000.00, ownership 35.00%, your all-time P&L down 300 doll | button | 300x218 | ok |
| `square.and.arrow.up` | Share Big Ballers | button | 44x44 | ok |
| `cabal-cover-card-0f1d6a52-0001-7000-8000-00000` | Tech Crew, 5 members, your share $2,782.32, ownership 14.00%, your all-time P&L up 50 dollars, up 1.8 percent | button | 300x218 | ok |
| `square.and.arrow.up` | Share Tech Crew | button | 44x44 | ok |
| `cabal-cover-card-0f1d6a52-0001-7000-8000-00000` | Underwater, 9 members, your share $1,300.00, ownership 6.50%, your all-time P&L down 167 dollars 68 cents, dow | button | 300x218 | ok |
| `square.and.arrow.up` | Share Underwater | button | 44x44 | ok |
| `—` | View all | button | 68x44 | ok |
| `home-leaderboard-category` | Leaderboard category, Cabals | button | 259x44 | ok |
| `—` | (no label) | button | 259x44 | ok |
| `home-leaderboard-period` | Leaderboard period, 1M | button | 85x44 | ok |
| `home-leaderboard-row-ape-society` | First, Ape Society, up 42.10 percent, up 12690 dollars | button | 393x72 | ok |
| `home-leaderboard-row-quant-kids` | Rank 2, Quant Kids, up 37.10 percent, up 7420 dollars | button | 393x72 | ok |
| `home-leaderboard-row-dip-buyers` | Rank 3, Dip Buyers, up 19.60 percent, up 3180 dollars | button | 393x72 | ok |
| `—` | Markets | button | 71x44 | ok |
| `home-market-mover` | NVIDIA · +4.18%, 24h token move | button | 393x72 | ok |
| `—` | Home | button selected | 74x54 | ok |
| `—` | Cabals | button | 81x54 | ok |
| `—` | Feed | button | 74x54 | ok |
| `—` | Profile | button | 74x54 | ok |

### `home__notifications__unread`

| Control | Label read | Trait | Frame | Verdict |
| --- | --- | --- | --- | --- |
| `activity-mark-all-read` | Mark all read | button | 92x44 | ok |
| `activity-item-activity-reply` | Unread, Replied to your Apple proposal, How much cash would we have left after this?, Big Ballers · 2h | button | 353x120 | ok |
| `—` | Replied to your Apple proposal, How much cash would we have left after this?, Big Ballers · 2h | button | 353x120 | ok |
| `activity-item-activity-passed` | Unread, The Microsoft proposal passed, The proposal reached its vote threshold., Big Ballers · 3h, Execution p | button | 353x154 | ok |
| `—` | The Microsoft proposal passed, The proposal reached its vote threshold., Big Ballers · 3h, Execution pending | button | 353x154 | ok |
| `activity-item-activity-vote` | Unread, Voted on your NVIDIA proposal, Voted Yes on your proposal., Tech Crew · 4h | button | 353x98 | ok |
| `—` | Voted on your NVIDIA proposal, Voted Yes on your proposal., Tech Crew · 4h | button | 353x98 | ok |
| `activity-item-activity-deposit` | Deposit confirmed, $100.00 · Yesterday | button | 393x72 | ok |

### `markets__instrument__ready`

| Control | Label read | Trait | Frame | Verdict |
| --- | --- | --- | --- | --- |
| `asset-watch-star` | Remove from watchlist | button selected | 44x43 | ok |
| `asset-chart-interval` | Bar interval, 15m | button | 85x44 | ok |
| `—` | (no label) | button | 85x44 | ok |
| `asset-chart-style-candles` | Candles | button selected | 44x44 | ok |
| `asset-chart-style-line` | Line | button | 44x44 | ok |
| `asset-chart-ranges` | 1D | button selected | 55x46 | ok |
| `asset-chart-ranges` | 1W | button | 56x46 | ok |
| `asset-chart-ranges` | 1M | button | 55x46 | ok |
| `asset-chart-ranges` | 3M | button | 56x46 | ok |
| `asset-chart-ranges` | 6M | button | 56x46 | ok |
| `asset-chart-ranges` | 1Y | button | 55x46 | ok |
| `asset-chart-ranges` | ALL | button | 56x46 | ok |
| `asset-detail-propose-buy` | Propose a buy | button | 353x48 | ok |
| `asset-detail-propose-sell` | Propose a sell | button | 353x48 | ok |
| `asset-position-01890a5d-ac96-774b-bcce-b302099` | QA pot, 0.73 shares, $250.00, Profit and loss | button | 393x72 | ok |

### `markets__markets__ready`

| Control | Label read | Trait | Frame | Verdict |
| --- | --- | --- | --- | --- |
| `markets-mover-NVDAx` | NVIDIA, NVDA, 24 hour price change up 4.18 percent, volume opens soon | button | 244x112 | ok |
| `—` | NVIDIA, NVDA, up 4.18 percent, 24h volume, Opens soon | button | 244x112 | ok |
| `markets-mover-AMDx` | AMD, AMD, 24 hour price change up 3.05 percent, volume opens soon | button | 244x112 | ok |
| `—` | AMD, AMD, up 3.05 percent, 24h volume, Opens soon | button | 244x112 | ok |
| `markets-mover-TSLAx` | Tesla, TSLA, 24 hour price change down 2.40 percent, volume opens soon | button | 244x112 | ok |
| `—` | Tesla, TSLA, down 2.40 percent, 24h volume, Opens soon | button | 244x112 | ok |
| `markets-watchlist-manage` | Manage | button | 70x44 | ok |
| `markets-watchlist-row-AAPLx` | Apple, AAPL, $240.00, up 1.24 percent | button | 393x72 | ok |
| `markets-watchlist-row-NVDAx` | NVIDIA, NVDA, $220.00, up 4.18 percent | button | 393x72 | ok |
| `markets-watchlist-row-MSFTx` | Microsoft, MSFT, $350.00, up 0.62 percent | button | 393x72 | ok |
| `markets-popular-row-NVDAx` | NVIDIA, NVDA, $220.00, up 4.18 percent | button | 393x72 | ok |
| `markets-popular-row-AAPLx` | Apple, AAPL, $240.00, up 1.24 percent | button | 393x72 | ok |
| `markets-popular-row-TSLAx` | Tesla, TSLA, $262.50, down 2.40 percent | button | 393x72 | ok |
| `stocks-sort` | Sort stocks, Default | button | 170x44 | ok |
| `—` | (no label) | button | 170x44 | ok |
| `stocks-filter` | Filter stocks, All | button | 170x44 | ok |
| `assets-row-NVDAx` | NVIDIA, NVDA, $220.00, up 4.18 percent | button | 393x72 | ok |
| `assets-row-AAPLx` | Apple, AAPL, $240.00, up 1.24 percent | button | 393x72 | ok |
| `assets-row-TSLAx` | Tesla, TSLA, $262.50, down 2.40 percent | button | 393x72 | ok |
| `assets-row-MSFTx` | Microsoft, MSFT, $350.00, up 0.62 percent | button | 393x72 | ok |
| `assets-row-AMZNx` | Amazon, AMZN, $205.00, down 0.35 percent | button | 393x72 | ok |
| `assets-row-GOOGLx` | Alphabet, GOOGL, $180.00, down 0.41 percent | button | 393x72 | ok |
| `assets-row-METAx` | Meta, META, $585.00, up 0.88 percent | button | 393x72 | ok |
| `assets-row-AMDx` | AMD, AMD, $178.40, up 3.05 percent | button | 393x72 | ok |

### `markets__watchlist__ready`

| Control | Label read | Trait | Frame | Verdict |
| --- | --- | --- | --- | --- |
| `watchlist-row-NVDAx` | NVIDIA, NVDA | button | 297x44 | ok |
| `watchlist-star-NVDAx` | Remove NVIDIA | button selected | 44x44 | ok |
| `watchlist-row-AAPLx` | Apple, AAPL | button | 297x44 | ok |
| `watchlist-star-AAPLx` | Remove Apple | button selected | 44x44 | ok |
| `watchlist-row-TSLAx` | Tesla, TSLA | button | 297x44 | ok |
| `watchlist-star-TSLAx` | Add Tesla | button | 44x44 | ok |
| `watchlist-row-MSFTx` | Microsoft, MSFT | button | 297x44 | ok |
| `watchlist-star-MSFTx` | Remove Microsoft | button selected | 44x44 | ok |
| `watchlist-row-AMZNx` | Amazon, AMZN | button | 297x44 | ok |
| `watchlist-star-AMZNx` | Add Amazon | button | 44x44 | ok |
| `watchlist-row-GOOGLx` | Alphabet, GOOGL | button | 297x44 | ok |
| `watchlist-star-GOOGLx` | Add Alphabet | button | 44x44 | ok |
| `watchlist-row-METAx` | Meta, META | button | 297x44 | ok |
| `watchlist-star-METAx` | Add Meta | button | 44x44 | ok |
| `watchlist-row-AMDx` | AMD, AMD | button | 297x44 | ok |
| `watchlist-star-AMDx` | Add AMD | button | 44x44 | ok |

### `money__activity__ready`

| Control | Label read | Trait | Frame | Verdict |
| --- | --- | --- | --- | --- |
| `account-activity-filter-all` | All | button selected | 180x44 | ok |
| `account-activity-filter-pending` | Pending | button | 180x44 | ok |
| `account-activity-row-reference-deposit` | Deposit received, USDC · 2 Oct at 11:00 PM, +$100.00 | button | 393x72 | ok |
| `account-activity-row-reference-fund` | Deposit to Big Ballers, From cash · 1 Oct at 11:00 PM, −$250.00 | button | 393x72 | ok |
| `account-activity-row-reference-withdrawal` | Withdrawal, USDC · 30 Sep at 11:00 PM, −$100.00 | button | 393x72 | ok |

### `money__add-money__ready`

| Control | Label read | Trait | Frame | Verdict |
| --- | --- | --- | --- | --- |
| `deposit-card-row` | Card or Apple Pay | button | 393x72 | ok |
| `—` | Card or Apple Pay, Continue to the deposit provider | button | 393x72 | ok |
| `deposit-crypto-row` | Send crypto | button | 393x72 | ok |
| `—` | Send crypto, USDC on a supported network | button | 393x72 | ok |

### `money__card-funding__handoff`

| Control | Label read | Trait | Frame | Verdict |
| --- | --- | --- | --- | --- |
| `deposit-card-spinner` | Continue to provider | button | 353x48 | ok |
| `card-funding-choose-another-button` | Choose another method | button | 353x48 | ok |

### `money__cash-out__ready`

| Control | Label read | Trait | Frame | Verdict |
| --- | --- | --- | --- | --- |
| `cash-out-amount` | 25% | button | 64x44 | ok |
| `cash-out-amount` | 50% | button | 65x44 | ok |
| `cash-out-amount` | All | button | 49x44 | ok |
| `amount-keypad-1` | 1 | button | 112x48 | ok |
| `amount-keypad-2` | 2 | button | 112x48 | ok |
| `amount-keypad-3` | 3 | button | 112x48 | ok |
| `amount-keypad-4` | 4 | button | 112x48 | ok |
| `amount-keypad-5` | 5 | button | 112x48 | ok |
| `amount-keypad-6` | 6 | button | 112x48 | ok |
| `amount-keypad-7` | 7 | button | 112x48 | ok |
| `amount-keypad-8` | 8 | button | 112x48 | ok |
| `amount-keypad-9` | 9 | button | 112x48 | ok |
| `amount-keypad-dot` | Decimal point | button | 112x48 | ok |
| `amount-keypad-0` | 0 | button | 112x48 | ok |
| `amount-keypad-delete` | Delete | button | 112x48 | ok |
| `cash-out-review` | Review cash-out | button | 353x48 | ok |

### `money__deposit__address`

| Control | Label read | Trait | Frame | Verdict |
| --- | --- | --- | --- | --- |
| `deposit-address-copy-button` | Copy address | button | 321x48 | ok |

### `money__fund-cabal__ready`

| Control | Label read | Trait | Frame | Verdict |
| --- | --- | --- | --- | --- |
| `—` | $25 | button | 60x44 | ok |
| `—` | $50 | button | 60x44 | ok |
| `—` | $100 | button | 67x44 | ok |
| `—` | Max | button | 61x44 | ok |
| `amount-keypad-1` | 1 | button | 112x48 | ok |
| `amount-keypad-2` | 2 | button | 112x48 | ok |
| `amount-keypad-3` | 3 | button | 112x48 | ok |
| `amount-keypad-4` | 4 | button | 112x48 | ok |
| `amount-keypad-5` | 5 | button | 112x48 | ok |
| `amount-keypad-6` | 6 | button | 112x48 | ok |
| `amount-keypad-7` | 7 | button | 112x48 | ok |
| `amount-keypad-8` | 8 | button | 112x48 | ok |
| `amount-keypad-9` | 9 | button | 112x48 | ok |
| `amount-keypad-dot` | Decimal point | button | 112x48 | ok |
| `amount-keypad-0` | 0 | button | 112x48 | ok |
| `amount-keypad-delete` | Delete | button | 112x48 | ok |
| `fund-cabal-view` | Enter an amount | button | 353x48 | ok |

### `money__group-cash__ready`

| Control | Label read | Trait | Frame | Verdict |
| --- | --- | --- | --- | --- |
| `group-cash-propose` | Propose an investment | button | 353x48 | ok |
| `group-cash-holdings` | Return to holdings, View the whole cabal portfolio | button | 393x72 | ok |

### `money__network__ready`

| Control | Label read | Trait | Frame | Verdict |
| --- | --- | --- | --- | --- |
| `deposit-network-solana` | Solana | button | 393x71 | ok |
| `—` | Solana, Direct USDC deposit | button | 393x71 | ok |
| `deposit-network-base` | Base | button | 393x72 | ok |
| `—` | Base, Arrives in a few minutes | button | 393x72 | ok |
| `deposit-network-arbitrum` | Arbitrum | button | 393x72 | ok |
| `—` | Arbitrum, Arrives in a few minutes | button | 393x72 | ok |
| `deposit-network-ethereum` | Ethereum | button | 393x72 | ok |
| `—` | Ethereum, Arrives in a few minutes | button | 393x72 | ok |

### `money__transaction__completed`

| Control | Label read | Trait | Frame | Verdict |
| --- | --- | --- | --- | --- |
| `account-txn-receipt-activity` | Account activity, Return to your transfer history | button | 393x72 | ok |
| `account-txn-receipt-solscan` | View on explorer, Opens the confirmed transaction | button | 393x72 | ok |

### `money__withdraw__ready`

| Control | Label read | Trait | Frame | Verdict |
| --- | --- | --- | --- | --- |
| `—` | Max | button | 61x44 | ok |
| `—` | Paste | button | 66x34 | hit area under 44pt, see Follow-ups |
| `withdraw-network-picker` | Network, Solana | button | 353x44 | ok |
| `—` | (no label) | button | 353x44 | ok |
| `withdraw-continue-button` | Review withdrawal | button | 353x48 | ok |
| `—` | Home | button selected | 74x54 | ok |
| `—` | Cabals | button | 81x54 | ok |
| `—` | Markets | button | 75x54 | ok |
| `—` | Feed | button | 74x54 | ok |
| `—` | Profile | button | 74x54 | ok |
| `dictation` | Dictate | button | 68x68 | ok |

### `profile__edit-profile__ready`

| Control | Label read | Trait | Frame | Verdict |
| --- | --- | --- | --- | --- |
| `profile-photo-picker` | Edit profile photo | button | 76x76 | ok |
| `profile-name-save` | Save changes | button | 353x48 | ok |

### `profile__people__results`

| Control | Label read | Trait | Frame | Verdict |
| --- | --- | --- | --- | --- |
| `friends-tab-discover` | Discover | button selected | 180x44 | ok |
| `friends-tab-following` | Following | button | 180x44 | ok |
| `friends-search-mia` | Mia Chen, @mia | button | 393x72 | ok |
| `friends-search-follow-sample-mia` | Follow | button | 82x44 | ok |
| `friends-search-noah` | Noah Park, @noah | button | 393x72 | ok |
| `friends-search-follow-sample-noah` | Follow | button | 82x44 | ok |
| `friends-search-zoe` | Zoe Ruiz, @zoe | button | 393x72 | ok |
| `friends-search-follow-sample-zoe` | Follow | button | 82x44 | ok |

### `profile__person__public`

| Control | Label read | Trait | Frame | Verdict |
| --- | --- | --- | --- | --- |
| `user-profile-followers` | 12, followers | button | 84x44 | ok |
| `user-profile-following` | 8, following | button | 77x44 | ok |
| `user-profile-follow` | Follow | button | 353x48 | ok |
| `user-profile-public-activity` | Public activity, Latest proposals and discussions | button | 393x72 | ok |

### `profile__profile__ready`

| Control | Label read | Trait | Frame | Verdict |
| --- | --- | --- | --- | --- |
| `profile-settings-row` | Settings | button | 44x43 | ok |
| `profile-photo-picker` | Edit profile photo | button | 64x64 | ok |
| `profile-edit-button` | Edit name and profile details | button | 189x77 | ok |
| `profile-share-button` | Share your return | button | 44x44 | ok |
| `—` | Net worth, $1,248.50 | button | 137x44 | ok |
| `—` | Cash, $248.50 | button | 103x44 | ok |
| `profile-add-money-link` | Deposit | button | 87x44 | ok |
| `—` | See all | button | 60x44 | ok |
| `cabal-cover-card-01890a5d-ac96-774b-bcce-b3020` | Alpha, Cabal, your share $600.00, ownership 60.00%, your all-time P&L up 10 dollars, up 1.7 percent | button | 300x218 | ok |
| `square.and.arrow.up` | Share Alpha | button | 44x44 | ok |
| `cabal-cover-card-01890a5d-ac96-774b-bcce-b3020` | Sunday Investors, Cabal, your share $400.00, ownership 40.00%, your all-time P&L up 4 dollars, up 1.0 percent | button | 300x218 | ok |
| `square.and.arrow.up` | Share Sunday Investors | button | 44x44 | ok |
| `profile-activity-row` | Account activity, Deposits, withdrawals and receipts | button | 393x72 | ok |
| `profile-find-friends` | Find friends, Search people on Monaco | button | 393x72 | ok |
| `profile-invite-row` | Invite friends, Share Monaco or a cabal invitation | button | 393x71 | ok |
| `profile-withdraw-row` | Withdraw, Send cash to an external wallet | button | 393x72 | ok |

### `profile__settings__ready`

| Control | Label read | Trait | Frame | Verdict |
| --- | --- | --- | --- | --- |
| `settings-notifications` | Notifications, On | button | 393x72 | ok |
| `settings-edit-profile` | Edit profile, Name, photo and optional bio | button | 393x72 | ok |
| `settings-activity` | Account activity, Transfers and receipts | button | 393x72 | ok |
| `settings-withdraw` | Withdraw, Send cash to an external wallet | button | 393x72 | ok |
| `settings-blocked-people` | Blocked people, People you've blocked | button | 393x72 | ok |
| `settings-advanced` | Block explorers, Choose where transaction links open | button | 393x72 | ok |
| `profileSignOutButton` | Sign out, Return to sign-in | button | 393x72 | ok |
| `settings-delete-account` | Delete account, Close your account and leave your cabals | button | 393x72 | ok |

### `proposals__proposal__open`

| Control | Label read | Trait | Frame | Verdict |
| --- | --- | --- | --- | --- |
| `—` | Jordan, up 8.2 percent, 1M | button | 184x44 | ok |
| `proposal-votes-see-all` | See all | button | 60x43 | ok |
| `proposal-vote-no` | Vote No | button | 170x48 | ok |
| `proposal-vote-yes` | Vote Yes | button | 170x48 | ok |
| `comment-composer-send` | Send comment | button | 44x44 | ok |
| `—` | Home | button | 74x54 | ok |
| `—` | Cabals | button selected | 81x54 | ok |
| `—` | Markets | button | 75x54 | ok |
| `—` | Feed | button | 74x54 | ok |
| `—` | Profile | button | 74x54 | ok |
| `comment-avatar-c1` | (no label) | button | 44x44 | ok |
| `comment-author-c1` | Maya | button | 58x44 | ok |
| `comment-reply-c1` | Reply | button | 44x44 | ok |
| `comment-avatar-c2` | (no label) | button | 44x44 | ok |
| `comment-author-c2` | Alex | button | 50x44 | ok |
| `comment-reply-c2` | Reply | button | 44x44 | ok |
| `comment-avatar-c3` | (no label) | button | 44x44 | ok |
| `comment-author-c3` | Maya | button | 58x44 | ok |
| `comment-reply-c3` | Reply | button | 44x44 | ok |
| `comment-avatar-c4` | (no label) | button | 44x44 | ok |
| `comment-author-c4` | Sam | button | 51x44 | ok |
| `comment-reply-c4` | Reply | button | 44x44 | ok |

### `proposals__proposals__open`

| Control | Label read | Trait | Frame | Verdict |
| --- | --- | --- | --- | --- |
| `proposals-segment-open` | Open | button selected | 180x44 | ok |
| `proposals-segment-closed` | Closed | button | 180x44 | ok |
| `proposal-card-proposal-1` | Jordan: Needs your vote. Buy Alphabet, $25.00 from the cabal pot. 3 yes votes needed · Closes in 11h | button | 353x196 | ok |
| `—` | Review | button | 83x44 | ok |
| `—` | 1 yes · 0 no · 2 not voted | button | 321x44 | ok |
| `proposal-card-proposal-2` | Jordan: Needs your vote. Buy Alphabet, $25.00 from the cabal pot. 3 yes votes needed · Closes in 11h | button | 353x196 | ok |

### `proposals__propose__ready`

| Control | Label read | Trait | Frame | Verdict |
| --- | --- | --- | --- | --- |
| `propose-kind-buy` | Buy stock | button | 361x44 | ok |
| `propose-kind-sell` | Sell a holding | button | 361x44 | ok |
| `propose-kind-bot` | Add a trading bot, Trading bots open soon. | button | 361x55 | ok |
| `propose-chooser-cancel` | Cancel | button | 361x46 | ok |

### `proposals__review-proposal__ready`

| Control | Label read | Trait | Frame | Verdict |
| --- | --- | --- | --- | --- |
| `—` | Sheet Grabber | button | 72x23 | hit area under 44pt, see Follow-ups |
| `arrow.up` | Expand | button | 42x42 | hit area under 44pt, see Follow-ups |
| `xmark` | Close | button | 42x42 | hit area under 44pt, see Follow-ups |
| `propose-review-send` | Send proposal | button | 338x46 | ok |

### `proposals__select-stock__buy`

| Control | Label read | Trait | Frame | Verdict |
| --- | --- | --- | --- | --- |
| `propose-select-sort` | Sort stocks, Volume | button | 172x44 | ok |
| `—` | (no label) | button | 172x44 | ok |
| `propose-select-filter` | Filter stocks, All | button | 172x44 | ok |
| `propose-buy-stock-GOOGLx` | Alphabet, GOOGL | button | 393x72 | ok |
| `propose-buy-stock-tSpaceX` | SpaceX, tSpaceX | button | 393x72 | ok |
| `propose-buy-stock-NEWCO` | Newco, NEWCO | button | 393x72 | ok |

### `proposals__trade-amount__ready`

| Control | Label read | Trait | Frame | Verdict |
| --- | --- | --- | --- | --- |
| `—` | $25 | button | 60x44 | ok |
| `—` | $50 | button | 60x44 | ok |
| `—` | $100 | button | 67x44 | ok |
| `—` | Max | button | 61x44 | ok |
| `propose-amount-review` | Review proposal | button | 353x48 | ok |
| `propose-amount-reason-done` | Done | button | 62x36 | hit area under 44pt, see Follow-ups |
| `dictation` | Dictate | button | 68x68 | ok |

### `proposals__votes__open`

| Control | Label read | Trait | Frame | Verdict |
| --- | --- | --- | --- | --- |
| `—` | Sheet Grabber | button | 72x23 | hit area under 44pt, see Follow-ups |
| `xmark` | Close | button | 42x42 | hit area under 44pt, see Follow-ups |
| `proposal-voter-01890a5d-ac96-774b-bcce-b302099` | Kai voted yes | button | 377x69 | ok |
| `proposal-voter-01890a5d-ac96-774b-bcce-b302099` | Jordan hasn't voted | button | 377x69 | ok |
| `proposal-voter-01890a5d-ac96-774b-bcce-b302099` | Priya hasn't voted | button | 377x69 | ok |
