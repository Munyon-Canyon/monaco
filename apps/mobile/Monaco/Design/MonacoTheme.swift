import SwiftUI
import UIKit

/// Shared visual tokens: a neutral canvas, one forest accent, and the money colours.
///
/// The values are the redesign's (`docs/design.md`); feature code uses the role names. The rule
/// that shapes the palette: **green means "this went up"**. The forest `brand` / `brandFill` is
/// the interactive colour and never a gain; `profit` is a lighter, far more saturated green.
/// `MonacoContrastTests` pins both, and every text pair's AA contrast.
enum MonacoTheme {
    // MARK: Surfaces

    static let background = Color.adaptive(light: 0xFFFFFF, dark: 0x000000)

    static let canvas = background

    static let surface = Color.adaptive(light: 0xF2F2F7, dark: 0x1C1C1E)

    static let surfaceSunken = Color.adaptive(light: 0xE5E5EA, dark: 0x2C2C2E)

    static let cashFill = border

    static let primaryText = Color.adaptive(light: 0x000000, dark: 0xFFFFFF)

    static let ink = primaryText

    static let bodyText = primaryText

    static let secondaryText = Color.adaptive(light: 0x636366, dark: 0xA1A1A6)

    static let muted = secondaryText

    static let tertiaryText = Color.adaptive(light: 0xAEAEB2, dark: 0x6E6E73)

    static let disabledLabel = Color.adaptive(light: 0x8E8E93, dark: 0x6E6E73)

    static let border = Color.adaptive(light: 0xC6C6C8, dark: 0x38383A)

    static let hairline = border

    // MARK: Brand

    static let tint = Color.adaptive(light: 0x1E5C3A, dark: 0x4DAE78)

    static let brand = tint

    static let brandFill = Color.adaptive(light: 0x1E5C3A, dark: 0x1E5C3A)

    static let onBrand = Color.adaptive(light: 0xFFFFFF, dark: 0xFFFFFF)

    static let brandSoft = surfaceSunken

    static let brandWash = brandSoft

    static let brandOnWash = tint

    static let warningWash = Color.adaptive(light: 0xF3ECD8, dark: 0x2C2819)

    // MARK: Glass and sheets

    static let sheetDim = Color(hex: 0x000000, alpha: 0.4)

    static let scrim = Color(hex: 0x000000, alpha: 0.6)

    // MARK: Retired hero band

    static let onHero = primaryText

    static let onHeroMuted = secondaryText

    static let onHeroHairline = hairline

    // MARK: Money

    static let profit = Color.adaptive(light: 0x00773A, dark: 0x30F57A)

    static let loss = Color.adaptive(light: 0xC80014, dark: 0xFF6259)

    static let profitVivid = Color.adaptive(light: 0x00843D, dark: 0x30F57A)
    static let onVivid = Color.adaptive(light: 0x0E1512, dark: 0x0E1512)

    static let lossVivid = Color.adaptive(light: 0xD70015, dark: 0xFF453A)

    static let profitWash = Color.clear

    static let lossWash = Color.clear

    static let profitOnWash = profit

    static let lossOnWash = loss

    static let profitWashOnHero = Color.clear

    static let lossWashOnHero = Color.clear

    static let profitOnHero = profit

    static let lossOnHero = loss

    static let buyWash = surface

    static let sellWash = surface

    // MARK: Toast

    static let toastFill = surface

    static let toastStroke = hairline

    static let toastLabel = primaryText

    static let toastSuccessGlyph = profit

    static let toastErrorGlyph = loss

    // MARK: Identity card

    static let identityFill = surface

    static let onIdentity = primaryText

    static let onIdentityMuted = secondaryText

    static let onIdentityBody = primaryText

    static let pencilFill = surfaceSunken

    // MARK: Covers

    static let onCover = Color.adaptive(light: 0xFFFFFF, dark: 0xFFFFFF)

    static let onCoverMuted = Color.adaptive(light: 0xC5D1CB, dark: 0xC5D1CB)

    static let onCoverGlass = Color.adaptive(light: 0xFFFFFF, lightAlpha: 0.09, dark: 0xFFFFFF, darkAlpha: 0.09)

