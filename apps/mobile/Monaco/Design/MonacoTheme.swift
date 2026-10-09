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

    /// Every screen, the tab bar, and the nav bar once content scrolls under it.
    static let background = Color.adaptive(light: 0xFFFFFF, dark: 0x0B0F0D)

    static let canvas = background

    /// Sheets and menus. The same as the canvas in light, on purpose.
    static let surface = Color.adaptive(light: 0xFFFFFF, dark: 0x131816)

    /// Fields, chips at rest, cards, the secondary button, skeletons, pressed rows.
    static let surfaceSunken = Color.adaptive(light: 0xF2F4F2, dark: 0x1B211E)

    static let cashFill = Color.adaptive(light: 0x76837B, dark: 0x7A8B80)

    static let primaryText = Color.adaptive(light: 0x0E1512, dark: 0xF2F4F2)

    static let ink = primaryText

    /// Subtitles and labels under figures.
    static let secondaryText = Color.adaptive(light: 0x5E6963, dark: 0x9AA59E)

    static let muted = secondaryText

    /// Timestamps, ranks, attribution. Light is a step darker than the spec's `#6E7872`, which is
    /// 4.1:1 on `surfaceSunken`: a proposal card's "Closes in 2d" sits there and has to clear AA.
    ///
    /// Not for disabled controls or placeholders: see `disabledLabel`.
    static let tertiaryText = Color.adaptive(light: 0x68716C, dark: 0x86908A)

    /// Disabled control labels and field placeholders. Below AA on purpose (WCAG 1.4.3 exempts
    /// disabled controls, and unavailable has to look unavailable), but held at 2.5:1 or more so it
    /// stays legible; light is a step darker than the spec's `#A3ABA6` (2.1:1 on sunken) for that.
    static let disabledLabel = Color.adaptive(light: 0x949C97, dark: 0x626B66)

    /// 1px separators, never 1pt.
    static let border = Color.adaptive(light: 0xE8EBE8, dark: 0x242B27)

    static let hairline = border

    // MARK: Brand

    /// The accent as text and glyphs: text links, the focus ring. 8.1:1 light, 11:1 dark.
    static let brand = Color.adaptive(light: 0x1F5A3D, dark: 0x9ED5B3)

    /// Primary button, selected chip and segment, my chat bubble. Light is the logo's forest.
    static let brandFill = Color.adaptive(light: 0x0F291C, dark: 0xEDF2EF)

    static let onBrand = Color.adaptive(light: 0xFFFFFF, dark: 0x0F291C)

    /// The viewer's own row on a board, and the "You" tag behind it.
    static let brandWash = Color.adaptive(light: 0x0F291C, lightAlpha: 0.08, dark: 0x9ED5B3, darkAlpha: 0.14)

    /// Brand text on `brandWash`.
    static let brandOnWash = brand

    // MARK: Retired hero band

    /// The dark hero band is gone. These alias the canvas and its text until their call sites
    /// move, so a screen that still draws a "hero" draws it on the canvas.
    static let heroInk = canvas

    static let onHero = primaryText

    static let onHeroMuted = secondaryText

    static let onHeroHairline = hairline

    // MARK: Money

    /// Signed P&L text. Clears AA on `canvas`, `surface` and `surfaceSunken`.
    static let profit = Color.adaptive(light: 0x0A7F46, dark: 0x2EDB8A)

    static let loss = Color.adaptive(light: 0xD0342C, dark: 0xFF6B5E)

    /// Chart strokes and the live dot only.
    static let profitVivid = Color.adaptive(light: 0x12B76A, dark: 0x2EDB8A)

    static let lossVivid = Color.adaptive(light: 0xE5484D, dark: 0xFF6B5E)

    /// Nothing draws a P&L wash any more: a gain or a loss is coloured text. These stay clear,
    /// with the bare money colours on them, until the badges that read them are text.
    static let profitWash = Color.clear

    static let lossWash = Color.clear

    static let profitOnWash = profit

    static let lossOnWash = loss

    static let profitWashOnHero = Color.clear

    static let lossWashOnHero = Color.clear

    static let profitOnHero = profit

    static let lossOnHero = loss

    // MARK: Toast

    /// The toast inverts the scheme: a dark panel in light, a light one in dark. Never the brand
    /// fill, or it reads as a second primary button (`theToastIsNotTheBrandFill`).
    static let toastFill = Color.adaptive(light: 0x33413A, dark: 0xF2F4F2)

    /// Hairline on the toast. Carries the capsule's edge in dark, where the drop shadow is invisible.
    static let toastStroke = Color.adaptive(light: 0x33413A, lightAlpha: 0, dark: 0xFFFFFF, darkAlpha: 0.10)

    static let toastLabel = Color.adaptive(light: 0xFFFFFF, dark: 0x0E1512)

    /// State glyphs on `toastFill`. The panel is the other scheme's surface, so the glyphs take
    /// the other scheme's money colours: the dark-mode green and the spec's `#FF8A7A` on the dark
    /// panel, the light-mode `profit` and `loss` on the light one. 4.5:1 or more in both.
    static let toastSuccessGlyph = Color.adaptive(light: 0x2EDB8A, dark: 0x0A7F46)

    static let toastErrorGlyph = Color.adaptive(light: 0xFF8A7A, dark: 0xD0342C)

    // MARK: Roles

    static let primaryButtonFill = brandFill

    static let primaryButtonLabel = onBrand

    static let secondaryButtonFill = surfaceSunken

    static let secondaryButtonLabel = primaryText

    static let destructive = loss

    static let onDestructive = Color.white

    static let accent = brand

    static let disabled = Color.adaptive(light: 0xCBD2CB, dark: 0x37453D)

    static let success = profit

    /// Pending, paused, after hours.
    static let warning = Color.adaptive(light: 0x8A5A16, dark: 0xE5B26A)

    // MARK: Gold

    /// Retired with the board's rank caption; kept until its last row moves.
    static let gold = Color.adaptive(light: 0x7A5C12, dark: 0xE2B04A)

    /// The rank-1 crown glyph, and nothing else.
    static let goldGlyph = Color.adaptive(light: 0xC9A24A, dark: 0xD9B85A)

    /// Retired with the leader's row wash; kept until its last two users move.
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
        /// 36 bold: tab root titles, the sign-in wordmark, a profile name.
        static var display: Font { scaled(size: 36, weight: .bold, relativeTo: .largeTitle) }
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
        static let card: CGFloat = 16
        static let sheet: CGFloat = 24
        /// `CabalMark` at 40pt; marks scale this proportionally (size × 0.3).
        static let tile: CGFloat = 12
        static let field: CGFloat = 12
        static let bubble: CGFloat = 18
        static let toast: CGFloat = 14
    }

    enum Space {
        static let xs: CGFloat = 4
        static let s: CGFloat = 8
        static let sm: CGFloat = 12
        static let m: CGFloat = 16
        static let l: CGFloat = 24
        static let xl: CGFloat = 32
        /// Screen side padding, and the only horizontal inset.
        static let gutter: CGFloat = 16
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
