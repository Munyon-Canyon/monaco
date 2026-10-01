---
name: ios-screen
description: Says what a good Monaco iOS screen looks like, from colour, type, touch targets and accessibility to toasts and product copy. Use when asked to design a screen, build the UI for a feature, make this look right, or change the layout of a SwiftUI view under apps/mobile.
---

# iOS screen

Where code goes and how state flows is `docs/architecture/ios.md`. Read it first; this skill does not restate it. This skill covers how the screen looks and reads.

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

Before the PR, preview the screen at the largest accessibility text size in both light and dark appearance.