    static let onCoverGlassStroke = Color.adaptive(
        light: 0xFFFFFF, lightAlpha: 0.14, dark: 0xFFFFFF, darkAlpha: 0.14)

    static let coverStroke = Color.adaptive(light: 0xFFFFFF, lightAlpha: 0.07, dark: 0xFFFFFF, darkAlpha: 0.07)

    static let coverOverlay = Color.adaptive(light: 0x00130B, lightAlpha: 0.80, dark: 0x00130B, darkAlpha: 0.80)

    static let coverOverlayReduced = Color.adaptive(light: 0x00130B, lightAlpha: 0.91, dark: 0x00130B, darkAlpha: 0.91)

    static let voteNeededFill = Color.adaptive(light: 0xD70015, dark: 0xD70015)

    static let voteNeededLabel = Color.adaptive(light: 0xFFFFFF, dark: 0xFFFFFF)

    static let discussionTint = tint

    static let discussionWash = surfaceSunken

    static let allocationTints: [Color] = [
        tint,
        Color.adaptive(light: 0x0040DD, dark: 0x0A84FF),
        Color.adaptive(light: 0x8944AB, dark: 0xBF5AF2),
        border,
    ]

    struct ShadowLayer {
        let color: Color
        let radius: CGFloat
        let y: CGFloat
    }

    enum Shadow {
        static let floating = ShadowLayer(
            color: Color.adaptive(light: 0x112E21, lightAlpha: 0.07, dark: 0x000000, darkAlpha: 0.20),
            radius: 24,
            y: 8)

        static let sheet = ShadowLayer(
            color: Color.adaptive(light: 0x001A15, lightAlpha: 0.25, dark: 0x000000, darkAlpha: 0.50),
            radius: 72,
            y: 22)
    }

    // MARK: Roles

    static let primaryButtonFill = brandFill

    static let primaryButtonLabel = onBrand

    static let secondaryButtonFill = surfaceSunken

    static let secondaryButtonLabel = primaryText

    static let destructive = loss

    static let onDestructive = Color.adaptive(light: 0xFFFFFF, dark: 0x000000)

    static let accent = tint

    static let disabled = Color.adaptive(light: 0xCBD2CB, dark: 0x37453D)

    static let success = profit

    static let warning = Color.adaptive(light: 0x866017, dark: 0xE6C477)

    // MARK: Gold

    static let crown = Color.adaptive(light: 0xFFCC00, dark: 0xFFD60A)

    static let gold = crown

    static let goldGlyph = crown

    /// Green for gains, red for losses, muted for zero / missing.
    /// Accepts ASCII "-" and the typographic minus "−" (U+2212) as a loss sign.
    static func signed(_ raw: String?) -> Color {
        let trimmed = raw?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        if trimmed.isEmpty || trimmed == "—" {
            return muted
        }
        let isNegative = trimmed.hasPrefix("-") || trimmed.hasPrefix("\u{2212}")
        let magnitude = trimmed.drop(while: { !$0.isNumber && $0 != "." })
        let digitsOnly = magnitude.filter { $0.isNumber || $0 == "." }
        if digitsOnly.isEmpty || digitsOnly.allSatisfy({ $0 == "0" || $0 == "." }) {
            return muted
        }
        if isNegative { return loss }
        if trimmed.hasPrefix("+") { return profit }
        let numeric =
            trimmed
            .replacingOccurrences(of: "%", with: "")
            .replacingOccurrences(of: ",", with: "")
            .replacingOccurrences(of: "$", with: "")
        if let value = Double(numeric) {
            if value > 0 { return profit }
            if value < 0 { return loss }
        }
        return muted
    }

    nonisolated enum CabalTint: CaseIterable {
        case indigo, orange, teal, pink, purple, brown, blue, cyan

        static func forGroupId(_ groupId: String) -> CabalTint {
            let all = CabalTint.allCases
            let key = groupId.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
            return all[Int(fnv1a64(key) % UInt64(all.count))]
        }

        static func fnv1a64(_ string: String) -> UInt64 {
            var hash: UInt64 = 0xCBF2_9CE4_8422_2325
            for byte in string.utf8 {
                hash ^= UInt64(byte)
                hash = hash &* 0x0000_0100_0000_01B3
            }
            return hash
        }
    }

