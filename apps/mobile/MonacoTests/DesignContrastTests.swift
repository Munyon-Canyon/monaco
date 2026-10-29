import SwiftUI
import Testing
import UIKit

@testable import Monaco

/// WCAG 2.1 contrast over resolved design tokens.
///
/// Tokens are role-based, so repointing one (for example `primaryButtonFill` to the electric-blue
/// brand) can silently push an unrelated pair below AA. This table is the guard.
///
/// It is not exhaustive, and should not be read as a claim that it is. It covers the text, glyph
/// and mark pairs the design system defines roles for; a screen that invents a new combination out
/// of existing tokens is not caught until its pair is added here.
///
/// Every text pair is measured on all three surfaces (`canvas`, `surface`, `surfaceSunken`), not
/// only on the canvas: a proposal card is sunken, and its caption and figures have to clear AA there.
enum WCAGContrast {
    struct RGBA {
        var red: Double
        var green: Double
        var blue: Double
        var alpha: Double
    }

    static func resolve(_ color: Color, _ scheme: UIUserInterfaceStyle) -> RGBA {
        let resolved = UIColor(color).resolvedColor(with: UITraitCollection(userInterfaceStyle: scheme))
        var red: CGFloat = 0
        var green: CGFloat = 0
        var blue: CGFloat = 0
        var alpha: CGFloat = 0
        resolved.getRed(&red, green: &green, blue: &blue, alpha: &alpha)
        return RGBA(red: Double(red), green: Double(green), blue: Double(blue), alpha: Double(alpha))
    }

    /// Source-over composite, so a wash is measured on the surface it is drawn over.
    static func composite(_ top: RGBA, over bottom: RGBA) -> RGBA {
        RGBA(
            red: top.red * top.alpha + bottom.red * (1 - top.alpha),
            green: top.green * top.alpha + bottom.green * (1 - top.alpha),
            blue: top.blue * top.alpha + bottom.blue * (1 - top.alpha),
            alpha: 1
        )
    }

    static func luminance(_ rgba: RGBA) -> Double {
        func channel(_ value: Double) -> Double {
            value <= 0.03928 ? value / 12.92 : pow((value + 0.055) / 1.055, 2.4)
        }
        return 0.2126 * channel(rgba.red) + 0.7152 * channel(rgba.green) + 0.0722 * channel(rgba.blue)
    }

    /// `backgrounds` are listed bottom-up: `[.canvas, .profitWash]` is the wash drawn over canvas.
    static func ratio(_ foreground: Color, on backgrounds: [Color], _ scheme: UIUserInterfaceStyle) -> Double {
        var background = resolve(backgrounds[0], scheme)
        background.alpha = 1
        for layer in backgrounds.dropFirst() {
            background = composite(resolve(layer, scheme), over: background)
        }
        let front = composite(resolve(foreground, scheme), over: background)
        let a = luminance(front)
        let b = luminance(background)
        return (max(a, b) + 0.05) / (min(a, b) + 0.05)
    }
}

/// OKLCh over resolved tokens, for the questions WCAG contrast cannot answer.
///
/// Contrast is a ratio against a *background*; it says nothing about whether two foregrounds look
/// like each other. `brand` and `profit` share a hue — the brand is a forest and profit is a green
/// — so the only honest way to assert they cannot be confused is perceptual lightness and chroma,
/// which is what OKLCh gives and HSB does not.
enum OKLCh {
    struct Value {
        /// Perceptual lightness, 0…1.
        var lightness: Double
        /// Chroma — how far from grey. ~0.04 is a tint, ~0.15 is a saturated colour.
        var chroma: Double
        /// Hue angle in degrees.
        var hue: Double
    }

    static func value(_ color: Color, _ scheme: UIUserInterfaceStyle) -> Value {
        let rgba = WCAGContrast.resolve(color, scheme)
        func linear(_ v: Double) -> Double {
            v <= 0.04045 ? v / 12.92 : pow((v + 0.055) / 1.055, 2.4)
        }
        let r = linear(rgba.red)
        let g = linear(rgba.green)
        let b = linear(rgba.blue)
        let l = 0.4122214708 * r + 0.5363325363 * g + 0.0514459929 * b
        let m = 0.2119034982 * r + 0.6806995451 * g + 0.1073969566 * b
        let s = 0.0883024619 * r + 0.2817188376 * g + 0.6299787005 * b
        let lRoot = cbrt(l)
        let mRoot = cbrt(m)
        let sRoot = cbrt(s)
        let lightness = 0.2104542553 * lRoot + 0.7936177850 * mRoot - 0.0040720468 * sRoot
        let a = 1.9779984951 * lRoot - 2.4285922050 * mRoot + 0.4505937099 * sRoot
        let bb = 0.0259040371 * lRoot + 0.7827717662 * mRoot - 0.8086757660 * sRoot
        return Value(
            lightness: lightness,
            chroma: (a * a + bb * bb).squareRoot(),
            hue: (atan2(bb, a) * 180 / .pi).truncatingRemainder(dividingBy: 360)
        )
    }

