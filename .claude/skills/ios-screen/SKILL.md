---
name: ios-screen
description: Says what a good Monaco iOS screen looks like, from colour, type, touch targets and accessibility to toasts and product copy. Use when asked to design a screen, build the UI for a feature, make this look right, change the layout of a SwiftUI view under apps/mobile, match the v3 design, or restyle to v3.
---

# iOS screen

Where code goes and how state flows is `docs/architecture/ios.md`. Read it first; this skill does not restate it. This skill covers how the screen looks and reads.

## Look (v3)

- Follow `docs/design.md`. References are in `docs/design/v3/screens/`; open a state with `docs/design/v3/monaco-ui-v3.html#screen=<template>&state=<state>`.
- Build with the shared components, once the F tickets land: `MonacoCard`, `MonacoNotice`, `StatusBadge` / `PnLPill`, `OutcomeView`, `MonacoGlassButton`, `MonacoSheetHeading`, `CabalCover` / `CabalCoverCard`, `VoteBar`, `AllocationBar`, `StatGrid`, `QuietChart`, `MonacoMenuSelect`, `.monacoOutline`. Don't re-create one inside a feature folder.
- 15pt floor: never shrink text to match a reference. A 10–13px caption in the design is `Typo.caption`.
- Glass: only `MonacoGlassButton`, the system tab bar, sheets and `BottomCTA` are glass. Every glass surface must honour Reduce Transparency.
- Backend gaps: build the UI against a fixture, show "Opens soon" where data is missing, and name the missing API in the PR.
- Covers: use `CabalCover` (tint gradient + uploaded picture + scrim). Never bundle photos.

## HIG

- Every colour is a `MonacoTheme` token (`apps/mobile/Monaco/Design/MonacoTheme.swift`). No `Color(hex:`, `Color(red:` or `UIColor(red:` outside `Design/`; the `raw-color` rule fails one.
- Text uses a text style (`.body`, `.headline`, …) or a `MonacoTheme.Typo` font, never a fixed point size. No `.system(size:` outside `Design/`; the `fixed-font` rule fails one.
- Every tappable target is at least 44 × 44 points.
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

- Compare your screenshot with the reference for every state the ticket lists, in Light and Dark.
- Preview the screen at the largest accessibility text size, and with Reduce Transparency on.
- Keep `docs/screens.md` in step.
