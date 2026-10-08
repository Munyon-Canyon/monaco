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
            // Timestamps and other quiet real content. Disabled labels and placeholders are
            // deliberately *not* this token any more — see `disabledLabelStaysQuieterThanContent`.
            pairs.append(Pair("tertiaryText", MonacoTheme.tertiaryText, on: [surface]))
            // Amounts that neither gained nor lost, and their badge.
            pairs.append(Pair("flat", PnLTone.flat.color, on: [surface]))
            pairs.append(Pair("flat on wash", PnLTone.flat.washColor, on: [surface, PnLTone.flat.wash]))
            // Warnings in banners and rows.
            pairs.append(Pair("warning", MonacoTheme.warning, on: [surface]))
            // `destructive` aliases `loss`; listed under its own role so repointing it is caught.
            pairs.append(Pair("destructive", MonacoTheme.destructive, on: [surface]))
            // PnLBadge: signed text on its own wash.
            pairs.append(Pair("profitOnWash", MonacoTheme.profitOnWash, on: [surface, MonacoTheme.profitWash]))
            pairs.append(Pair("lossOnWash", MonacoTheme.lossOnWash, on: [surface, MonacoTheme.lossWash]))
            // Bare signed text, no wash.
            pairs.append(Pair("profit", MonacoTheme.profit, on: [surface]))
            pairs.append(Pair("loss", MonacoTheme.loss, on: [surface]))
            // CircleAction glyph and selected chips.
            pairs.append(Pair("brandOnWash", MonacoTheme.brandOnWash, on: [surface, MonacoTheme.brandWash]))
            // `brand` as a label: "See all", "Show more", "Try again", the selected tab item.
            pairs.append(Pair("brand", MonacoTheme.brand, on: [surface]))
            // `gold` as text: the rank-1 caption on a board. `goldGlyph` is not here on purpose —
            // the crown is decoration beside a rank the row also prints, so it is exempt.
            pairs.append(Pair("gold", MonacoTheme.gold, on: [surface]))
            pairs.append(Pair("gold on wash", MonacoTheme.gold, on: [surface, MonacoTheme.goldWash]))
        }

        pairs.append(Pair("onBrand", MonacoTheme.onBrand, on: [MonacoTheme.brandFill]))
        // The same pair through the role aliases, so repointing a role is caught even if the
        // token it aliased stayed put.
        pairs.append(
            Pair(
                "primaryButtonLabel",
                MonacoTheme.primaryButtonLabel,
                on: [MonacoTheme.primaryButtonFill]
            ))
        pairs.append(
            Pair(
                "secondaryButtonLabel",
                MonacoTheme.secondaryButtonLabel,
                on: [MonacoTheme.secondaryButtonFill]
            ))
        pairs.append(Pair("onHero", MonacoTheme.onHero, on: [MonacoTheme.heroInk]))
        pairs.append(Pair("onHeroMuted", MonacoTheme.onHeroMuted, on: [MonacoTheme.heroInk]))
        pairs.append(Pair("profitOnHero", MonacoTheme.profitOnHero, on: [MonacoTheme.heroInk]))
        pairs.append(Pair("lossOnHero", MonacoTheme.lossOnHero, on: [MonacoTheme.heroInk]))
        pairs.append(
            Pair(
                "profitOnHero on wash",
                MonacoTheme.profitOnHero,
                on: [MonacoTheme.heroInk, MonacoTheme.profitWashOnHero]
            ))
        pairs.append(
            Pair(
                "lossOnHero on wash",
                MonacoTheme.lossOnHero,
                on: [MonacoTheme.heroInk, MonacoTheme.lossWashOnHero]
            ))
        // A flat figure on the hero card, over its own wash.
        pairs.append(Pair("flat on hero", PnLTone.flat.inkCardColor, on: [MonacoTheme.heroInk]))
        pairs.append(
            Pair(
                "flat on hero wash",
                PnLTone.flat.inkCardColor,
                on: [MonacoTheme.heroInk, PnLTone.flat.inkCardWash]
            ))
        // The toast.
        pairs.append(Pair("toastLabel", MonacoTheme.toastLabel, on: [MonacoTheme.toastFill]))
        return pairs
    }

    /// Graphical objects and large bold text: WCAG 1.4.11 / 1.4.3 ask for 3:1.
    ///
    /// The toast glyphs are held to 4.5 rather than 3 even though they are graphical. They are the
    /// only thing distinguishing a failure toast from a success one for a member who does not read
    /// the copy, and they sit over money screens. The nearest is 4.5:1, the error glyph on the
    /// light panel in dark mode. At a 3:1 bar a retune could take a third off that and still pass.
    private static var glyphPairs: [Pair] {
        var pairs: [Pair] = [
            Pair("toastSuccessGlyph", MonacoTheme.toastSuccessGlyph, on: [MonacoTheme.toastFill], minimum: 4.5),
            Pair("toastErrorGlyph", MonacoTheme.toastErrorGlyph, on: [MonacoTheme.toastFill], minimum: 4.5),
        ]
        for tint in MonacoTheme.CabalTint.allCases {
            // Bold tile initials, 15pt and up: the large-text bar, not the body one.
            pairs.append(Pair("\(tint) onFill", tint.onFill, on: [tint.fill], minimum: 3))
            // The retired on-ink tint, which is now the stroke on the canvas the hero aliases.
            pairs.append(Pair("\(tint) onInk", tint.onInk, on: [MonacoTheme.heroInk], minimum: 3))
        }
        return pairs
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

    /// The rule the whole palette is built around: **green means "this went up"**.
    ///
    /// The brand is a forest and `profit` is a green, so they share a hue. Hue cannot separate
    /// them and this test does not pretend it can. What separates them is that the brand reads as
    /// *ink* and profit reads as *colour*: profit sits further from the text's lightness and
    /// carries more chroma, in both schemes.
    ///
    /// Measured today, with the floors this asserts in brackets:
    ///
    /// | scheme | pair             | ΔL*           | chroma ratio  | ΔE OKLab      |
    /// |--------|------------------|---------------|---------------|---------------|
    /// | light  | brand vs profit  | 0.104 (≥0.03) | 1.65 (≥1.6)   | 0.116 (≥0.10) |
    /// | light  | fill  vs profit  | 0.269 (≥0.09) | 3.16 (≥1.9)   | 0.283 (≥0.13) |
    /// | dark   | brand vs profit  | 0.037 (≥0.03) | 2.39 (≥1.6)   | 0.110 (≥0.10) |
    /// | dark   | fill  vs profit  | 0.168 (≥0.09) | 26.8 (≥1.9)   | 0.240 (≥0.13) |
    ///
    /// For scale: a just-noticeable difference in OKLab is about 0.02, so the closest pair is five
    /// JNDs apart. The fill keeps the floors it always had. The text `brand` gets lower ones: the
    /// redesign lifts it to a mint in dark (11:1 on the canvas) and only ever sets it as a text
    /// link, never as a figure, so a link sits nearer profit than the fill does. These floors are
    /// where a retune would start making a link look like a gain.
    @Test func brandAndProfitCannotBeConfused() {
        for scheme in [UIUserInterfaceStyle.light, .dark] {
            let name = scheme == .light ? "light" : "dark"
            let profit = OKLCh.value(MonacoTheme.profit, scheme)
            let text = OKLCh.value(MonacoTheme.primaryText, scheme)
            let floors = [
                ("brand", MonacoTheme.brand, (lightness: 0.03, chroma: 1.6, delta: 0.10)),
                ("brandFill", MonacoTheme.brandFill, (lightness: 0.09, chroma: 1.9, delta: 0.13)),
            ]
            for (label, token, floor) in floors {
                let brand = OKLCh.value(token, scheme)
                let deltaLightness = abs(profit.lightness - brand.lightness)
                // Which side the brand sits on flips with the scheme — it is ink on cream in light
                // and cream on ink in dark, because it follows the surfaces. What does not flip is
                // that it stays on the *text's* side of profit. Asserting "brand is darker than
                // profit" would only have been true in light mode.
                #expect(
                    abs(brand.lightness - text.lightness) < abs(profit.lightness - text.lightness),
                    "\(label) in \(name) is further from primaryText than profit is: it reads as money"
                )
                #expect(
                    deltaLightness >= floor.lightness,
                    "\(label) vs profit in \(name): ΔL* is \(deltaLightness), under \(floor.lightness)"
                )
                #expect(
                    profit.chroma / brand.chroma >= floor.chroma,
                    "\(label) vs profit in \(name): profit is only \(profit.chroma / brand.chroma)× its chroma"
                )
                let delta = OKLCh.distance(token, MonacoTheme.profit, scheme)
                #expect(
                    delta >= floor.delta, "\(label) vs profit in \(name): ΔE OKLab is \(delta), under \(floor.delta)")
            }
            // And the two money colours from each other. Here hue *is* the separator — and it has
            // to stay one, because red/green is the pair a member reads fastest and the pair a
            // deuteranopic member reads slowest.
            let hueGap = abs(
                OKLCh.value(MonacoTheme.profit, scheme).hue - OKLCh.value(MonacoTheme.loss, scheme).hue
            )
            #expect(min(hueGap, 360 - hueGap) >= 90, "profit and loss in \(name) are \(hueGap)° apart")
            let moneyDelta = OKLCh.distance(MonacoTheme.profit, MonacoTheme.loss, scheme)
            #expect(moneyDelta >= 0.2, "profit vs loss in \(name): ΔE OKLab is \(moneyDelta)")
            // The "You" tag must not look like a gain either. The P&L washes are clear now, so the
            // pair is `brand` against bare `profit`, held to the text brand's floor.
            let onWash = OKLCh.distance(MonacoTheme.brandOnWash, MonacoTheme.profitOnWash, scheme)
            #expect(onWash >= 0.10, "brandOnWash vs profitOnWash in \(name): ΔE OKLab is \(onWash)")
        }
    }

    /// Five cabals have to be five colours at a glance, and hue alone does not deliver that.
    ///
    /// A first pass at the forest tints separated them by hue only, with the two earthiest sitting
    /// at nearly the same lightness. On the Home list, two cabals in adjacent rows came out as one
    /// brown. Hue separation is what keeps a tint off the money colours; *lightness* separation is
    /// what keeps two tints off each other when they happen to land next to one another, and which
    /// pair lands next to which is a hash of the group id, so every pair has to survive it.
    ///
    /// Either route is enough on its own: 60° of hue, or 0.08 of L*.
    @Test func everyPairOfCabalTintsIsTellableApart() {
        let tints = MonacoTheme.CabalTint.allCases
        for scheme in [UIUserInterfaceStyle.light, .dark] {
            let name = scheme == .light ? "light" : "dark"
            for (index, first) in tints.enumerated() {
                for second in tints[(index + 1)...] {
                    let a = OKLCh.value(first.fill, scheme)
                    let b = OKLCh.value(second.fill, scheme)
                    let hueGap = abs(a.hue - b.hue)
                    let deltaHue = min(hueGap, 360 - hueGap)
                    let deltaLightness = abs(a.lightness - b.lightness)
                    #expect(
                        deltaHue >= 60 || deltaLightness >= 0.08,
                        "\(first) and \(second) in \(name) are only \(deltaHue)° apart at ΔL* \(deltaLightness): two cabals would read as one"
                    )
                }
            }
        }
    }

    /// A cabal tile sits in a row that also carries a gain or a loss, so no tint may be mistaken for
    /// either.
    ///
    /// Hue is the whole guard here, and deliberately so. The tints are *not* quieter than the money
    /// colours — measured, they run 0.9× to 1.2× profit's chroma — so a "the tint is more muted"
    /// assertion would have been asserting something false. What separates them is that no tint
    /// shares a hue with a money colour, and the two that come nearest are moss (a yellow-green, 38°
    /// off profit) and ochre (a gold, 46° off loss).
    @Test func noCabalTintReadsAsAMoneyColour() {
        for scheme in [UIUserInterfaceStyle.light, .dark] {
            let name = scheme == .light ? "light" : "dark"
            for tint in MonacoTheme.CabalTint.allCases {
                for (label, money) in [("profit", MonacoTheme.profit), ("loss", MonacoTheme.loss)] {
                    let hueGap = abs(OKLCh.value(tint.fill, scheme).hue - OKLCh.value(money, scheme).hue)
                    #expect(
                        min(hueGap, 360 - hueGap) >= 30,
                        "\(tint) vs \(label) in \(name): only \(min(hueGap, 360 - hueGap))° of hue apart"
                    )
                }
            }
        }
    }

    /// `disabledLabel` is the one token held *below* AA on purpose, so it needs a two-sided guard:
    /// AA-or-better makes an unavailable control read as a live one, and too low makes it
    /// unreadable. The upper bound is expressed against `tertiaryText` rather than a bare number,
    /// so the two cannot quietly converge back into the single token this split undid.
    @Test func disabledLabelStaysQuieterThanContent() {
        for surface in Self.surfaces {
            for scheme in [UIUserInterfaceStyle.light, .dark] {
                let disabled = WCAGContrast.ratio(MonacoTheme.disabledLabel, on: [surface], scheme)
                let content = WCAGContrast.ratio(MonacoTheme.tertiaryText, on: [surface], scheme)
                #expect(disabled < 4.5, "disabledLabel is \(disabled): a disabled control reads as live")
                #expect(disabled >= 2.5, "disabledLabel is \(disabled): unavailable is not the same as invisible")
                #expect(
                    content - disabled > 1,
                    "tertiaryText (\(content)) and disabledLabel (\(disabled)) have converged"
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