    /// Euclidean distance in OKLab. Roughly 0.02 is a just-noticeable difference.
    static func distance(_ first: Color, _ second: Color, _ scheme: UIUserInterfaceStyle) -> Double {
        let a = value(first, scheme)
        let b = value(second, scheme)
        let aa = a.chroma * cos(a.hue * .pi / 180)
        let ab = a.chroma * sin(a.hue * .pi / 180)
        let ba = b.chroma * cos(b.hue * .pi / 180)
        let bb = b.chroma * sin(b.hue * .pi / 180)
        return
            ((a.lightness - b.lightness) * (a.lightness - b.lightness)
            + (aa - ba) * (aa - ba) + (ab - bb) * (ab - bb)).squareRoot()
    }
}

private struct Pair {
    let label: String
    let foreground: Color
    let backgrounds: [Color]
    /// 4.5 for body text, 3 for large text and graphical objects.
    let minimum: Double

    init(_ label: String, _ foreground: Color, on backgrounds: [Color], minimum: Double = 4.5) {
        self.label = label
        self.foreground = foreground
        self.backgrounds = backgrounds
        self.minimum = minimum
    }
}

struct MonacoContrastTests {
    private static let surfaces: [Color] = [MonacoTheme.canvas, MonacoTheme.surface, MonacoTheme.surfaceSunken]

    @Test func adaptiveColorsResolveOnTheSwiftUIRendererQueue() async {
        let canvas = MonacoTheme.canvas
        let alpha = await Task.detached {
            UIColor(canvas)
                .resolvedColor(with: UITraitCollection(userInterfaceStyle: .dark))
                .cgColor.alpha
        }.value
        #expect(alpha == 1)
    }

    private static var textPairs: [Pair] {
        var pairs: [Pair] = []
        for surface in surfaces {
            pairs.append(Pair("primaryText", MonacoTheme.primaryText, on: [surface]))
            pairs.append(Pair("secondaryText", MonacoTheme.secondaryText, on: [surface]))
            pairs.append(Pair("tint", MonacoTheme.tint, on: [surface]))
            pairs.append(Pair("flat", PnLTone.flat.color, on: [surface]))
            pairs.append(Pair("warning", MonacoTheme.warning, on: [surface]))
            pairs.append(Pair("destructive", MonacoTheme.destructive, on: [surface]))
            pairs.append(Pair("profit", MonacoTheme.profit, on: [surface]))
            pairs.append(Pair("loss", MonacoTheme.loss, on: [surface]))
        }

        pairs.append(
            Pair(
                "secondaryText on warningWash", MonacoTheme.secondaryText,
                on: [MonacoTheme.canvas, MonacoTheme.warningWash]))
        pairs.append(
            Pair("warning on warningWash", MonacoTheme.warning, on: [MonacoTheme.canvas, MonacoTheme.warningWash]))
        pairs.append(Pair("voteNeededLabel", MonacoTheme.voteNeededLabel, on: [MonacoTheme.voteNeededFill]))
        pairs.append(Pair("onDestructive", MonacoTheme.onDestructive, on: [MonacoTheme.destructive]))
        for tint in MonacoTheme.CabalTint.allCases {
            pairs.append(Pair("\(tint) onFill", tint.onFill, on: [tint.fill]))
        }
        for (label, color) in [
            ("onCover", MonacoTheme.onCover), ("onCoverMuted", MonacoTheme.onCoverMuted),
            ("profitOnCover", MonacoTheme.profitOnCover), ("lossOnCover", MonacoTheme.lossOnCover),
        ] {
            pairs.append(Pair(label, color, on: [Color.white, MonacoTheme.coverOverlay]))
        }

        pairs.append(Pair("onBrand", MonacoTheme.onBrand, on: [MonacoTheme.brandFill]))
        pairs.append(
            Pair(
                "primaryButtonLabel",
                MonacoTheme.primaryButtonLabel,
                on: [MonacoTheme.primaryButtonFill]
            ))
        pairs.append(Pair("toastLabel", MonacoTheme.toastLabel, on: [MonacoTheme.toastFill]))
        return pairs
    }

