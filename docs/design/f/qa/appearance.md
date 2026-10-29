# F appearance review

This page is the result of #6585's sweep of the 95 F screens in the appearance each frame was not drawn in, plus Reduce Transparency, the contrast contract and the repo rules. #6583's parity review (`parity.md`) judged each frame in its own appearance; #6586 and #6587 cover text sizes and VoiceOver.

## How it was run

Build: `just build mobile` at `staging` `7dee8a432`, Debug, on simulator "Monaco 6585" (iOS 27.0, stock simulator: SimSlim is not installed on this Mac), headless, every step under `scripts/qa/xcode-lock.sh sim`.

Light pass, after `xcrun simctl ui <udid> appearance light`: 90 shot and 0 failed.

    MONACO_QA_SCREEN_ONLY="$(scripts/qa/parity.py frames --appearance dark --kind screen --regex)" scripts/qa/screens.sh

Dark pass of the 5 light twins (`02-home-light`, `03-cabal-treasury-light`, `04-proposal-light`, `05-feed-light`, `07-stock-light`) the same way: 5 shot.

Reduce Transparency: the simulator ignores the system switch, so each frame was launched with the flag from #6470, then shot after 7 s, glass against solid, in Light and Dark.

    xcrun simctl launch <udid> xyz.monacolabs.app <the frame's manifest arguments> [-MonacoReduceTransparency YES]
    xcrun simctl io <udid> screenshot <file>

Shots stay outside the repo.

Reshoot for #7108: `just build mobile` at `ee109c882` on simulator "Monaco 7108", each frame terminated, launched with its manifest arguments and screenshotted every 0.5 to 2 s for 6 to 20 s. `12-toast` was caught mid-hold in the first 2 to 3 s in Light and Dark, each with and without `-MonacoReduceTransparency YES`. The three catch-up frames stayed on Home's "Couldn't load your portfolio" in Light on that run. #7114 reshot them on simulator "Monaco 7114" at `8392ed2fa`: on the first launches after install the harness route had not reached Home's stack, and once the app had launched on that simulator `deck`, `deckScrolled` and `caughtUp` each drew within 2 s in Light. Terminate and relaunch a frame that shows Home before judging it.

Audit: `FAppearanceAuditUITests`, run with `test-without-building` in two holds, groups 02 to 08 and then 09 to 19, because one run of all 18 groups exceeds `xcode-lock`'s 1800 s sim hold. Run it one or a few groups at a time (`-only-testing:MonacoUITests/FAppearanceAuditUITests/test07Frames`).

## Screen frames

Each frame is judged in the appearance named in the second column against the frame in `docs/design/f/screens/`. The Reduce Transparency column is filled only for the 8 frames that were shot with the flag.

