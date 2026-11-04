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
            pairs.append(Pair("white", Color.white, on: [surface]))
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
        ] {
            pairs.append(Pair(label, color, on: [Color.white, MonacoTheme.coverOverlay]))
        }

        pairs.append(Pair("onBrand", MonacoTheme.onBrand, on: [MonacoTheme.brandFill], minimum: 7))
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

    @Test func tokensKeepTheirDarkValues() {
        let expected: [(name: String, color: Color, hex: UInt32, alpha: Double)] = [
            ("background", MonacoTheme.background, 0x000000, 1),
            ("surface", MonacoTheme.surface, 0x1C1C1E, 1),
            ("sheetSurface", MonacoTheme.sheetSurface, 0x1C1C1E, 1),
            ("surfaceSunken", MonacoTheme.surfaceSunken, 0x2C2C2E, 1),
            ("primaryText", MonacoTheme.primaryText, 0xFFFFFF, 1),
            ("secondaryText", MonacoTheme.secondaryText, 0xA1A1A6, 1),
            ("tertiaryText", MonacoTheme.tertiaryText, 0x6E6E73, 1),
            ("disabledLabel", MonacoTheme.disabledLabel, 0x6E6E73, 1),
            ("border", MonacoTheme.border, 0x38383A, 1),
            ("tint", MonacoTheme.tint, 0xFFFFFF, 1),
            ("link", MonacoTheme.link, 0xFFFFFF, 1),
            ("action", MonacoTheme.action, 0x1E5C3A, 1),
            ("onAction", MonacoTheme.onAction, 0xFFFFFF, 1),
            ("gain", MonacoTheme.gain, 0x30F57A, 1),
            ("switchOn", MonacoTheme.switchOn, 0x1E5C3A, 1),
            ("chipOn", MonacoTheme.chipOn, 0xFFFFFF, 1),
            ("onChipOn", MonacoTheme.onChipOn, 0x000000, 1),
            ("brandFill", MonacoTheme.brandFill, 0x1E5C3A, 1),
            ("onBrand", MonacoTheme.onBrand, 0xFFFFFF, 1),
            ("warningWash", MonacoTheme.warningWash, 0x2C2819, 1),
            ("profit", MonacoTheme.profit, 0x30F57A, 1),
            ("loss", MonacoTheme.loss, 0xFF6259, 1),
            ("profitVivid", MonacoTheme.profitVivid, 0x30F57A, 1),
            ("onVivid", MonacoTheme.onVivid, 0x0E1512, 1),
            ("lossVivid", MonacoTheme.lossVivid, 0xFF453A, 1),
            ("onCover", MonacoTheme.onCover, 0xFFFFFF, 1),
            ("onCoverMuted", MonacoTheme.onCoverMuted, 0xC5D1CB, 1),
            ("onCoverGlass", MonacoTheme.onCoverGlass, 0xFFFFFF, 0.09),
            ("onCoverGlassStroke", MonacoTheme.onCoverGlassStroke, 0xFFFFFF, 0.14),
            ("coverStroke", MonacoTheme.coverStroke, 0xFFFFFF, 0.07),
            ("coverOverlay", MonacoTheme.coverOverlay, 0x00130B, 0.8),
            ("coverOverlayReduced", MonacoTheme.coverOverlayReduced, 0x00130B, 0.91),
            ("voteNeededFill", MonacoTheme.voteNeededFill, 0xD70015, 1),
            ("voteNeededLabel", MonacoTheme.voteNeededLabel, 0xFFFFFF, 1),
            ("allocationTints[0]", MonacoTheme.allocationTints[0], 0x30F57A, 1),
            ("allocationTints[1]", MonacoTheme.allocationTints[1], 0x0A84FF, 1),
            ("allocationTints[2]", MonacoTheme.allocationTints[2], 0xBF5AF2, 1),
            ("onDestructive", MonacoTheme.onDestructive, 0x000000, 1),
            ("disabled", MonacoTheme.disabled, 0x37453D, 1),
            ("warning", MonacoTheme.warning, 0xE6C477, 1),
            ("crown", MonacoTheme.crown, 0xFFD60A, 1),
            ("Shadow.floating.color", MonacoTheme.Shadow.floating.color, 0x000000, 0.2),
            ("Shadow.sheet.color", MonacoTheme.Shadow.sheet.color, 0x000000, 0.5),
        ]
        let tolerance = 0.5 / 255
        for token in expected {
            let rgba = WCAGContrast.resolve(token.color, .dark)
            #expect(abs(rgba.red - Double((token.hex >> 16) & 0xFF) / 255) <= tolerance, "\(token.name) red")
            #expect(abs(rgba.green - Double((token.hex >> 8) & 0xFF) / 255) <= tolerance, "\(token.name) green")
            #expect(abs(rgba.blue - Double(token.hex & 0xFF) / 255) <= tolerance, "\(token.name) blue")
            #expect(abs(rgba.alpha - token.alpha) <= tolerance, "\(token.name) alpha")
        }
    }

    @Test func knownRatioMatchesTheWCAGFormula() {
        let ratio = WCAGContrast.ratio(.white, on: [.black], .dark)
        #expect(abs(ratio - 21) < 0.01)
    }

    @Test func textTokensClearAA() {
        for pair in Self.textPairs {
            let ratio = WCAGContrast.ratio(pair.foreground, on: pair.backgrounds, .dark)
            #expect(ratio >= pair.minimum, "\(pair.label) is \(ratio), below \(pair.minimum)")
        }
    }

    @Test func glyphTokensClearTheGraphicalMinimum() {
        for pair in Self.glyphPairs {
            let ratio = WCAGContrast.ratio(pair.foreground, on: pair.backgrounds, .dark)
            #expect(ratio >= pair.minimum, "\(pair.label) is \(ratio), below \(pair.minimum)")
        }
    }

    @Test func profitAndLossCannotBeConfused() {
        let hueGap = abs(
            OKLCh.value(MonacoTheme.profit, .dark).hue - OKLCh.value(MonacoTheme.loss, .dark).hue
        )
        #expect(min(hueGap, 360 - hueGap) >= 90, "profit and loss are \(hueGap)° apart")
        let moneyDelta = OKLCh.distance(MonacoTheme.profit, MonacoTheme.loss, .dark)
        #expect(moneyDelta >= 0.2, "profit vs loss: ΔE OKLab is \(moneyDelta)")
    }

    private static let identityHexes: [UInt32] = [
        0x5E5CE6, 0xFF9F0A, 0x40C8E0, 0xFF375F, 0xBF5AF2, 0xAC8E68, 0x0A84FF, 0x64D2FF,
    ]

    private static func hex(_ color: Color, _ scheme: UIUserInterfaceStyle) -> UInt32 {
        let rgba = WCAGContrast.resolve(color, scheme)
        func byte(_ value: Double) -> UInt32 { UInt32((value * 255).rounded()) }
        return byte(rgba.red) << 16 | byte(rgba.green) << 8 | byte(rgba.blue)
    }

    @Test func roleColoursDoNotAliasTint() {
        let roles: [(name: String, color: Color, expected: Color)] = [
            ("allocationTints[0]", MonacoTheme.allocationTints[0], MonacoTheme.gain),
            ("pollVoteWash", MonacoTheme.pollVoteWash, MonacoTheme.brandFill),
            ("unreadDot", MonacoTheme.unreadDot, MonacoTheme.voteNeededFill),
            ("thesisBar", MonacoTheme.thesisBar, MonacoTheme.gain),
        ]
        for scheme in [UIUserInterfaceStyle.dark, .light] {
            for role in roles {
                #expect(Self.hex(role.color, scheme) != Self.hex(MonacoTheme.tint, scheme), "\(role.name) \(scheme)")
                #expect(Self.hex(role.color, scheme) == Self.hex(role.expected, scheme), "\(role.name) \(scheme)")
            }
        }
    }

    @Test func identityHuesAreTheBoards() {
        #expect(MonacoTheme.CabalTint.allCases.map { Self.hex($0.fill, .dark) } == Self.identityHexes)
    }

    @Test func noIdentityHueIsGreen() {
        for tint in MonacoTheme.CabalTint.allCases {
            var hue = OKLCh.value(tint.fill, .dark).hue
            if hue < 0 { hue += 360 }
            #expect(hue < 120 || hue > 170, "\(tint) hue \(hue)° is in the green band")
        }
    }

    @Test func noCabalTintReadsAsAMoneyColour() {
        for tint in MonacoTheme.CabalTint.allCases {
            let hueGap = abs(OKLCh.value(tint.fill, .dark).hue - OKLCh.value(MonacoTheme.profit, .dark).hue)
            #expect(
                min(hueGap, 360 - hueGap) >= 30,
                "\(tint) vs profit: only \(min(hueGap, 360 - hueGap))° of hue apart"
            )
        }
    }

    @Test func theActionFillAndTheGainGreenDifferInLightness() {
        let fill = OKLCh.value(MonacoTheme.brandFill, .dark).lightness
        let gain = OKLCh.value(MonacoTheme.profit, .dark).lightness
        #expect(gain - fill >= 0.4, "brandFill vs profit: OKLab lightness gap is \(gain - fill)")
    }

    @Test func theChartLineIsTheGainGreen() {
        #expect(Self.hex(MonacoTheme.profitVivid, .dark) == Self.hex(MonacoTheme.profit, .dark))
    }

    @Test func theBadgeIsWhiteOnTheBoardsRed() {
        #expect(Self.hex(MonacoTheme.voteNeededFill, .dark) == 0xD70015)
        #expect(Self.hex(MonacoTheme.voteNeededLabel, .dark) == 0xFFFFFF)
        #expect(WCAGContrast.ratio(MonacoTheme.voteNeededLabel, on: [MonacoTheme.voteNeededFill], .dark) >= 4.5)
    }

    @Test func disabledLabelStaysQuieterThanContent() {
        for surface in Self.surfaces {
            let disabled = WCAGContrast.ratio(MonacoTheme.disabledLabel, on: [surface], .dark)
            let content = WCAGContrast.ratio(MonacoTheme.secondaryText, on: [surface], .dark)
            #expect(disabled < 4.5, "disabledLabel is \(disabled): a disabled control reads as live")
            #expect(disabled >= 2.5, "disabledLabel is \(disabled): unavailable is not the same as invisible")
            #expect(
                content - disabled > 1,
                "secondaryText (\(content)) and disabledLabel (\(disabled)) have converged"
            )
        }
    }

    @Test func theToastIsNotTheBrandFill() {
        // The toast borrowed `primaryButton*`, so an error toast read as a second primary button
        // stacked above the real one. Keep the two apart.
        let toast = WCAGContrast.resolve(MonacoTheme.toastFill, .dark)
        let button = WCAGContrast.resolve(MonacoTheme.primaryButtonFill, .dark)
        let delta = abs(WCAGContrast.luminance(toast) - WCAGContrast.luminance(button))
        #expect(delta > 0.02, "toastFill and primaryButtonFill are nearly the same colour")
    }
}

extension MonacoContrastTests {
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
            let ratio = WCAGContrast.ratio(pair.foreground, on: pair.backgrounds, .dark)
            if ratio < pair.minimum {
                failed.append("\(pair.label) is \(ratio), below \(pair.minimum)")
            }
        }
        return failed
    }

    @Test func v3TextPairsClearAA() {
        #expect(Self.failures(Self.v3TextPairs).isEmpty, "\(Self.failures(Self.v3TextPairs))")
    }

    @Test func v3MarksClearTheGraphicalMinimum() {
        #expect(Self.failures(Self.v3MarkPairs).isEmpty, "\(Self.failures(Self.v3MarkPairs))")
    }

    @Test func hairlineStaysVisibleOnTheCanvas() {
        let ratio = WCAGContrast.ratio(MonacoTheme.hairline, on: [MonacoTheme.canvas], .dark)
        #expect(ratio >= 1.1, "hairline is \(ratio) on canvas")
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

    @Test func fMarksClearTheGraphicalMinimum() {
        #expect(Self.failures(Self.fMarkPairs).isEmpty, "\(Self.failures(Self.fMarkPairs))")
    }
}