    private static var glyphPairs: [Pair] {
        [
            Pair("toastSuccessGlyph", MonacoTheme.toastSuccessGlyph, on: [MonacoTheme.toastFill], minimum: 4.5),
            Pair("toastErrorGlyph", MonacoTheme.toastErrorGlyph, on: [MonacoTheme.toastFill], minimum: 4.5),
        ]
    }

    @Test func adaptiveTokensActuallyResolvePerScheme() {
        // Guards the harness itself: if resolution stopped following the scheme every ratio below
        // would be measured twice in light mode and the table would prove nothing.
        let light = WCAGContrast.resolve(MonacoTheme.canvas, .light)
        let dark = WCAGContrast.resolve(MonacoTheme.canvas, .dark)
        #expect(WCAGContrast.luminance(light) > WCAGContrast.luminance(dark))
    }

    @Test func knownRatioMatchesTheWCAGFormula() {
        let ratio = WCAGContrast.ratio(.white, on: [.black], .light)
        #expect(abs(ratio - 21) < 0.01)
    }

    @Test func textTokensClearAA() {
        for pair in Self.textPairs {
            for scheme in [UIUserInterfaceStyle.light, .dark] {
                let ratio = WCAGContrast.ratio(pair.foreground, on: pair.backgrounds, scheme)
                #expect(
                    ratio >= pair.minimum,
                    "\(pair.label) in \(scheme == .light ? "light" : "dark") is \(ratio), below \(pair.minimum)"
                )
            }
        }
    }

    @Test func glyphTokensClearTheGraphicalMinimum() {
        for pair in Self.glyphPairs {
            for scheme in [UIUserInterfaceStyle.light, .dark] {
                let ratio = WCAGContrast.ratio(pair.foreground, on: pair.backgrounds, scheme)
                #expect(
                    ratio >= pair.minimum,
                    "\(pair.label) in \(scheme == .light ? "light" : "dark") is \(ratio), below \(pair.minimum)"
                )
            }
        }
    }

    @Test func profitAndLossCannotBeConfused() {
        for scheme in [UIUserInterfaceStyle.light, .dark] {
            let name = scheme == .light ? "light" : "dark"
            let hueGap = abs(
                OKLCh.value(MonacoTheme.profit, scheme).hue - OKLCh.value(MonacoTheme.loss, scheme).hue
            )
            #expect(min(hueGap, 360 - hueGap) >= 90, "profit and loss in \(name) are \(hueGap)° apart")
            let moneyDelta = OKLCh.distance(MonacoTheme.profit, MonacoTheme.loss, scheme)
            #expect(moneyDelta >= 0.2, "profit vs loss in \(name): ΔE OKLab is \(moneyDelta)")
        }
    }

    private static let identityHexes: [UInt32] = [
        0x5E5CE6, 0xFF9F0A, 0x40C8E0, 0xFF375F, 0xBF5AF2, 0xAC8E68, 0x0A84FF, 0x64D2FF,
    ]

    private static func hex(_ color: Color, _ scheme: UIUserInterfaceStyle) -> UInt32 {
        let rgba = WCAGContrast.resolve(color, scheme)
        func byte(_ value: Double) -> UInt32 { UInt32((value * 255).rounded()) }
        return byte(rgba.red) << 16 | byte(rgba.green) << 8 | byte(rgba.blue)
    }

    @Test func identityHuesAreTheBoards() {
        for scheme in [UIUserInterfaceStyle.light, .dark] {
            #expect(MonacoTheme.CabalTint.allCases.map { Self.hex($0.fill, scheme) } == Self.identityHexes)
        }
    }

    @Test func noIdentityHueIsGreen() {
        for scheme in [UIUserInterfaceStyle.light, .dark] {
            for tint in MonacoTheme.CabalTint.allCases {
                var hue = OKLCh.value(tint.fill, scheme).hue
                if hue < 0 { hue += 360 }
                #expect(hue < 120 || hue > 170, "\(tint) hue \(hue)° is in the green band")
            }
        }
    }

    @Test func noCabalTintReadsAsAMoneyColour() {
        for scheme in [UIUserInterfaceStyle.light, .dark] {
            let name = scheme == .light ? "light" : "dark"
            for tint in MonacoTheme.CabalTint.allCases {
                let hueGap = abs(OKLCh.value(tint.fill, scheme).hue - OKLCh.value(MonacoTheme.profit, scheme).hue)
                #expect(
                    min(hueGap, 360 - hueGap) >= 30,
                    "\(tint) vs profit in \(name): only \(min(hueGap, 360 - hueGap))° of hue apart"
                )
            }
        }
    }

    @Test func tintAndProfitStayApart() {
        for scheme in [UIUserInterfaceStyle.light, .dark] {
            let delta = OKLCh.distance(MonacoTheme.tint, MonacoTheme.profit, scheme)
            #expect(delta >= 0.08, "tint vs profit in \(scheme == .light ? "light" : "dark"): ΔE OKLab is \(delta)")
        }
    }

    @Test func theBadgeIsWhiteOnTheBoardsRed() {
        for scheme in [UIUserInterfaceStyle.light, .dark] {
            #expect(Self.hex(MonacoTheme.voteNeededFill, scheme) == 0xD70015)
            #expect(Self.hex(MonacoTheme.voteNeededLabel, scheme) == 0xFFFFFF)
            #expect(WCAGContrast.ratio(MonacoTheme.voteNeededLabel, on: [MonacoTheme.voteNeededFill], scheme) >= 4.5)
        }
    }

    @Test func disabledLabelStaysQuieterThanContent() {
        for surface in Self.surfaces {
            for scheme in [UIUserInterfaceStyle.light, .dark] {
                let disabled = WCAGContrast.ratio(MonacoTheme.disabledLabel, on: [surface], scheme)
                let content = WCAGContrast.ratio(MonacoTheme.secondaryText, on: [surface], scheme)
                #expect(disabled < 4.5, "disabledLabel is \(disabled): a disabled control reads as live")
                #expect(disabled >= 2.5, "disabledLabel is \(disabled): unavailable is not the same as invisible")
                #expect(
                    content - disabled > 1,
                    "secondaryText (\(content)) and disabledLabel (\(disabled)) have converged"
                )
            }
        }
    }

    /// The toast panel inverts the scheme (dark in light, light in dark), so its glyphs have to
    /// invert with it: a glyph tuned for one panel and left on the other drops to 1.6:1. Aliasing
    /// them to a paper token is the `primaryButtonFill = brandFill` pattern from #309, so the
    /// glyphs are their own tokens and this pins that they flip exactly when the panel does.
    /// Caught structurally, because a ratio check passes right up until the day it does not.
    @Test func toastGlyphsFlipWithTheToastPanel() {
        func flips(_ color: Color) -> Bool {
            let light = WCAGContrast.resolve(color, .light)
            let dark = WCAGContrast.resolve(color, .dark)
            return abs(light.red - dark.red) > 0.001
                || abs(light.green - dark.green) > 0.001
                || abs(light.blue - dark.blue) > 0.001
        }
        for (name, glyph) in [
            ("toastSuccessGlyph", MonacoTheme.toastSuccessGlyph),
            ("toastErrorGlyph", MonacoTheme.toastErrorGlyph),
        ] {
            #expect(
                flips(glyph) == flips(MonacoTheme.toastFill),
                "\(name) and toastFill disagree on whether the toast changes with the scheme"
            )
        }
    }

    @Test func theToastIsNotTheBrandFill() {
        // The toast borrowed `primaryButton*`, so an error toast read as a second primary button
        // stacked above the real one. Keep the two apart.
        for scheme in [UIUserInterfaceStyle.light, .dark] {
            let toast = WCAGContrast.resolve(MonacoTheme.toastFill, scheme)
            let button = WCAGContrast.resolve(MonacoTheme.primaryButtonFill, scheme)
            let delta = abs(WCAGContrast.luminance(toast) - WCAGContrast.luminance(button))
            #expect(delta > 0.02, "toastFill and primaryButtonFill are nearly the same colour")
        }
    }
}

