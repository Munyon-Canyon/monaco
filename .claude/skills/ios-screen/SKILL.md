---
name: ios-screen
description: Says what a good Monaco iOS screen looks like, from colour, type, touch targets and accessibility to toasts and product copy. Use when asked to design a screen, build the UI for a feature, make this look right, change the layout of a SwiftUI view under apps/mobile, match the F design, restyle to F, or build this F frame.
---

# iOS screen

Where code goes and how state flows is `docs/architecture/ios.md`. Read it first; this skill does not restate it. This skill covers how the screen looks and reads.

## Look (F)

- Follow `docs/design.md`. References are `docs/design/f/screens/<frame>.png`; the build map in `docs/design/f/notes.md` names the frame for each screen. Build Launch frames only; never build a Later or Alternatives frame.
- Dark first: check Dark before Light. Light is pure white.
- Build with the shared parts, never a copy inside a feature folder:
  - `MonacoUnderlineTabs`, `MonacoRangeChips` and `MonacoPeriodLabels` (plain period labels), chips and buttons (`.monacoPrimary` is the 50pt prominent button): #6449.
  - The toast with an Undo action (`MonacoToast`, `MonacoToastAction`): #6458.
  - `AmountKeypadScreen` on `AmountKeypad`, the Phantom keypad: #6459.
  - `VoteTally`, `VotePair` and `VoteHoldButton` (hold to vote), `VoteSidesRow`: #6460.
  - `RankedRow`, `CabalMark` (the crest), `StatGrid` (the stat row) and `RemindControl`: #6461.
  - `QuietChart` and `Sparkline` with the flat fill: #6450.
  - `MonacoGlassButton`, `BottomCTA`'s dock and the sheet helper (`monacoSheet`, `MonacoSheetDetents`): #6470.
  - The post reaction chip (`MonacoReactionChip`, on `ReactionChipMetrics`: 24 visible, 44 × 44 tap), the reaction row and the reply control: #6469. The composer: #6548; its ⊕ menu (Poll, Quote the thesis, Share a stock): #6563; the GIF sheet on the GIPHY iOS SDK: #6556.
  - If a final type name differs once those land, the ticket that renames it updates this list.
- 15pt floor: never shrink text to match a frame. A 13pt row sub-line or caption is `Typo.subhead` or `Typo.caption`.
- Lists, not boxes: flat rows on the canvas with an inset hairline. A card holds one object.
- Sheets open at the half detent through #6470's sheet helper, which owns the detents, including the accessibility-size exception. Never set detents by hand.
- Chats take no image uploads: never add a photo or camera attach to the composer, a reply or a discussion. GIFs come from the GIPHY SDK.
- Materials: system materials on chrome only, each with a Reduce Transparency fallback.
- Sample data: frames use the fixture table in `docs/design/f/notes.md`. Never ship a fixture value; build against a fixture and show real data.

## HIG

- Every colour is a `MonacoTheme` token (`apps/mobile/Monaco/Design/MonacoTheme.swift`). No `Color(hex:`, `Color(red:` or `UIColor(red:` outside `Design/`; the `raw-color` rule fails one.
- Text uses a text style (`.body`, `.headline`, …) or a `MonacoTheme.Typo` font, never a fixed point size. No `.system(size:` outside `Design/`; the `fixed-font` rule fails one.
- Every tappable target is at least 44 × 44 points, including each reaction chip, quick emoji and text Reply.
- Support light and dark mode. The tokens adapt; a hard-coded colour does not.
- Support Dynamic Type up to the accessibility sizes. Lay out rows with `ViewThatFits` or a vertical fallback so nothing clips or truncates.
- Give every icon-only button a VoiceOver label with `.accessibilityLabel(_:)`.

## Monaco copy

- Confirm an action with a toast through the shared toast presenter: `MonacoToast` with `.monacoToast(_:)` today, `ToastCenter` once #945 lands. Do not add banners or alerts to the main UI.
- Say "cabal", never "club" or "group".
- Say "deposit" for inbound USDC, and "fund this cabal" for moving USDC into a treasury.
- Show a stock by name or symbol through `AssetDisplayName`, never a mint address or "xStock".
- Show a wallet address with `MonacoWalletAddressText`. Never hyphenate it or edit it by truncating.

## Check

Before the PR:

- Compare your screenshot with every frame the ticket lists, in Dark and Light.
- Preview the screen at the largest accessibility text size, and with Reduce Transparency on.
- Keep `docs/screens.md` in step.
