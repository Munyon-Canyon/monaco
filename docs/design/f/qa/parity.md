# F parity review

This page is the result of #6583's parity review of the 104 F launch (phase 1) frames: every app screen was shot on one Debug build of `staging` and set beside its frame at `docs/design/f/screens/`. Each frame is judged in the appearance it was drawn in; the other appearance, text sizes and VoiceOver are #6585, #6586 and #6587.

## How it was run

Build: `just build mobile` on `6583-f-parity-review` at `0c9caa91c` (`staging` at `9297866b2` plus the two `ParitySampleHarness` cases), Debug, lane simulator `Monaco 6583` (iPhone 17 Pro class, iOS 27.0, stock, not SimSlim: SimSlim is not installed on this Mac). One simulator, booted with `xcrun simctl boot` under `scripts/qa/xcode-lock.sh sim`, shut down at the end.

    udid="$(scripts/gold-sim-udid.sh)"
    out="${TMPDIR:-/tmp}/monaco-parity-f/run1"
    xcrun simctl ui "$udid" appearance dark
    MONACO_QA_SCREEN_ONLY="$(scripts/qa/parity.py frames --appearance dark --regex)" scripts/qa/screens.sh "$udid" <Debug Monaco.app> "$out/dark"
    xcrun simctl ui "$udid" appearance light
    MONACO_QA_SCREEN_ONLY="$(scripts/qa/parity.py frames --appearance light --regex)" scripts/qa/screens.sh "$udid" <Debug Monaco.app> "$out/light"
    xcrun simctl ui "$udid" appearance dark
    scripts/qa/parity.py sheet --shots "$out" --out "$out/sheet" --png --allow-missing 04-vote-picked 04-vote-holding

The dark run shot 91 lines and the light run the 5 twins, with no failed draw and no crash. `screens.sh` warned that three pairs are the same picture: `04-vote-committed` and `04-vote-idle` (`voteSettles` settles only after a tap), `04-proposal-sides` and `04-proposal-passed` (`closedSidesOn` and `nvdaPassed` load the same proposal), and `16-catch-up-empty-scrolled` and `16-catch-up-empty` (the caught-up page does not scroll). Those rows are `no harness` or record the shot they drew.

Pushes: `proposal_created_grouped.apns`, `cabal_paused_grouped.apns`, `proposal_created.apns`, `proposal_result_filled.apns`, `proposal_result_failed.apns`, `vote_reminder.apns`, `vote_deciding.apns`, `weekly_ledger.apns` and `post_reply.apns` were sent with `xcrun simctl push`. Every one was refused with `UNErrorDomain 2003: Source is not authorized` (the freshly installed app had no notification permission), and #6853, which closed #6565, attached no screenshots. The five `system` frames are recorded unshot; #7035 shoots them.

Shots and sheets stay outside the repo. The per-group PNG sheets are attached to the PR.

## Reading the verdicts