extension MonacoContrastTests {
    private static let bothSchemes: [UIUserInterfaceStyle] = [.light, .dark]

    private static func schemeName(_ scheme: UIUserInterfaceStyle) -> String {
        scheme == .light ? "light" : "dark"
    }

    private static let paper: [Color] = [MonacoTheme.canvas, MonacoTheme.surface]

    private static var coverBases: [(label: String, layers: [Color])] {
        [("white", [Color.white])]
            + MonacoTheme.CabalTint.allCases.map { ("\($0) fill", [$0.fill]) }
    }

    private static var coverPairs: [Pair] {
        var pairs: [Pair] = []
        for (overlayLabel, overlay) in [
            ("coverOverlay", MonacoTheme.coverOverlay), ("coverOverlayReduced", MonacoTheme.coverOverlayReduced),
        ] {
            for base in coverBases {
                for (label, color) in [
                    ("onCover", MonacoTheme.onCover), ("onCoverMuted", MonacoTheme.onCoverMuted),
                    ("profitOnCover", MonacoTheme.profitOnCover), ("lossOnCover", MonacoTheme.lossOnCover),
                ] {
                    pairs.append(
                        Pair("\(label) on \(overlayLabel) over \(base.label)", color, on: base.layers + [overlay]))
                }
            }
        }
        return pairs
    }