    /// One family: SF Pro. Sizes keep the floor #4110 set after hands-on QA found text too small.
    /// A role at a system text style's size uses that style; display and button have no
    /// matching style and pre-scale with `scaled`. Figures add `.monospacedDigit()`; nothing sets letter-spacing,
    /// because SF Pro's optical tracking is the tracking.
    enum Typo {
        /// 28 bold: root tab titles.
        static var display: Font { scaled(size: 28, weight: .bold, relativeTo: .largeTitle) }
        /// 28 bold: moments, headings set in content, a stock name.
        static let title = Font.title.weight(.bold)
        /// 22 bold: the name on a pushed screen (cabal, stock, proposal).
        static let title2 = Font.title2.weight(.bold)
        /// 17 semibold: section and sheet titles, the Feed post's ticker and amount line.
        static let section = Font.headline
        /// 16 semibold: button labels.
        static var button: Font { scaled(size: 16, weight: .semibold, relativeTo: .callout) }
        /// 15 semibold: row, post, comment and tile titles, row values.
        static let headline = Font.subheadline.weight(.semibold)
        /// 15 regular: a thesis, comments, sheet and empty-state copy.
        static let body = Font.subheadline
        /// 15 regular (F's 13 at the 15 pt floor): second lines, trailing changes, stat labels.
        static let subhead = Font.subheadline
        /// 15 semibold: text links, chip and segment labels, toast text, a reaction chip's count.
        static let subheadStrong = Font.subheadline.weight(.semibold)
        /// 15 medium: timestamps, "Closes in 2d", attribution.
        static let caption = Font.subheadline.weight(.medium)
        /// 15 semibold: the "You" tag, a count capsule, an outcome chip.
        static let captionStrong = Font.subheadline.weight(.semibold)
        /// 15 semibold, tabular: rank numerals in rows and on the podium.
        static let rank = Font.subheadline.weight(.semibold).monospacedDigit()

        // MARK: Retired roles, aliased until their call sites move

        static var rowTitle: Font { headline }
        static var bodyStrong: Font { headline }
        static let callout = subhead
        static let calloutStrong = subheadStrong
        static let micro = captionStrong
        /// The SF Mono market voice is gone: a ticker sets in SF Pro.
        static var ticker: Font { headline }
        static let quote = Font.subheadline.weight(.semibold).monospacedDigit()
        static let data = Font.subheadline.monospacedDigit()
        static let dataStrong = Font.subheadline.weight(.semibold).monospacedDigit()
        static let dataCaption = Font.subheadline.weight(.medium).monospacedDigit()
        static let dataMicro = Font.footnote.weight(.semibold).monospacedDigit()
        static let stamp = Font.footnote.weight(.medium).monospacedDigit()

        /// SF Pro at a size no system text style has, pre-scaled against the process-wide content
        /// size category. SwiftUI scales only a custom-named font with `relativeTo:`, never the
        /// system font at a custom size.
        // ponytail: a pre-scaled size ignores a `.dynamicTypeSize` cap and picks up a text-size
        // change on the view's next render; a `@ScaledMetric` modifier like `MoneyFont` fixes both
        // once these roles move off `Font` statics. No caller sits under a cap today.
        static func scaled(size: CGFloat, weight: Font.Weight, relativeTo style: UIFont.TextStyle) -> Font {
            Font.system(size: UIFontMetrics(forTextStyle: style).scaledValue(for: size), weight: weight)
        }
    }

    enum Radius {
        static let chip: CGFloat = 12
        static let card: CGFloat = 12
        static let cardTight: CGFloat = 12
        static let button: CGFloat = 10
        static let shareCard: CGFloat = 16
        static let small: CGFloat = 8
        static let search: CGFloat = 16
        static let notice: CGFloat = 12
        static let badge: CGFloat = 8
        static let pill: CGFloat = 6
        static let identity: CGFloat = 12
        static let tile: CGFloat = 6
        static let field: CGFloat = 8
        static let bubble: CGFloat = 17
        static let toast: CGFloat = 15
        static let logoFraction: CGFloat = 0.15
        static let keypadKey: CGFloat = 8
        static let crest: CGFloat = 6
        static let squircleFraction: CGFloat = 0.225
    }

