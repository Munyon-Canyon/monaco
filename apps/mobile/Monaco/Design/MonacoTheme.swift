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

    static let background = Color.adaptive(light: 0xFAF7F0, dark: 0x03130F)

    static let canvas = background

    static let surface = Color.adaptive(light: 0xF6F7F2, dark: 0x0A2119)

    static let surfaceSunken = Color.adaptive(light: 0xEDEFE7, dark: 0x102A20)

    static let cashFill = border

    static let primaryText = Color.adaptive(light: 0x0C3526, dark: 0xF5F8F4)

    static let ink = primaryText

    static let bodyText = Color.adaptive(light: 0x172C24, dark: 0xE6EFEB)

    static let secondaryText = Color.adaptive(light: 0x646D67, dark: 0x9EAFA5)

    static let muted = secondaryText

    static let tertiaryText = Color.adaptive(light: 0x646D67, dark: 0x8E9E95)

    static let disabledLabel = Color.adaptive(light: 0x8F9791, dark: 0x5E6B63)

    static let border = Color.adaptive(light: 0xE5E7DF, dark: 0x18342A)

    static let hairline = border

    // MARK: Brand

    static let brand = Color.adaptive(light: 0x07643E, dark: 0x60EFAA)

    static let brandFill = Color.adaptive(light: 0x0D5C3A, dark: 0x66EFAB)

    static let onBrand = Color.adaptive(light: 0xFFFFFF, dark: 0x072219)

    static let brandSoft = Color.adaptive(light: 0xE8F0E7, dark: 0x14372A)

    static let brandWash = brandSoft

    static let brandOnWash = brand

    static let warningWash = Color.adaptive(light: 0xF3ECD8, dark: 0x2C2819)

    // MARK: Glass and sheets

    static let glassFill = Color.adaptive(light: 0xFBFCF7, lightAlpha: 0.86, dark: 0x0C231B, darkAlpha: 0.85)

    static let glassStroke = Color.adaptive(light: 0xFFFFFF, lightAlpha: 0.98, dark: 0xB0EAC8, darkAlpha: 0.20)

    static let sheetGlass = Color.adaptive(light: 0xF8FAF5, lightAlpha: 0.90, dark: 0x11291F, darkAlpha: 0.94)

    static let sheetStroke = Color.adaptive(light: 0xFFFFFF, lightAlpha: 0.95, dark: 0xA8E3C2, darkAlpha: 0.21)

    static let sheetDim = Color.adaptive(light: 0x000E08, lightAlpha: 0.30, dark: 0x000604, darkAlpha: 0.48)

    static let scrim = Color.adaptive(light: 0x01150E, lightAlpha: 0.33, dark: 0x000000, darkAlpha: 0.73)

    // MARK: Retired hero band

    static let heroInk = canvas

    static let onHero = primaryText

    static let onHeroMuted = secondaryText

    static let onHeroHairline = hairline

    // MARK: Money

    static let profit = Color.adaptive(light: 0x0F7A4E, dark: 0x68EBAB)

    static let loss = Color.adaptive(light: 0xB54647, dark: 0xF1807F)

    static let profitVivid = Color.adaptive(light: 0x108153, dark: 0x68EBAB)
    static let onVivid = Color.adaptive(light: 0x0E1512, dark: 0x0E1512)

    static let lossVivid = Color.adaptive(light: 0xBD5455, dark: 0xF1807F)

    static let profitWash = Color.clear

    static let lossWash = Color.clear

    static let profitOnWash = profit

    static let lossOnWash = loss

    static let profitWashOnHero = Color.clear

    static let lossWashOnHero = Color.clear

    static let profitOnHero = profit

    static let lossOnHero = loss

    static let buyWash = Color.adaptive(light: 0xE9F4EC, dark: 0x0E3022)

    static let sellWash = Color.adaptive(light: 0xF8ECE9, dark: 0x30201F)

    static let priceWash = Color.adaptive(light: 0xEDF0F9, dark: 0x13232B)

    // MARK: Toast

    static let toastFill = Color.adaptive(light: 0x0C3526, dark: 0xF5F8F4)

    static let toastStroke = Color.clear

    static let toastLabel = Color.adaptive(light: 0xFAF7F0, dark: 0x03130F)

    static let toastSuccessGlyph = Color.adaptive(light: 0x68EBAB, dark: 0x0F7A4E)

    static let toastErrorGlyph = Color.adaptive(light: 0xFFAAAA, dark: 0xB54647)

    // MARK: Identity card

    static let identityFill = Color.adaptive(light: 0x0C3828, dark: 0x0D2C21)

    static let onIdentity = Color.adaptive(light: 0xF8FAF5, dark: 0xF5FAF6)

    static let onIdentityMuted = Color.adaptive(light: 0xB6D5C3, dark: 0xB6D5C3)

    static let onIdentityBody = Color.adaptive(light: 0xE0EEE3, dark: 0xE0EEE3)

    static let profitOnIdentity = Color.adaptive(light: 0x81E8B1, dark: 0x81E8B1)

    static let lossOnIdentity = Color.adaptive(light: 0xFFAAAA, dark: 0xFFAAAA)

    static let pencilFill = Color.adaptive(light: 0xE6F5E8, dark: 0xE6F5E8)

    // MARK: Covers

    static let onCover = Color.adaptive(light: 0xFFFFFF, dark: 0xFFFFFF)

    static let onCoverMuted = Color.adaptive(light: 0xC5D1CB, dark: 0xC5D1CB)

    static let profitOnCover = Color.adaptive(light: 0x83F1B9, dark: 0x83F1B9)

    static let lossOnCover = Color.adaptive(light: 0xFFB2B0, dark: 0xFFB2B0)

    static let onCoverGlass = Color.adaptive(light: 0xFFFFFF, lightAlpha: 0.09, dark: 0xFFFFFF, darkAlpha: 0.09)

    static let onCoverGlassStroke = Color.adaptive(
        light: 0xFFFFFF, lightAlpha: 0.14, dark: 0xFFFFFF, darkAlpha: 0.14)

    static let coverStroke = Color.adaptive(light: 0xFFFFFF, lightAlpha: 0.07, dark: 0xFFFFFF, darkAlpha: 0.07)

    static let coverOverlay = Color.adaptive(light: 0x00130B, lightAlpha: 0.80, dark: 0x00130B, darkAlpha: 0.80)

    static let coverOverlayReduced = Color.adaptive(light: 0x00130B, lightAlpha: 0.91, dark: 0x00130B, darkAlpha: 0.91)

    static let voteNeededFill = Color.adaptive(light: 0xC8F7CF, dark: 0xC8F7CF)

    static let voteNeededLabel = Color.adaptive(light: 0x083123, dark: 0x083123)

    static let discussionTint = Color.adaptive(light: 0x535AAE, dark: 0xC7B9F7)

    static let discussionWash = Color.adaptive(
        light: 0x676BDD, lightAlpha: 0.075, dark: 0xBCA4FF, darkAlpha: 0.10)

    static let allocationTints: [Color] = [
        brand,
        Color.adaptive(light: 0x6F8296, dark: 0x7B91A7),
        Color.adaptive(light: 0x8C77A4, dark: 0x9A83B4),
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

    static let secondaryButtonFill = brandSoft

    static let secondaryButtonLabel = brand

    static let destructive = loss

    static let onDestructive = Color.adaptive(light: 0xFAF7F0, dark: 0x03130F)

    static let accent = brand

    static let disabled = Color.adaptive(light: 0xCBD2CB, dark: 0x37453D)

    static let success = profit

    static let warning = Color.adaptive(light: 0x866017, dark: 0xE6C477)

    // MARK: Gold

    static let gold = Color.adaptive(light: 0x7A5C12, dark: 0xE2B04A)

    static let goldGlyph = Color.adaptive(light: 0xC9A24A, dark: 0xD9B85A)

    static let goldWash = Color.adaptive(light: 0xC9A24A, lightAlpha: 0.16, dark: 0xD9B85A, darkAlpha: 0.16)

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

    /// Saturated identity tints. Picked from the group id, never from the name, so a rename keeps
    /// the colour. `soft` is the low-alpha wash for tinted areas that still hold ink text.
    ///
    /// Retuned for the forest brand. The old five were carried over from the electric-blue app and
    /// none of them was a money colour, which was the correctness bar — but at full saturation a
    /// teal, a safety orange and a hot crimson are a different design language from cream paper and
    /// forest ink, and a cabal row sat between the two. These five keep the same job and the same
    /// separation while belonging to the palette around them.
    ///
    /// The constraint is not contrast, it is *hue*. Five cabals have to be five colours at a glance,
    /// and none of them may be mistaken for `profit` or `loss` in a row that also carries money:
    ///
    /// | tint   | hue  | L*    | from `profit` | from `loss` |
    /// |--------|------|-------|---------------|-------------|
    /// | pine   | 190° | 0.487 | 35°           | 159°        |
    /// | ochre  |  78° | 0.535 | 78°           |  46°        |
    /// | plum   | 341° | 0.447 | 175°          |  51°        |
    /// | indigo | 255° | 0.452 |  99°          | 137°        |
    /// | moss   | 118° | 0.438 |  38°          |  93°        |
    ///
    /// Hue alone is not enough, which a first pass proved on the Home list: ochre and moss are the
    /// closest pair at 40°, and at equal lightness two cabals in adjacent rows read as one colour.
    /// So every pair is separated by 60° of hue *or* 0.08 of L\*, and ochre and moss take the
    /// second route — moss is a deep olive where ochre is a mid gold.
    ///
    /// That is also why moss barely lifts in dark while the others do. Lifting each fill to the
    /// same contrast floor independently flattened the ladder and put ochre and moss back within
    /// 0.024 of each other; the ladder is the constraint, and white initials clear AA on a darker
    /// tile anyway.
    ///
    /// The closest approach to a money colour is moss at 38° from profit. Hue is what does that
    /// work — these tints are not quieter than the money colours, they run 0.9× to 1.2× profit's
    /// chroma — and it is enough because a tint only ever fills a mark or a stripe, never a figure.
    ///
    /// White initials clear 4.5:1 on every fill in both schemes, where the old palette met only the
    /// 3:1 large-text bar in dark. That was defensible for 15pt bold initials; holding the body bar
    /// costs nothing here and means `fill` is safe behind small white text too.
    nonisolated enum CabalTint: CaseIterable {
        // Order is the identity mapping: a cabal's tint is its id hashed mod 5, so these stay in
        // their slots and no existing cabal changes which of the five it gets.
        case pine, ochre, plum, indigo, moss

        /// The one tint function for a cabal. Every surface (rows, strip cards, hero, chat header, profile)
        /// passes the cabal's `groupId`, never its name, so a cabal is the same colour everywhere.
        /// Stable across launches: FNV-1a 64 over the UTF-8 bytes of the trimmed, lowercased id, mod 5
        /// (lowercased because Swift's `UUID.uuidString` is uppercase while the API sends lowercase).
        /// Never `String.hashValue`, which is randomised per launch.
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
    /// A role at a system text style's size uses that style; display, headline and body have no
    /// matching style and pre-scale with `scaled`. Figures add `.monospacedDigit()`; nothing sets
    /// letter-spacing, because SF Pro's optical tracking is the tracking.
    enum Typo {
        /// 30 bold: tab root titles, the sign-in wordmark, a profile name.
        static var display: Font { scaled(size: 30, weight: .bold, relativeTo: .largeTitle) }
        /// 28 semibold: headings set in content, a stock name, keypad digits.
        static let title = Font.title.weight(.semibold)
        /// 22 semibold: section titles.
        static let section = Font.title2.weight(.semibold)
        /// 18 semibold: row titles, button labels, card headlines.
        static var headline: Font { scaled(size: 18, weight: .semibold, relativeTo: .body) }
        /// 18 regular: prose, chat, reasons.
        static var body: Font { scaled(size: 18, weight: .regular, relativeTo: .body) }
        /// 17 regular: row subtitles, helper lines, labels under figures.
        static let subhead = Font.body
        /// 17 semibold: text links, chip and segment labels, toast text.
        static let subheadStrong = Font.headline
        /// 15 medium: timestamps, "Closes in 2d", attribution.
        static let caption = Font.subheadline.weight(.medium)
        /// 15 semibold: the "You" tag, a count capsule, an outcome chip.
        static let captionStrong = Font.subheadline.weight(.semibold)

        // MARK: Retired roles, aliased until their call sites move

        static var rowTitle: Font { headline }
        static var bodyStrong: Font { headline }
        static var button: Font { headline }
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

    /// Squarer than before. The 24pt cards read as the app's whole personality, and now that a
    /// section is rows on the paper rather than a card, the few cards left are things a member
    /// acts on and want to sit flat, not float.
    enum Radius {
        static let chip: CGFloat = 12
        static let card: CGFloat = 20
        static let cardTight: CGFloat = 18
        static let search: CGFloat = 16
        static let notice: CGFloat = 15
        static let badge: CGFloat = 8
        static let pill: CGFloat = 6
        static let identity: CGFloat = 22
        static let sheet: CGFloat = 24
        /// `CabalMark` at 40pt; marks scale this proportionally (size × 0.3).
        static let tile: CGFloat = 12
        static let field: CGFloat = 15
        static let bubble: CGFloat = 17
        static let toast: CGFloat = 15
    }

    enum Space {
        static let xs: CGFloat = 4
        static let s: CGFloat = 8
        static let sm: CGFloat = 12
        static let m: CGFloat = 16
        static let l: CGFloat = 24
        static let xl: CGFloat = 32
        static let section: CGFloat = 24
        /// Screen side padding, and the only horizontal inset.
        static let gutter: CGFloat = 20
    }
}

extension MonacoTheme.CabalTint {
    /// Mark tile, accent stripe, chart key.
    var fill: Color {
        switch self {
        case .pine: return Color.adaptive(light: 0x0E6E6A, dark: 0x11827D)
        case .ochre: return Color.adaptive(light: 0x8F6410, dark: 0x9A6C11)
        case .plum: return Color.adaptive(light: 0x7A3A66, dark: 0xB15494)
        case .indigo: return Color.adaptive(light: 0x2F5788, dark: 0x4076B9)
        case .moss: return Color.adaptive(light: 0x4E5817, dark: 0x545F19)
        }
    }

    /// Initials and glyphs drawn on `fill`.
    var onFill: Color { .white }

    /// Low-alpha wash of `fill` for tinted surfaces that still carry ink text.
    var soft: Color { fill.opacity(0.12) }

    /// Retired with the dark hero band: the tint on the canvas, which is what `stroke` is tuned for.
    var onInk: Color { stroke }

    /// Chart line colour for this cabal.
    var stroke: Color {
        switch self {
        case .pine: return Color.adaptive(light: 0x0E6E6A, dark: 0x35C4BD)
        case .ochre: return Color.adaptive(light: 0x8F6410, dark: 0xE2B04A)
        case .plum: return Color.adaptive(light: 0x7A3A66, dark: 0xD68CC2)
        case .indigo: return Color.adaptive(light: 0x2F5788, dark: 0x7DAFE0)
        case .moss: return Color.adaptive(light: 0x4E5817, dark: 0xB8CC63)
        }
    }

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