| Frame | Judged in | Verdict | Reduce Transparency | Difference | Follow-up |
| --- | --- | --- | --- | --- | --- |
| [`02-home-dark`](../screens/02-home-dark.png) | Light | match | match | — | — |
| [`02-home-empty`](../screens/02-home-empty.png) | Light | match | n/a | — | — |
| [`02-home-light (Dark)`](../screens/02-home-light.png) | Dark | match | n/a | — | — |
| [`02-home-scrolled`](../screens/02-home-scrolled.png) | Light | match | n/a | — | — |
| [`03-cabal-empty`](../screens/03-cabal-empty.png) | Light | match | n/a | — | — |
| [`03-cabal-members`](../screens/03-cabal-members.png) | Light | match | n/a | — | — |
| [`03-cabal-members-all`](../screens/03-cabal-members-all.png) | Light | match | n/a | — | — |
| [`03-cabal-members-scrolled`](../screens/03-cabal-members-scrolled.png) | Light | match | n/a | — | — |
| [`03-cabal-rules`](../screens/03-cabal-rules.png) | Light | match | n/a | — | — |
| [`03-cabal-rules-scrolled`](../screens/03-cabal-rules-scrolled.png) | Light | match | n/a | — | — |
| [`03-cabal-rules-sides`](../screens/03-cabal-rules-sides.png) | Light | match | n/a | — | — |
| [`03-cabal-treasury`](../screens/03-cabal-treasury.png) | Light | match | n/a | — | — |
| [`03-cabal-treasury-light (Dark)`](../screens/03-cabal-treasury-light.png) | Dark | match | n/a | — | — |
| [`03-cabal-treasury-scrolled`](../screens/03-cabal-treasury-scrolled.png) | Light | match | n/a | — | — |
| [`03-cabals-tab`](../screens/03-cabals-tab.png) | Light | match | n/a | — | — |
| [`03-cabals-tab-menu`](../screens/03-cabals-tab-menu.png) | Light | match | n/a | — | — |
| [`04-composer-menu`](../screens/04-composer-menu.png) | Light | difference | n/a | The harness draws the wrong state in both appearances, a known parity gap. Not a colour defect. | #7022 |
| [`04-gif-reply`](../screens/04-gif-reply.png) | Light | difference | n/a | The harness draws the wrong state in both appearances, a known parity gap. Not a colour defect. | #7022 |
| [`04-gif-sheet`](../screens/04-gif-sheet.png) | Light | match | n/a | — | — |
| [`04-proposal-dark`](../screens/04-proposal-dark.png) | Light | match | match: Content behind is plain; nothing shows through. | — | — |
| [`04-proposal-failed`](../screens/04-proposal-failed.png) | Light | match | n/a | — | — |
| [`04-proposal-light (Dark)`](../screens/04-proposal-light.png) | Dark | match | n/a | — | — |
| [`04-proposal-passed`](../screens/04-proposal-passed.png) | Light | match | n/a | — | — |
| [`04-proposal-preipo`](../screens/04-proposal-preipo.png) | Light | match | n/a | — | — |
| [`04-proposal-scrolled`](../screens/04-proposal-scrolled.png) | Light | match | n/a | — | — |
| [`04-proposal-sides`](../screens/04-proposal-sides.png) | Light | match | n/a | — | — |
| [`04-reply-composer`](../screens/04-reply-composer.png) | Light | match | n/a | — | — |
| [`04-vote-closed`](../screens/04-vote-closed.png) | Light | match | n/a | — | — |
| [`04-vote-committed`](../screens/04-vote-committed.png) | Light | match | n/a | — | — |
| [`04-vote-holding`](../screens/04-vote-holding.png) | Light | match | n/a | — | — |
| [`04-vote-idle`](../screens/04-vote-idle.png) | Light | match | n/a | — | — |
| [`04-vote-picked`](../screens/04-vote-picked.png) | Light | match | n/a | — | — |
| [`04b-hold-1-pick`](../screens/04b-hold-1-pick.png) | Light | match | n/a | — | — |
| [`04b-hold-2-holding`](../screens/04b-hold-2-holding.png) | Light | match | n/a | — | — |
| [`04b-hold-3-committed`](../screens/04b-hold-3-committed.png) | Light | match | n/a | — | — |
| [`05-comment-actions`](../screens/05-comment-actions.png) | Light | match | n/a | — | — |
| [`05-emoji-picker`](../screens/05-emoji-picker.png) | Light | match | match: Content behind is plain; nothing shows through. | — | — |
| [`05-feed-A`](../screens/05-feed-A.png) | Light | difference | n/a | Crest is a bare identity-hue tile with no initials (CabalCover .thumb), in both appearances. | #7087 |
| [`05-feed-A-scrolled`](../screens/05-feed-A-scrolled.png) | Light | difference | n/a | Crest is a bare identity-hue tile with no initials (CabalCover .thumb), in both appearances. | #7087 |
| [`05-feed-empty`](../screens/05-feed-empty.png) | Light | match | n/a | — | — |
| [`05-feed-light (Dark)`](../screens/05-feed-light.png) | Dark | difference | n/a | Crest is a bare identity-hue tile with no initials (CabalCover .thumb), in both appearances. | #7087 |
| [`05-post`](../screens/05-post.png) | Light | match | n/a | — | — |
| [`05-post-public`](../screens/05-post-public.png) | Light | difference | n/a | Crest is a bare identity-hue tile with no initials (CabalCover .thumb), in both appearances. | #7087 |
| [`05-post-reply`](../screens/05-post-reply.png) | Light | match | n/a | — | — |
| [`05-post-scrolled`](../screens/05-post-scrolled.png) | Light | match | n/a | — | — |
| [`05-reactions`](../screens/05-reactions.png) | Light | match | n/a | — | — |
| [`06-markets-A`](../screens/06-markets-A.png) | Light | match | n/a | — | — |
| [`06-markets-A-scrolled`](../screens/06-markets-A-scrolled.png) | Light | match | n/a | — | — |
| [`06-markets-held`](../screens/06-markets-held.png) | Light | match | n/a | — | — |
| [`06-markets-search`](../screens/06-markets-search.png) | Light | match | n/a | — | — |
| [`07-stock-dark`](../screens/07-stock-dark.png) | Light | difference | difference: The toolbar-placed rounds do not read the flag. | Crest is a bare identity-hue tile with no initials (CabalCover .thumb), in both appearances. | #7087 #7091 |
| [`07-stock-light (Dark)`](../screens/07-stock-light.png) | Dark | difference | n/a | Crest is a bare identity-hue tile with no initials (CabalCover .thumb), in both appearances. | #7087 |
| [`07-stock-preipo`](../screens/07-stock-preipo.png) | Light | difference | n/a | Crest is a bare identity-hue tile with no initials (CabalCover .thumb), in both appearances. | #7087 |
| [`07-stock-preipo-scrolled`](../screens/07-stock-preipo-scrolled.png) | Light | difference | n/a | Crest is a bare identity-hue tile with no initials (CabalCover .thumb), in both appearances. | #7087 |
| [`08-review-sheet`](../screens/08-review-sheet.png) | Light | match | difference: The system half-sheet body does not read the flag. | — | #7091 |
| [`08-review-sheet-preipo`](../screens/08-review-sheet-preipo.png) | Light | match | n/a | — | — |
| [`09-fund-cabal`](../screens/09-fund-cabal.png) | Light | match | n/a | — | — |
| [`09-fund-cabal-empty`](../screens/09-fund-cabal-empty.png) | Light | match | n/a | — | — |
| [`09-fund-cabal-invalid`](../screens/09-fund-cabal-invalid.png) | Light | match | n/a | — | — |
| [`09-fund-cabal-settled`](../screens/09-fund-cabal-settled.png) | Light | match | n/a | — | — |
| [`09-withdraw`](../screens/09-withdraw.png) | Light | match | match | — | — |
| [`09-withdraw-empty`](../screens/09-withdraw-empty.png) | Light | match | n/a | — | — |
| [`09-withdraw-invalid`](../screens/09-withdraw-invalid.png) | Light | match | n/a | — | — |
| [`09-withdraw-settled`](../screens/09-withdraw-settled.png) | Light | match | n/a | — | — |
| [`10-profile-other`](../screens/10-profile-other.png) | Light | match | n/a | — | — |
| [`10-profile-other-cabals`](../screens/10-profile-other-cabals.png) | Light | match | n/a | — | — |
| [`10-profile-own`](../screens/10-profile-own.png) | Light | match | match | — | — |
| [`10-profile-own-scrolled`](../screens/10-profile-own-scrolled.png) | Light | match | n/a | — | — |
| [`11-invite-sheet`](../screens/11-invite-sheet.png) | Light | match | n/a | — | — |
| [`11-share-A`](../screens/11-share-A.png) | Light | match | n/a | The share card is drawn dark on purpose (see the note under the table). | — |
| [`11-share-B`](../screens/11-share-B.png) | Light | match | n/a | The share card is drawn dark on purpose (see the note under the table). | — |
| [`11-share-C`](../screens/11-share-C.png) | Light | match | n/a | The share card is drawn dark on purpose (see the note under the table). | — |
| [`11-share-card-vote`](../screens/11-share-card-vote.png) | Light | match | n/a | The share card is drawn dark on purpose (see the note under the table). | — |
| [`11-share-sticker`](../screens/11-share-sticker.png) | Light | match | n/a | The share card is drawn dark on purpose (see the note under the table). | — |
| [`11-share-sticker-on`](../screens/11-share-sticker-on.png) | Light | match | n/a | The share card is drawn dark on purpose (see the note under the table). | — |
| [`12-info-sheet`](../screens/12-info-sheet.png) | Light | match | n/a | — | — |
| [`12-toast`](../screens/12-toast.png) | Light | match | match: Light and Dark, with and without Reduce Transparency, the capsule shows "Voted Yes. Passed 5–1." legible on its fill; solid it drops the blur and keeps the same tint. | — | — |
| [`13-leaderboard-C`](../screens/13-leaderboard-C.png) | Light | match | n/a | — | — |
| [`13-leaderboard-C-cabals`](../screens/13-leaderboard-C-cabals.png) | Light | match | n/a | — | — |
| [`13-leaderboard-C-people`](../screens/13-leaderboard-C-people.png) | Light | match | n/a | — | — |
| [`13-leaderboard-C-scrolled`](../screens/13-leaderboard-C-scrolled.png) | Light | match | n/a | — | — |
| [`13-leaderboard-C-unranked`](../screens/13-leaderboard-C-unranked.png) | Light | match | n/a | — | — |
| [`14-hold-sheet`](../screens/14-hold-sheet.png) | Light | match | n/a | — | — |
| [`14-ledger-story`](../screens/14-ledger-story.png) | Light | match | n/a | The story frame is drawn dark on purpose and reads the same in Light: green figure, white labels and the Share and Copy image buttons on black. The sample uses a placeholder cover and an initial for the avatar. | — |
| [`14-wrap-sheet`](../screens/14-wrap-sheet.png) | Light | match | n/a | The share card is drawn dark on purpose (see the note under the table). | — |
| [`15-first-fund`](../screens/15-first-fund.png) | Light | match | n/a | The "You're in." card is the cabal's soft tint, as in the dark frame. | — |
| [`15-leaderboard-moment`](../screens/15-leaderboard-moment.png) | Light | match | n/a | — | — |
| [`15-passed`](../screens/15-passed.png) | Light | match | n/a | — | — |
| [`16-catch-up`](../screens/16-catch-up.png) | Light | difference | n/a | Crest is a bare identity-hue tile with no initials (CabalCover .thumb), in both appearances. | #7087 |
| [`16-catch-up-empty`](../screens/16-catch-up-empty.png) | Light | match | n/a | Caught-up card, Open Feed and Cleared today read as in the dark frame. The sample uses initials crests and avatars in place of photos. | — |
| [`16-catch-up-empty-scrolled`](../screens/16-catch-up-empty-scrolled.png) | Light | difference | n/a | Crest is a bare identity-hue tile with no initials (CabalCover .thumb), in both appearances. | #7087 |
| [`16-catch-up-open`](../screens/16-catch-up-open.png) | Light | match | n/a | Deck card 2 of 6, Up next and Keep and Clear read as in the dark frame. The sample uses initials crests and avatars in place of photos. | — |
| [`16-catch-up-open-scrolled`](../screens/16-catch-up-open-scrolled.png) | Light | match | n/a | The deck card scrolled under the 2 of 6 bar, with Up next through Sell TSLA, reads as in the dark frame. The sample uses initials crests and avatars in place of photos. | — |
| [`18-invite-landing`](../screens/18-invite-landing.png) | Light | match | n/a | The stat and vote cards use `surfaceSunken`, as in the dark frame, which #6583 matched. | — |
| [`19-permission-sheet`](../screens/19-permission-sheet.png) | Light | match | n/a | — | — |