    enum Space {
        static let xs: CGFloat = 4
        static let s: CGFloat = 8
        static let sm: CGFloat = 12
        static let m: CGFloat = 16
        static let l: CGFloat = 24
        static let xl: CGFloat = 32
        static let section: CGFloat = 16
        static let root: CGFloat = 24
        static let cardPadding: CGFloat = 12
        static let titleGap: CGFloat = 6
        static let stackGap: CGFloat = 8
        static let tabClear: CGFloat = 104
        static let homeClear: CGFloat = 40
        static let gutter: CGFloat = 16
    }

    enum ReactionChipMetrics {
        static let visibleHeight: CGFloat = 24
        static let compactVisibleHeight: CGFloat = 22
        static let minWidth: CGFloat = 44
        static let horizontalPadding: CGFloat = 8
        static let gap: CGFloat = 4
        static let tapSize: CGFloat = 44
    }
}

extension MonacoTheme.CabalTint {
    var fill: Color {
        switch self {
        case .indigo: return Color(hex: 0x5E5CE6)
        case .orange: return Color(hex: 0xFF9F0A)
        case .teal: return Color(hex: 0x40C8E0)
        case .pink: return Color(hex: 0xFF375F)
        case .purple: return Color(hex: 0xBF5AF2)
        case .brown: return Color(hex: 0xAC8E68)
        case .blue: return Color(hex: 0x0A84FF)
        case .cyan: return Color(hex: 0x64D2FF)
        }
    }

    var onFill: Color { self == .indigo ? .white : .black }

    var soft: Color { fill.opacity(0.12) }

    var onInk: Color { stroke }

    var stroke: Color { fill }

    /// Background fill for a cabal: `CabalTint.forGroupId(groupId).fill`.
    static func fill(forGroupId groupId: String) -> Color {
        forGroupId(groupId).fill
    }

    /// Low-alpha wash for a cabal: `CabalTint.forGroupId(groupId).soft`.
    static func soft(forGroupId groupId: String) -> Color {
        forGroupId(groupId).soft
    }

    /// Chart line colour for a cabal: `CabalTint.forGroupId(groupId).stroke`.
    static func stroke(forGroupId groupId: String) -> Color {
        forGroupId(groupId).stroke
    }
}

/// Retired with Avenir Next: nothing in the app sets it any more. The PostScript names, so a
/// weight asked for in SwiftUI terms lands on a real face.
enum MonacoTypeface {
    static func avenirNext(_ weight: Font.Weight) -> String {
        switch weight {
        case .ultraLight, .thin, .light: return "AvenirNext-Regular"
        case .regular: return "AvenirNext-Regular"
        case .medium: return "AvenirNext-Medium"
        case .semibold: return "AvenirNext-DemiBold"
        case .bold, .heavy, .black: return "AvenirNext-Bold"
        default: return "AvenirNext-DemiBold"
        }
    }
}

extension Color {
    /// A single fixed colour from an 0xRRGGBB literal — same in both schemes.
    init(hex: UInt32, alpha: Double = 1) {
        self.init(uiColor: UIColor(hex: hex, alpha: alpha))
    }

    nonisolated static func adaptive(
        light: UInt32, lightAlpha: Double = 1, dark: UInt32, darkAlpha: Double = 1
    ) -> Color {
        Color(
            uiColor: UIColor { traits in
                traits.userInterfaceStyle == .dark
                    ? UIColor(hex: dark, alpha: darkAlpha)
                    : UIColor(hex: light, alpha: lightAlpha)
            }
        )
    }
}

extension UIColor {
    nonisolated convenience init(hex: UInt32, alpha: Double = 1) {
        self.init(
            red: CGFloat((hex >> 16) & 0xFF) / 255,
            green: CGFloat((hex >> 8) & 0xFF) / 255,
            blue: CGFloat(hex & 0xFF) / 255,
            alpha: CGFloat(alpha)
        )
    }
}