    private static var v3TextPairs: [Pair] {
        var pairs = coverPairs
        for surface in paper {
            for tone in StatusBadge.Tone.allCases {
                pairs.append(Pair("StatusBadge \(tone)", tone.label, on: [surface, tone.fill]))
            }
            for kind in EventPill.Kind.allCases {
                pairs.append(Pair("EventPill \(kind)", kind.label, on: [surface, kind.fill]))
            }
        }
        return pairs
    }

    private static var v3MarkPairs: [Pair] {
        var pairs: [Pair] = []
        for surface in paper {
            for (label, color) in [
                ("profitVivid", MonacoTheme.profitVivid), ("lossVivid", MonacoTheme.lossVivid),
                ("flat chart line", MonacoTheme.secondaryText),
            ] {
                pairs.append(Pair("chart \(label)", color, on: [surface], minimum: 3))
            }
        }
        for (label, color) in [("yes", MonacoTheme.profitVivid), ("no", MonacoTheme.lossVivid)] {
            pairs.append(Pair("VoteBar \(label) against canvas", color, on: [MonacoTheme.canvas], minimum: 3))
        }
        for (index, color) in MonacoTheme.allocationTints.dropLast().enumerated() {
            pairs.append(
                Pair("AllocationBar segment \(index) against canvas", color, on: [MonacoTheme.canvas], minimum: 3))
            pairs.append(
                Pair(
                    "AllocationBar segment \(index) against its cash track", color, on: [MonacoTheme.border], minimum: 3
                ))
        }
        return pairs
    }

    private static func failures(_ pairs: [Pair]) -> [String] {
        var failed: [String] = []
        for pair in pairs {
            for scheme in bothSchemes {
                let ratio = WCAGContrast.ratio(pair.foreground, on: pair.backgrounds, scheme)
                if ratio < pair.minimum {
                    failed.append("\(pair.label) in \(schemeName(scheme)) is \(ratio), below \(pair.minimum)")
                }
            }
        }
        return failed
    }

    @Test func v3TextPairsClearAAInBothSchemes() {
        #expect(Self.failures(Self.v3TextPairs).isEmpty, "\(Self.failures(Self.v3TextPairs))")
    }

    @Test func v3MarksClearTheGraphicalMinimumInBothSchemes() {
        #expect(Self.failures(Self.v3MarkPairs).isEmpty, "\(Self.failures(Self.v3MarkPairs))")
    }

    @Test func hairlineStaysVisibleOnTheCanvas() {
        for scheme in Self.bothSchemes {
            let ratio = WCAGContrast.ratio(MonacoTheme.hairline, on: [MonacoTheme.canvas], scheme)
            #expect(ratio >= 1.1, "hairline in \(Self.schemeName(scheme)) is \(ratio) on canvas")
        }
    }

    @Test func reduceTransparencyDrawsTheMoreOpaqueOverlayOnEveryCover() {
        let styles: [CabalCover<EmptyView>.Style] = [.hero, .heroTall, .card, .memberCard]
        for style in styles {
            #expect(
                CabalCover<EmptyView>.overlay(for: style, reduceTransparency: true) == MonacoTheme.coverOverlayReduced)
        }
        #expect(CabalCover<EmptyView>.overlay(for: .thumb(40), reduceTransparency: true) == nil)
    }
}

extension MonacoContrastTests {
    private static var fMarkPairs: [Pair] {
        var pairs: [Pair] = []
        for (label, color) in [
            ("VoteTallyBar winner fill", VoteTallyBar.fill(.yes, winner: .yes)),
            ("VoteTallyBar idle fill", VoteTallyBar.fill(.yes, winner: nil)),
        ] {
            pairs.append(Pair("\(label) against its track", color, on: [MonacoTheme.surfaceSunken], minimum: 3))
        }
        pairs.append(
            Pair(
                "MonacoUnderlineTabs selected underline against canvas", MonacoTheme.primaryText,
                on: [MonacoTheme.canvas], minimum: 3))
        return pairs
    }

    @Test func fMarksClearTheGraphicalMinimumInBothSchemes() {
        #expect(Self.failures(Self.fMarkPairs).isEmpty, "\(Self.failures(Self.fMarkPairs))")
    }
}