The share card (`11-share-*`, `14-wrap-sheet`) is drawn dark on purpose and does not follow the appearance. The system tab bar follows only the device setting (#6470), so check it once on a phone.

## Contrast pairs

| Pair | On | Bar | Light | Dark | Result |
|---|---|---|---|---|---|
| primaryText | canvas | 4.5 | 21.00 | 21.00 | pass |
| secondaryText | canvas | 4.5 | 5.99 | 8.16 | pass |
| tint | canvas | 4.5 | 7.93 | 7.64 | pass |
| flat | canvas | 4.5 | 5.99 | 8.16 | pass |
| warning | canvas | 4.5 | 5.68 | 12.53 | pass |
| destructive | canvas | 4.5 | 6.06 | 7.14 | pass |
| profit | canvas | 4.5 | 5.68 | 14.47 | pass |
| loss | canvas | 4.5 | 6.06 | 7.14 | pass |
| primaryText | surface | 4.5 | 18.82 | 17.01 | pass |
| secondaryText | surface | 4.5 | 5.37 | 6.61 | pass |
| tint | surface | 4.5 | 7.11 | 6.19 | pass |
| flat | surface | 4.5 | 5.37 | 6.61 | pass |
| warning | surface | 4.5 | 5.09 | 10.15 | pass |
| destructive | surface | 4.5 | 5.43 | 5.79 | pass |
| profit | surface | 4.5 | 5.09 | 11.72 | pass |
| loss | surface | 4.5 | 5.43 | 5.79 | pass |
| primaryText | surfaceSunken | 4.5 | 16.73 | 13.94 | pass |
| secondaryText | surfaceSunken | 4.5 | 4.77 | 5.42 | pass |
| tint | surfaceSunken | 4.5 | 6.32 | 5.07 | pass |
| flat | surfaceSunken | 4.5 | 4.77 | 5.42 | pass |
| warning | surfaceSunken | 4.5 | 4.52 | 8.31 | pass |
| destructive | surfaceSunken | 4.5 | 4.83 | 4.74 | pass |
| profit | surfaceSunken | 4.5 | 4.52 | 9.60 | pass |
| loss | surfaceSunken | 4.5 | 4.83 | 4.74 | pass |
| secondaryText on warningWash | canvas+warningWash | 4.5 | 5.08 | 5.73 | pass |
| warning on warningWash | canvas+warningWash | 4.5 | 4.81 | 8.79 | pass |
| voteNeededLabel | voteNeededFill | 4.5 | 5.38 | 5.38 | pass |
| onDestructive | destructive | 4.5 | 6.06 | 7.14 | pass |
| CabalTint onFill (min of 8; worst hue indigo 5E5CE6, onFill white) | tint.fill | 4.5 | 5.06 | 5.06 | pass |
| onCover (min over 9 bases x 2 overlays; worst white+coverOverlay) | base+overlay | 4.5 | 10.56 | 10.56 | pass |
| onCoverMuted (min over 9 bases x 2 overlays; worst white+coverOverlay) | base+overlay | 4.5 | 6.71 | 6.71 | pass |
| profitOnCover (min over 9 bases x 2 overlays; worst white+coverOverlay) | base+overlay | 4.5 | 7.66 | 7.66 | pass |
| lossOnCover (min over 9 bases x 2 overlays; worst white+coverOverlay) | base+overlay | 4.5 | 6.16 | 6.16 | pass |
| onBrand | brandFill | 4.5 | 7.93 | 7.93 | pass |
| primaryButtonLabel | primaryButtonFill | 4.5 | 7.93 | 7.93 | pass |
| toastLabel | toastFill | 4.5 | 18.82 | 17.01 | pass |
| toastSuccessGlyph | toastFill | 4.5 | 5.09 | 11.72 | pass |
| toastErrorGlyph | toastFill | 4.5 | 5.43 | 5.79 | pass |
| StatusBadge positive | canvas+brandSoft | 4.5 | 6.32 | 5.07 | pass |
| StatusBadge negative | canvas+sellWash | 4.5 | 5.43 | 5.79 | pass |
| StatusBadge warn | canvas+warningWash | 4.5 | 4.81 | 8.79 | pass |
| StatusBadge neutral | canvas+surfaceSunken | 4.5 | 4.77 | 5.42 | pass |
| StatusBadge attention | canvas+voteNeededFill | 4.5 | 5.38 | 5.38 | pass |
| StatusBadge discussion | canvas+discussionWash | 4.5 | 6.32 | 5.07 | pass |
| StatusBadge positive | surface+brandSoft | 4.5 | 6.32 | 5.07 | pass |
| StatusBadge negative | surface+sellWash | 4.5 | 5.43 | 5.79 | pass |
| StatusBadge warn | surface+warningWash | 4.5 | 4.81 | 8.79 | pass |
| StatusBadge neutral | surface+surfaceSunken | 4.5 | 4.77 | 5.42 | pass |
| StatusBadge attention | surface+voteNeededFill | 4.5 | 5.38 | 5.38 | pass |
| StatusBadge discussion | surface+discussionWash | 4.5 | 6.32 | 5.07 | pass |
| EventPill proposal | canvas+brandSoft | 4.5 | 6.32 | 5.07 | pass |
| EventPill cabal | canvas+brandSoft | 4.5 | 6.32 | 5.07 | pass |
| EventPill buy | canvas+buyWash | 4.5 | 5.09 | 11.72 | pass |
| EventPill sell | canvas+sellWash | 4.5 | 5.43 | 5.79 | pass |
| EventPill priceMove | canvas+warningWash | 4.5 | 4.81 | 8.79 | pass |
| EventPill approved | canvas+warningWash | 4.5 | 4.81 | 8.79 | pass |
| EventPill discussion | canvas+discussionWash | 4.5 | 6.32 | 5.07 | pass |
| EventPill proposal | surface+brandSoft | 4.5 | 6.32 | 5.07 | pass |
| EventPill cabal | surface+brandSoft | 4.5 | 6.32 | 5.07 | pass |
| EventPill buy | surface+buyWash | 4.5 | 5.09 | 11.72 | pass |
| EventPill sell | surface+sellWash | 4.5 | 5.43 | 5.79 | pass |
| EventPill priceMove | surface+warningWash | 4.5 | 4.81 | 8.79 | pass |
| EventPill approved | surface+warningWash | 4.5 | 4.81 | 8.79 | pass |
| EventPill discussion | surface+discussionWash | 4.5 | 6.32 | 5.07 | pass |
| chart profitVivid | canvas | 3 | 4.81 | 14.47 | pass |
| chart lossVivid | canvas | 3 | 5.38 | 6.16 | pass |
| chart flat chart line | canvas | 3 | 5.99 | 8.16 | pass |
| chart profitVivid | surface | 3 | 4.31 | 11.72 | pass |
| chart lossVivid | surface | 3 | 4.83 | 4.99 | pass |
| chart flat chart line | surface | 3 | 5.37 | 6.61 | pass |
| VoteBar yes | canvas | 3 | 4.81 | 14.47 | pass |
| VoteBar no | canvas | 3 | 5.38 | 6.16 | pass |
| AllocationBar segment 0 vs canvas | canvas | 3 | 7.93 | 7.64 | pass |
| AllocationBar segment 0 vs cash track | border | 3 | 4.65 | 4.26 | pass |
| AllocationBar segment 1 vs canvas | canvas | 3 | 7.56 | 5.76 | pass |
| AllocationBar segment 1 vs cash track | border | 3 | 4.43 | 3.21 | pass |
| AllocationBar segment 2 vs canvas | canvas | 3 | 6.04 | 5.96 | pass |
| AllocationBar segment 2 vs cash track | border | 3 | 3.54 | 3.32 | pass |
| VoteTallyBar winner fill (primaryText) | surfaceSunken | 3 | 16.73 | 13.94 | pass |
| VoteTallyBar idle fill (secondaryText) | surfaceSunken | 3 | 4.77 | 5.42 | pass |
| MonacoUnderlineTabs underline (primaryText) | canvas | 3 | 21.00 | 21.00 | pass |
| hairline | canvas | 1.1 | 1.71 | 1.79 | pass |
| disabledLabel (need 2.5<=r<4.5; secondaryText 5.99/8.16, gap>1) | canvas | 2.5-4.5 | 3.26 | 4.14 | pass |
| disabledLabel (need 2.5<=r<4.5; secondaryText 5.37/6.61, gap>1) | surface | 2.5-4.5 | 2.92 | 3.36 | pass |
| disabledLabel (need 2.5<=r<4.5; secondaryText 4.77/5.42, gap>1) | surfaceSunken | 2.5-4.5 | 2.60 | 2.75 | pass |

## FAIL rows

None.

Computed from MonacoTheme's hex values by the WCAG formula, the same as `WCAGContrast`, and asserted by `DesignContrastTests` (the F tally and underline pairs were added in #7059). MonacoTests are not run locally (operator rule); the hourly staging indicator runs them.

## Exempt by the contract

| Role | Contract reason | Where the sweep saw it |
| --- | --- | --- |
| `tertiaryText` | Glyphs only (a row's chevron when the title says where it goes, the "–" before "did not vote"); 1.4.11 covers only marks needed to understand content. | No text in tertiary in any of the 95 shots. |
| Crown | The rank and the accessibility label ("first") say it. | Beside a printed rank on board rows. On member avatars (`CabalMemberRow`, `FirstFundView`), the podium and the profile header there is no printed rank, but every crown is labelled ("Month leader", "leads this month") or sits in an element whose label is `row.spoken`, so the exemption holds through the label. |
| Identity hue as a fill on the canvas | The cabal's name sits beside every crest and is its accessibility label. | The crest's name always sits beside it or in its label. |
| System-drawn chrome (tab bar, its badge, system materials and sheets) | No fixed colour exists to assert; measured only by the `.contrast` UI audit. | The audit samples the glass, which is why the audit rows are allow-listed. |

## Repo-rules sweep

- `grep -rnE 'Color\((hex|red):|\.system\(size:|\.gradient|LinearGradient' apps/mobile/Monaco/Features` gives 0 hits.
- Of the `RepoRulesAllowlist.txt` changes since before #6605 (`194ba5494^..origin/staging`), none adds a `raw-color` or `fixed-font` row.
- No gradient.
- No colour role was added to MonacoTheme after #6605. The toast tokens were re-pointed at existing ones (`surface`, `primaryText`, `profit`, `loss`), and those pairs are covered.

## Audit

95 screens, each in Dark and Light. 70 screens report contrast issues, the identical set in both appearances: 430 unique (screen, label) pairs and 969 issue reports (most "Contrast failed", some "not high enough unless font size is larger"). 25 screens are clean.

All 430 pairs are allow-listed by class, each with a reason in `FAppearanceAuditUITests.swift`: `crest` (initials on an identity hue, 1 to 3 capital letters or digits) and `token` (everything else). Every flagged label is token-coloured text whose pair `DesignContrastTests` holds at AA in both schemes. The identical set across appearances shows that the audit samples composited pixels, not the drawn pair. The allow-list makes the audit a regression lock: a new label that fails fails the run.

## Follow-ups

- #7087 (crest, #6461's area): the crest is a bare identity-hue tile with no initials.
- #7091 (glass, #6470's area): toolbar-placed rounds and the system half-sheet body ignore Reduce Transparency.

Parity follow-ups referenced: #7022, #7025, #7033, #7034.