- `match`: the same order, components, copy and states as the frame.
- `match (15 pt floor)`: matches, and is taller only because the app floors row sub-lines at 15 pt (#4110), or differs only by a Context allowance: the real status bar, keyboard and tab bar; sample-harness data instead of the fixture's names and numbers; initials or an identity hue where the frame shows a photo; the real vote window; Q22's initials emblem; #6509's first-fund order.
- `no backend at launch`: the Treasury tab's bot row hidden and the stat block's "Bot Off" (#6511, Q29). On `03-cabal-treasury*` that gap is never a difference; those rows are `difference` only for their other gaps.
- `difference`: a different kind of fact, a missing or extra component, a different order, copy or state. The row says what each side shows.
- `no harness`: the state exists, but no harness line draws it (or the line draws another frame's state).
- `system`: a Lock Screen, Notification Center, expanded push or Face ID surface, judged from a push shot.

Every `difference` and `no harness` row links one follow-up issue per group; the issue names the owning ticket per row.

## Summary

104 launch frames.

| Verdict | Frames |
| --- | ---: |
| match | 0 |
| match (15 pt floor) | 14 |
| no backend at launch | 0 |
| difference | 77 |
| not built | 0 |
| no harness | 8 |
| system | 5 |

No launch screen shows a season row, a streak or a "Season" word. `10-profile-other` and `10-profile-other-cabals` show a Follow control, and so do their frames; #7030 sends it to the founder on #6410 as a phase 2 element.

## Boards (01)

| Frame | Appearance | Verdict | Difference | Follow-up |
| --- | --- | --- | --- | --- |
| [`01-tokens`](../screens/01-tokens.png) | dark | difference | Frame is one colour board (dark and light swatches with contrast ratios, identity hues, radii, spacing); app shows a role list with hex values only, no ratios, radii or spacing, and loss reads #FF6259 where the frame says #FF453A. | #7017 (#6432, #6440) built on the board; re-shoot pending |
| [`01-tokens-type`](../screens/01-tokens-type.png) | dark | difference | Frame lists each role with size/line height, where it is used, and the sample (cents in grey, plus Money hero $250 and Rules); app shows samples with names like "Typo.display", has no usage column, no grey cents on $20,000.00, no Money hero or Rules, and body/row sub lines are 15 pt. | #7017 (#6440) built on the board; re-shoot pending |
| [`01-tokens-controls`](../screens/01-tokens-controls.png) | dark | difference | Frame is a "which control where" table down to Crest and Toast (tally, pick/hold/committed, closed sides, stat row, ranked row, reactions, GIF); app shows only tabs, ranges, period header, chips, buttons, and extra network chips, then the Vote section cut off, with no usage notes. | #7017 (#6449, #6460, #6461, #6458, #6469, #6548) built on the board; re-shoot pending |
| [`01-chart-variants`](../screens/01-chart-variants.png) | dark | difference | Frame shows three fill variants side by side (flat tint, line only, gradient); app shows three performance lines (gain, loss, flat) stacked, with no line-only or gradient variant. | #7017 (#6450) built on the board; re-shoot pending |

founder:

## Home (02, 04b)

| Frame | Appearance | Verdict | Difference | Follow-up |
| --- | --- | --- | --- | --- |
| [`02-home-dark`](../screens/02-home-dark.png) | dark | difference | App shows Sep 29 / Oct 29 date labels under the chart and the bell has no circular button; frame has neither (all else matches, 15 pt floor). | #7018 (#6480, #6496) |
| [`02-home-light`](../screens/02-home-light.png) | light | difference | Same as dark: extra Sep 29 / Oct 29 date labels under the chart and a bare bell with no circular button. | #7018 (#6480, #6496) |
| [`02-home-scrolled`](../screens/02-home-scrolled.png) | dark | difference | Bare bell with no circular button; otherwise same order (cash, cabals, leaders card) within the 15 pt floor. | #7018 (#6480, #6496) |
| [`02-home-empty`](../screens/02-home-empty.png) | dark | difference | Start a cabal and Join with a link buttons lack the + and link icons, the bell has no circular button, and the empty emblem shows three people where the frame shows two. | #7018 (#6564) |
| [`04b-hold-1-pick`](../screens/04b-hold-1-pick.png) | dark | match (15 pt floor) | Ring on Yes and "Hold to vote · lift to cancel" match; cabal names, red fixture chart and scroll offset differ only by sample data. | — |
| [`04b-hold-2-holding`](../screens/04b-hold-2-holding.png) | dark | difference | Frame draws the fill plus a sweep arc along the top edge of Yes; app draws only the left fill, no sweep arc. | #7018 (#6508) |
| [`04b-hold-3-committed`](../screens/04b-hold-3-committed.png) | dark | match (15 pt floor) | "Voted Yes 5" and "Yours was the 5th yes · at 5 of 8 it passes" match; fixture names and bell badge differ. | — |

founder:

## Toasts and info sheet (12)

| Frame | Appearance | Verdict | Difference | Follow-up |
| --- | --- | --- | --- | --- |
| [`12-toast`](../screens/12-toast.png) | dark | match | Toast shows over Home (no route pushed) with the vote card collapsed to "You voted Yes ✓5 ✗1"; no title behind it. | #7019 (#6458, #6506) |
| [`12-info-sheet`](../screens/12-info-sheet.png) | dark | match | Sheet says "Solana, Base and Robinhood Chain ... trades 24/7" with Source CoinGecko, Updated <now>, Delay up to 1 min, no market-hours sentence, no stale banner behind. | #7019 (#6562) |

founder:

## Cabals tab and cabal screen (03)

| Frame | Appearance | Verdict | Difference | Follow-up |
| --- | --- | --- | --- | --- |
| [`03-cabals-tab`](../screens/03-cabals-tab.png) | dark | difference | Frame puts "3 · 1 invited" inline after "Your cabals"; app right-aligns it (rest matches). | #7020 (#6522) |
| [`03-cabals-tab-menu`](../screens/03-cabals-tab-menu.png) | dark | difference | Frame floats a small cabal header over a blurred tab with trailing icons and a gap before Invite/Share, header value green; app shows a large sheet with leading icons and grey value. | #7020 (#6560) |
| [`03-cabal-treasury`](../screens/03-cabal-treasury.png) | dark | difference | Frame has a "Treasury $20,000.00 −4.1% · 1M" line under the title and "6 replies" in the vote row; app has neither (Bot Off is no backend at launch). | #7020 (#6481) |
| [`03-cabal-treasury-light`](../screens/03-cabal-treasury-light.png) | light | difference | Same as dark: Treasury total line and "6 replies" missing (Bot Off is no backend at launch). | #7020 (#6481) |
| [`03-cabal-treasury-scrolled`](../screens/03-cabal-treasury-scrolled.png) | dark | difference | Frame vote row reads "6 replies"; app omits it, and Fund/Propose sits under a faded header; the trading-bot row is hidden (no backend at launch). | #7020 (#6481) |
| [`03-cabal-rules`](../screens/03-cabal-rules.png) | dark | difference | App Rules card runs edge to edge with larger rows (frame is inset, 16 pt margins); Treasury total line and "6 replies" missing. | #7020 (#6523, #6481) |
| [`03-cabal-rules-scrolled`](../screens/03-cabal-rules-scrolled.png) | dark | difference | Shot is not scrolled to the frame's state (no collapsed "AC Capital" title, no pinned Treasury line, footer cut off); card is full-bleed; "6 replies" missing. | #7020 (#6523, #6481) |
| [`03-cabal-rules-sides`](../screens/03-cabal-rules-sides.png) | dark | difference | Same as rules-scrolled; toggle on and "on since Oct" copy do match. | #7020 (#6523, #6481) |
| [`03-cabal-members`](../screens/03-cabal-members.png) | dark | difference | Frame shows "Andres · you" and a Treasury line, "6 replies"; app shows no "you" marker and none of the others. | #7020 (#6534, #6481) |
| [`03-cabal-members-scrolled`](../screens/03-cabal-members-scrolled.png) | dark | difference | Andres lacks "· you"; "6 replies" missing; See all / Invite rows are plain rows in the app, a tinted card with icons in the frame. | #7020 (#6534) |
| [`03-cabal-members-all`](../screens/03-cabal-members-all.png) | dark | difference | Admin rows (Andres, Christelle) lack the "N of M right · voted" line and "· you"; every row shows a stray second chevron. | #7020 (#6534) |
| [`03-cabal-empty`](../screens/03-cabal-empty.png) | dark | difference | Frame has "Treasury $600.00" line and a teal "Quiet in here" card; app has no Treasury line, a grey card, and the "Cabals like yours start with" heading is flush left of the margin. | #7020 (#6524) |

founder:

## Invite landing (18)

| Frame | Appearance | Verdict | Difference | Follow-up |
| --- | --- | --- | --- | --- |
| [`18-invite-landing`](../screens/18-invite-landing.png) | dark | match | Caption reads "You sign in at your first vote or fund." under the inline Join button below the vote card (#7021). | |

founder:

## Proposal (04)

| Frame | Appearance | Verdict | Difference | Follow-up |
| --- | --- | --- | --- | --- |
| [`04-proposal-dark`](../screens/04-proposal-dark.png) | dark | difference | Frame replies show a quoted-reply line ("Logan 44% NVDA...") and a heart reaction chip on Sam's reply; app replies (Maya, "View 2 replies", Sam "In.") have neither. | #7022 (#6491) |
| [`04-proposal-light`](../screens/04-proposal-light.png) | light | difference | Same as 04-proposal-dark: no quoted-reply line and no reaction chip on the replies. | #7022 (#6491) |
| [`04-proposal-scrolled`](../screens/04-proposal-scrolled.png) | dark | difference | Frame ends the reply list with an "All 6 replies >" link; the app shows none, and the replies lack the quote line and reaction chip. | #7022 (#6491) |
| [`04-vote-idle`](../screens/04-vote-idle.png) | dark | difference | Same page as 04-proposal-dark: replies lack the quoted-reply line and reaction chip. | #7022 (#6506) |
| [`04-vote-picked`](../screens/04-vote-picked.png) | dark | no harness | No harness line draws the picked state. | #7022 (#6506) |
| [`04-vote-holding`](../screens/04-vote-holding.png) | dark | no harness | No harness line draws the holding state. | #7022 (#6506) |
| [`04-vote-committed`](../screens/04-vote-committed.png) | dark | no harness | Harness line draws 04-vote-idle's state (voteSettles needs a tap); the shot shows an unvoted Yes/No card, not "Voted Yes 5 / Passed 5-1". | #7022 (#6506) |
| [`04-vote-closed`](../screens/04-vote-closed.png) | dark | difference | App shows "Passed 2-0" with a right-side check and a voter list (Kai, Jordan, "Priya did not vote"); frame shows "Passed 4-1" with a tally row (check 4, "5 of 6 voted", 1 x), no voter list, and "You voted Yes / 4th of 6" with its subline. | #7022 (#6532) |
| [`04-proposal-passed`](../screens/04-proposal-passed.png) | dark | difference | App shows the in-progress state ("Price / Of treasury / Passed" stats, "buying 1.82 sh - in progress", "Buying 1.82 sh In progress", voter list) where the frame shows the filled state (Fill / Shares / Now / Since, "filled at $219.40", "Bought ... fee"); "You voted Yes" lacks its subline and Monaco's system reply is not visible in the first reply slot. | #7022 (#6532, #6541) |
| [`04-proposal-sides`](../screens/04-proposal-sides.png) | dark | no harness | Harness line draws 04-proposal-passed's state (in-progress, initials, two voters); the frame's filled 5-1 result with photo voter rows by side is not drawn. | #7022 (#6539) |
| [`04-proposal-failed`](../screens/04-proposal-failed.png) | dark | difference | App shows "Failed 2-0 needed 2" with a voter list and a "Selling 2.2 sh In progress" row, no "Propose again" button, no "You voted No 2nd of 5" subline and no Monaco reply; frame shows "Failed 2-2 / needed 3 - no trade" with the tally row. | #7022 (#6532, #6541) |
| [`04-proposal-preipo`](../screens/04-proposal-preipo.png) | dark | difference | App titles "Buy tSpaceX" (frame "Buy SpaceX") with a wrapped three-line subtitle, truncates "Closes Fri 9:10...", squeezes the Last round / Secondary estimate rows, reads "fills when it passes" (frame "fills at the next tender, Jan 2027"), and shows an unvoted "QA pot members vote" tally where the frame shows "You voted Yes ... Change". | #7022 (#6550) |
| [`04-reply-composer`](../screens/04-reply-composer.png) | dark | difference | App draws an empty composer ("Say something" with a plus button and a keyboard-tip overlay) over a blank black screen; frame shows the proposal behind, a typed reply "Fine by me if we cap it at 45%", a green send button and the keyboard. | #7022 (#6548) |
| [`04-composer-menu`](../screens/04-composer-menu.png) | dark | difference | App shows only a blank screen with the composer bar; the Poll / Quote the thesis / Share a stock menu is not drawn. | #7022 (#6563, #6568) |
| [`04-gif-sheet`](../screens/04-gif-sheet.png) | dark | difference | App sheet sits over a blank screen ("posts=0") instead of the proposal, puts "Powered by GIPHY" next to the title instead of right-aligned, and shows placeholder tiles instead of photos. | #7022 (#6556, #6558) |
| [`04-gif-reply`](../screens/04-gif-reply.png) | dark | difference | App shows a "Replies 2" list (Mia, "View 1 reply") under a "posts=0" stub, not the proposal with Jordan's GIF reply (quote line, GIF badge, heart 2). | #7022 (#6556, #6558) |

founder:

## Moments (15)

| Frame | Appearance | Verdict | Difference | Follow-up |
| --- | --- | --- | --- | --- |
| [`15-first-fund`](../screens/15-first-fund.png) | dark | difference | The first-vote row subline truncates ("ends in...") where the frame shows "ends in 4h", "See all" has an extra chevron, and the member row order differs (Christelle, Andres, Logan, Mia, Jordan vs Christelle, Logan, Mia, Jordan, Andres). | #7023 (#6509) |
| [`15-passed`](../screens/15-passed.png) | dark | difference | App shows the plain proposal page with no moment sheet; the frame shows the "Buy NVDA - $400 / Passed / 5 x 1 / Mia called it" sheet with a Share button over the page. | #7023 (#6545) |
| [`15-leaderboard-moment`](../screens/15-leaderboard-moment.png) | dark | difference | App shows names under the top-three avatars, which the frame omits, and Sam's rank change reads "-" where the frame shows "down 1". | #7023 (#6526) |

founder:

## Permission sheet (19)

| Frame | Appearance | Verdict | Difference | Follow-up |
| --- | --- | --- | --- | --- |
| [`19-permission-sheet`](../screens/19-permission-sheet.png) | dark | difference | App puts the "Hear how votes end?" title top-left above the bell icon; frame centers the bell, then the centered title, then the copy. The page behind shows an unvoted Yes/No card, not "Voted Yes 5 / Passed 5-1". | #7024 (#6554) |

founder:

## Feed and post page (05)

| Frame | Appearance | Verdict | Difference | Follow-up |
| --- | --- | --- | --- | --- |
| [`05-feed-A`](../screens/05-feed-A.png) | dark | difference | Frame shows emoji reaction chips (💪 3, 👀 2, …) on every card and row 4 reads "Logan on Sell TSLA"; app shows only + and comment chips and "Logan on TSLA". | #7025 (#6469, #6500) |
| [`05-feed-A-scrolled`](../screens/05-feed-A-scrolled.png) | dark | difference | Frame cards carry reaction chips; app cards have none (only + and comment count). | #7025 (#6469) |
| [`05-feed-light`](../screens/05-feed-light.png) | light | difference | Frame shows reaction chips and "on Sell TSLA"; app shows no reaction chips and "on TSLA". | #7025 (#6469, #6500) |
| [`05-reactions`](../screens/05-reactions.png) | dark | difference | Frame opens the six-emoji tray under the NVDA proposal card in the feed; app draws bare one-line text rows ("Maya proposed buying $500 of AAPLx") with no feed cards. | #7025 (#6469) |
| [`05-emoji-picker`](../screens/05-emoji-picker.png) | dark | difference | Frame is a full-width sheet over the feed on the Smileys tab; app is an inset floating card over bare text rows, opened on Recents. | #7025 (#6494) |
| [`05-post`](../screens/05-post.png) | dark | match (15 pt floor) | — | — |
| [`05-post-scrolled`](../screens/05-post-scrolled.png) | dark | match (15 pt floor) | — | — |
| [`05-post-reply`](../screens/05-post-reply.png) | dark | difference | Frame is the reply sheet over the feed with draft text and an enabled send button, quoting "Logan sold"; app shows an empty "Say something" composer on a black screen quoting "Logan proposed". | #7025 (#6548, #6556, #6558) |
| [`05-comment-actions`](../screens/05-comment-actions.png) | n/a | no harness | No shot: the harness has no line for the comment-actions menu. | #7025 (#6552) |
| [`05-post-public`](../screens/05-post-public.png) | dark | difference | Frame ends with a "Members of Pilates reply here" footer and no composer or Reply links; app shows the "Say something" composer and Reply links to a non-member. | #7025 (#6553, #6506) |
| [`05-feed-empty`](../screens/05-feed-empty.png) | dark | difference | Frame's "From everyone" rows read "Buy AMZN $250" and "Buy Stripe $300"; app rows read bare "AMZN" and "STRIPE" with no side or amount. | #7025 (#6542) |

founder:

## Markets (06)

| Frame | Appearance | Verdict | Difference | Follow-up |
| --- | --- | --- | --- | --- |
| [`06-markets-A`](../screens/06-markets-A.png) | dark | match | — | — |
| [`06-markets-A-scrolled`](../screens/06-markets-A-scrolled.png) | dark | match (15 pt floor) | — | — |
| [`06-markets-search`](../screens/06-markets-search.png) | dark | match | — | — |
| [`06-markets-held`](../screens/06-markets-held.png) | dark | match | — | — |

founder:

## Stock (07)

| Frame | Appearance | Verdict | Difference | Follow-up |
| --- | --- | --- | --- | --- |
| [`07-stock-dark`](../screens/07-stock-dark.png) | dark | match | — | — |
| [`07-stock-light`](../screens/07-stock-light.png) | light | match | — | — |
| [`07-stock-preipo`](../screens/07-stock-preipo.png) | dark | difference | App now draws the hero block behind the logo, but the repo has no SpaceX photo asset, so the block is flat instead of the frame's photo. | #6410 (founder: supply the photo or accept the block) |
| [`07-stock-preipo-scrolled`](../screens/07-stock-preipo-scrolled.png) | dark | match (15 pt floor) | — | — |

founder:

## Review sheet (08)

| Frame | Appearance | Verdict | Difference | Follow-up |
| --- | --- | --- | --- | --- |
| [`08-review-sheet`](../screens/08-review-sheet.png) | dark | match | The sample draws the NVIDIA stock page behind the sheet and the empty "Why this trade? (optional)" row. | #7028 (#6567) |
| [`08-review-sheet-preipo`](../screens/08-review-sheet-preipo.png) | dark | match | The sample draws the SpaceX page behind the sheet, "Buy ≈ 2.70 sh at the estimate" and the Last round / Secondary estimate card. | #7028 (#6567) |

founder:

## Money (09)

| Frame | Appearance | Verdict | Difference | Follow-up |
| --- | --- | --- | --- | --- |
| [`09-fund-cabal-empty`](../screens/09-fund-cabal-empty.png) | dark | match (15 pt floor) | — | — |
| [`09-fund-cabal`](../screens/09-fund-cabal.png) | dark | match (15 pt floor) | — | — |
| [`09-fund-cabal-invalid`](../screens/09-fund-cabal-invalid.png) | dark | match (15 pt floor) | — | — |
| [`09-fund-cabal-settled`](../screens/09-fund-cabal-settled.png) | dark | match (15 pt floor) | — | #7029 |
| [`09-withdraw-empty`](../screens/09-withdraw-empty.png) | dark | match (15 pt floor) | — | #7029 |
| [`09-withdraw`](../screens/09-withdraw.png) | dark | match (15 pt floor) | — | #7029 |
| [`09-withdraw-invalid`](../screens/09-withdraw-invalid.png) | dark | match (15 pt floor) | — | #7029 |
| [`09-withdraw-settled`](../screens/09-withdraw-settled.png) | dark | match (15 pt floor) | Address abbreviation is sample data. | #7029 |

founder:

## Profile (10)

| Frame | Appearance | Verdict | Difference | Follow-up |
| --- | --- | --- | --- | --- |
| [`10-profile-own`](../screens/10-profile-own.png) | dark | no harness | Harness line draws the loading skeleton, not the ready profile. | #7030 (#6488) |
| [`10-profile-own-scrolled`](../screens/10-profile-own-scrolled.png) | dark | no harness | Harness line draws the loading skeleton, not the scrolled ready profile. | #7030 (#6505) |
| [`10-profile-other`](../screens/10-profile-other.png) | dark | difference | App has no back / share / more buttons, shows a Follow control, and the followers line has no "·" separators; frame has the three nav buttons. | #7030 (#6530) |
| [`10-profile-other-cabals`](../screens/10-profile-other-cabals.png) | dark | difference | Same: nav buttons missing, Follow control present, "·" separators missing; cabal emblems are initial tiles (allowed). | #7030 (#6530) |

founder:

## Invite and share (11)

| Frame | Appearance | Verdict | Difference | Follow-up |
| --- | --- | --- | --- | --- |
| [`11-invite-sheet`](../screens/11-invite-sheet.png) | dark | difference | Sheet content matches, but it sits over a cabal page (QA pot) instead of the Profile screen the frame shows. | #7031 (#6489) |
| [`11-share-A`](../screens/11-share-A.png) | dark | difference | Back button missing; chart marker labels sit differently (not on the frame's positions). | #7031 (#6503) |
| [`11-share-B`](../screens/11-share-B.png) | dark | difference | Back button missing, header sub-line truncated ("pass…"), and the chart is cut off with overlapping labels and a large empty gap. | #7031 (#6531) |
| [`11-share-C`](../screens/11-share-C.png) | dark | difference | App adds an "AC Capital · Your share" row between chart and P&L that the frame does not have; back button missing; "Join with code" absent as in frame. | #7031 (#6537) |
| [`11-share-card-vote`](../screens/11-share-card-vote.png) | dark | difference | "$400" is small and green in the app, large and white in the frame; back button missing. | #7031 (#6531) |
| [`11-share-sticker`](../screens/11-share-sticker.png) | dark | difference | Frame shows the minimal sticker on a checker backdrop with P&L, Buy/Sell and Code toggles hidden; app shows the full card text with all toggles on, no checker backdrop and a "Replay the position" row. | #7031 (#6537) |
| [`11-share-sticker-on`](../screens/11-share-sticker-on.png) | dark | difference | Frame shows the editor with the on-state sticker on the checker and toggles; app shows only a dashed-border sticker on black with no title, toggles or back button. | #7031 (#6537) |

founder:

## Leaderboard (13)

| Frame | Appearance | Verdict | Difference | Follow-up |
| --- | --- | --- | --- | --- |
| [`13-leaderboard-C-people`](../screens/13-leaderboard-C-people.png) | dark | match (15 pt floor) | Rival's name is missing under the rival avatar; top-three names added (small). | — |
| [`13-leaderboard-C`](../screens/13-leaderboard-C.png) | dark | match (15 pt floor) | — | — |
| [`13-leaderboard-C-scrolled`](../screens/13-leaderboard-C-scrolled.png) | dark | match (15 pt floor) | — | #7032 (#6484) |
| [`13-leaderboard-C-cabals`](../screens/13-leaderboard-C-cabals.png) | dark | match (15 pt floor) | — | #7032 (#6514) |
| [`13-leaderboard-C-unranked`](../screens/13-leaderboard-C-unranked.png) | dark | match (15 pt floor) | — | — |

founder:

## Ledger (14)

| Frame | Appearance | Verdict | Difference | Follow-up |
| --- | --- | --- | --- | --- |
| [`14-hold-sheet`](../screens/14-hold-sheet.png) | dark | difference | Sheet shows the proposal page behind it instead of Home; sheet's vote buttons sit in a separate large card with a different header. | #7033 (#6557, #6559) |
| [`14-wrap-sheet`](../screens/14-wrap-sheet.png) | dark | difference | Sheet sits over a "Couldn't load this view" error instead of Home; "+1.4%" caption is stacked below instead of inline. | #7033 (#6570) |
| [`14-ledger-story`](../screens/14-ledger-story.png) | dark | difference | App puts the +1.4% headline below the chart, adds a "monaco" wordmark above the card, and uses a purple placeholder cover with a glitch instead of the cabal photo; back button missing. | #7033 (#6581) |

founder:

## Catch up (16)

| Frame | Appearance | Verdict | Difference | Follow-up |
| --- | --- | --- | --- | --- |
| [`16-catch-up`](../screens/16-catch-up.png) | dark | difference | Top-right control is icon-only (frame has "One by one" label), reply rows lack the quote bar, and Vote/Join are green (frame gray). | #7034 (#6561) |
| [`16-catch-up-open`](../screens/16-catch-up-open.png) | dark | match (15 pt floor) | — | — |
| [`16-catch-up-open-scrolled`](../screens/16-catch-up-open-scrolled.png) | dark | match (15 pt floor) | — | — |
| [`16-catch-up-empty`](../screens/16-catch-up-empty.png) | dark | difference | Cabal emblem is a blank cyan tile with no initials and "Open Feed" uses a bolt glyph (frame: newspaper glyph). | #7034 (#6566) |
| [`16-catch-up-empty-scrolled`](../screens/16-catch-up-empty-scrolled.png) | dark | no harness | Harness line draws 16-catch-up-empty's state (not scrolled with compact title). | #7034 (#6566) |

founder:

## System frames

| Frame | Appearance | Verdict | Difference | Follow-up |
| --- | --- | --- | --- | --- |
| [`14-lock-screen-launch`](../screens/14-lock-screen-launch.png) | dark | system | Not shot: the simulator refused the push fixtures (no notification permission). | #7035 |
| [`19-notifications`](../screens/19-notifications.png) | dark | system | Not shot: the simulator refused the push fixtures (no notification permission). | #7035 |
| [`14-notification`](../screens/14-notification.png) | dark | system | Not shot: the simulator refused the push fixtures (no notification permission). | #7035 |
| [`14-faceid-alt`](../screens/14-faceid-alt.png) | dark | system | Not shot: the simulator refused the push fixtures (no notification permission). | #7035 |
| [`15-last-vote`](../screens/15-last-vote.png) | dark | system | Not shot: the simulator refused the push fixtures (no notification permission). | #7035 |

founder:

## Follow-ups

One issue per group, each listing the owner ticket for every row.

- #7017: Boards (01), owners #6432, #6440, #6449, #6450, #6458, #6460, #6461, #6469, #6548.
- #7018: Home (02, 04b), owners #6480, #6496, #6508, #6564.
- #7019: Toasts and info sheet (12), owners #6458, #6506, #6562.
- #7020: Cabals tab and cabal screen (03), owners #6481, #6522, #6523, #6524, #6534, #6560.
- #7021: Invite landing (18), owners #6502.
- #7022: Proposal (04), owners #6491, #6506, #6532, #6539, #6541, #6548, #6550, #6556, #6558, #6563, #6568.
- #7023: Moments (15), owners #6509, #6526, #6545.
- #7024: Permission sheet (19), owners #6554.
- #7025: Feed and post page (05), owners #6469, #6494, #6500, #6506, #6542, #6548, #6552, #6553, #6556, #6558.
- #7026: Markets (06), owners #6487, #6501, #6516.
- #7027: Stock (07), owners #6528, #6536, #6544.
- #7028: Review sheet (08), owners #6567.
- #7029: Money (09), owners #6475, #6476, #6504.
- #7030: Profile (10), owners #6488, #6505, #6530.
- #7031: Invite and share (11), owners #6489, #6503, #6531, #6537.
- #7032: Leaderboard (13), owners #6484, #6514.
- #7033: Ledger (14), owners #6557, #6559, #6570, #6581.
- #7034: Catch up (16), owners #6561, #6566.
- #7035: the five system frames, owners #6413, #6444, #6454, #6525, #6565.
