import MonacoCore
import SwiftUI

/// Size role for every figure that renders `$`, `%` or a share count.
enum MoneyStyle {
    case hero, large, row, caption

    /// Design size at the default text size.
    var baseSize: CGFloat {
        switch self {
        case .hero: return 48
        case .large: return 32
        case .row: return 18
        case .caption: return 15
        }
    }

    /// The text style the figure scales with.
    var textStyle: Font.TextStyle {
        switch self {
        case .hero: return .largeTitle
        case .large: return .title
        case .row: return .body
        case .caption: return .subheadline
        }
    }

    var weight: Font.Weight {
        switch self {
        case .hero, .large: return .bold
        case .row, .caption: return .semibold
        }
    }

    /// Hero figures shrink before they wrap; rows keep their size and truncate last.
    var minimumScaleFactor: CGFloat {

        switch self {
        case .hero: return 0.5
        case .large: return 0.6
        case .row, .caption: return 0.8
        }
    }
}

/// Scales a money figure inside the view tree, so a `.dynamicTypeSize` cap applies to it and a
/// text-size change invalidates the view. a pre-scaled font cannot do either: it asks
/// `UIFontMetrics` for a size once, outside the environment, and returns a fixed-size font.
struct MoneyFont: ViewModifier {
    let style: MoneyStyle
    var weightOverride: Font.Weight?

    // `@ScaledMetric` needs its text style and base size as literals in the property wrapper, so
    // there is one per role rather than one driven by `style`. The sizes come from `MoneyStyle`
    // so the two cannot drift; the text styles are asserted against it in `MoneyStyleScalingTests`.
    @ScaledMetric(relativeTo: .largeTitle) private var hero = MoneyStyle.hero.baseSize
    @ScaledMetric(relativeTo: .title) private var large = MoneyStyle.large.baseSize
    @ScaledMetric(relativeTo: .body) private var row = MoneyStyle.row.baseSize
    @ScaledMetric(relativeTo: .subheadline) private var caption = MoneyStyle.caption.baseSize

    private var size: CGFloat {
        switch style {
        case .hero: return hero
        case .large: return large
        case .row: return row
        case .caption: return caption
        }
    }

    func body(content: Content) -> some View {
        // SF Pro with tabular figures, so a column of money lines up.
        content.font(.system(size: size, weight: weightOverride ?? style.weight).monospacedDigit())
    }
}

extension View {
    /// The one way to set a money figure's font.
    func moneyFont(_ style: MoneyStyle, weight: Font.Weight? = nil) -> some View {
        modifier(MoneyFont(style: style, weightOverride: weight))
    }
}

/// "$1,248.50" in tabular SF Pro. Rolls digits on change unless Reduce Motion is on.
struct MoneyText: View {
    private let text: String
    private let numericValue: Double?
    private let style: MoneyStyle
    private let color: Color

    init(_ usd: Decimal, style: MoneyStyle, color: Color = MonacoTheme.ink) {
        text = UsdAmountFormatter.format(decimal: usd)
        numericValue = (usd as NSDecimalNumber).doubleValue
        self.style = style
        self.color = color
    }

    /// Unparseable input renders "—" in muted.
    init(decimalString: String, style: MoneyStyle, color: Color = MonacoTheme.ink) {
        let trimmed = decimalString.trimmingCharacters(in: .whitespacesAndNewlines)
        if let decimal = Decimal(string: trimmed, locale: Locale(identifier: "en_US_POSIX")),
            trimmed.allSatisfy({ $0.isNumber || $0 == "." || $0 == "-" || $0 == "+" })
        {
            text = UsdAmountFormatter.format(decimal: decimal)
            numericValue = (decimal as NSDecimalNumber).doubleValue
            self.color = color
        } else {
            text = "—"
            numericValue = nil
            self.color = MonacoTheme.muted
        }
        self.style = style
    }

    init(micros: Int64, style: MoneyStyle, color: Color = MonacoTheme.ink) {
        self.init(Decimal(micros) / Decimal(1_000_000), style: style, color: color)
    }

    var body: some View {
        MoneyFigure(text: text, value: numericValue, style: style, color: color)
    }
}

/// Signed dollar P&L: "+$48.20" profit, "−$7.60" loss, "$0.00" muted when it rounds to zero.
struct PnLText: View {
    private let dollarPnl: String
    private let style: MoneyStyle

    init(dollarPnl: String, style: MoneyStyle) {
        self.dollarPnl = dollarPnl
        self.style = style
    }

    init(signedMicros: Int64, style: MoneyStyle) {
        self.dollarPnl = UsdAmountFormatter.format(signedMicros: signedMicros)
        self.style = style
    }

    var body: some View {
        MoneyFigure(
            text: SignedUsdFormatter.format(dollarPnl),
            value: SignedUsdFormatter.parse(dollarPnl).map { ($0 as NSDecimalNumber).doubleValue },
            style: style,
            color: PnLTone(dollarPnl: dollarPnl).color
        )
        .accessibilityLabel(PnLSpeech.dollars(dollarPnl))
    }
}

/// Signed return: "+9.6%" / "−3.6%" coloured; nil or "—" muted.
struct PercentText: View {
    private let formatted: String
    private let style: MoneyStyle

    init(percentReturn: String?, style: MoneyStyle) {
        formatted = PercentReturnFormatter.format(percentReturn)
        self.style = style
    }

    init(basisPoints: Int64, style: MoneyStyle) {
        formatted = PercentFormatter.format(basisPoints: basisPoints, signed: true)
        self.style = style
    }

    var body: some View {
        MoneyFigure(
            text: formatted,
            value: PnLSpeech.percentValue(formatted),
            style: style,
            color: MonacoTheme.signed(formatted)
        )
        .accessibilityLabel(PnLSpeech.percent(formatted))
    }
}

/// A gain or a loss as coloured text, never a capsule: "+$48.20 (+9.6%)". Dollars only when the
/// percent is missing.
struct PnLBadge: View {
    private let dollarPnl: String
    private let percentReturn: String?
    private let style: MoneyStyle

    init(dollarPnl: String, percentReturn: String?, style: MoneyStyle = .caption) {
        self.dollarPnl = dollarPnl
        self.percentReturn = percentReturn
        self.style = style
    }

    init(signedMicros: Int64, basisPoints: Int64, style: MoneyStyle = .caption) {
        self.dollarPnl = UsdAmountFormatter.format(signedMicros: signedMicros)
        self.percentReturn = PercentFormatter.format(basisPoints: basisPoints, signed: true)
        self.style = style
    }

    static func label(dollarPnl: String, percentReturn: String?) -> String {
        let dollars = SignedUsdFormatter.format(dollarPnl)
        let percent = PercentReturnFormatter.format(percentReturn)
        return percent == "—" ? dollars : "\(dollars) (\(percent))"
    }

    var body: some View {
        Text(Self.label(dollarPnl: dollarPnl, percentReturn: percentReturn))
            .moneyFont(style)
            .foregroundStyle(PnLTone(dollarPnl: dollarPnl).color)
            .lineLimit(1)
            .minimumScaleFactor(0.8)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel("Profit and loss")
            .accessibilityValue(PnLSpeech.badge(dollarPnl: dollarPnl, percentReturn: percentReturn))
    }
}

/// "In cabals" / "$1,248.50" / "+$48.20 (+9.6%) All time"; a missing change reads "$0.00 (0.0%)".
struct MoneyHero: View {
    let label: String
    let value: String
    var dollarChange: String?
    var percentChange: String?
    var period = "All time"
    var alignment: HorizontalAlignment = .leading

    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    private var change: String {
        guard let dollarChange else { return "$0.00 (0.0%)" }
        return PnLBadge.label(dollarPnl: dollarChange, percentReturn: percentChange)
    }

    var body: some View {
        VStack(alignment: alignment, spacing: MonacoTheme.Space.xs) {
            Text(label)
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText)
            Text(value)
                .moneyFont(.hero)
                .foregroundStyle(MonacoTheme.ink)
                .lineLimit(1)
                .minimumScaleFactor(MoneyStyle.hero.minimumScaleFactor)
                .contentTransition(reduceMotion ? .identity : .numericText())
                .animation(reduceMotion ? nil : .snappy(duration: 0.25), value: value)
                .modifier(HeroTypeCap(isHero: true))
            (Text(change).foregroundStyle(PnLTone(dollarPnl: dollarChange ?? "0").color)
                + Text(" \(period)").foregroundStyle(MonacoTheme.secondaryText))
                .moneyFont(.caption)
                .lineLimit(1)
                .minimumScaleFactor(0.8)
        }
        .frame(maxWidth: .infinity, alignment: Alignment(horizontal: alignment, vertical: .center))
        .accessibilityElement(children: .combine)
    }
}

// MARK: - Internals

private struct MoneyFigure: View {
    let text: String
    let value: Double?
    let style: MoneyStyle
    let color: Color

    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    var body: some View {
        Text(text)
            .moneyFont(style)
            .foregroundStyle(color)
            .lineLimit(1)
            .minimumScaleFactor(style.minimumScaleFactor)
            .contentTransition(reduceMotion || value == nil ? .identity : .numericText(value: value ?? 0))
            .animation(reduceMotion ? nil : .snappy, value: text)
            .modifier(HeroTypeCap(isHero: style == .hero))
    }
}

private struct HeroTypeCap: ViewModifier {
    let isHero: Bool

    func body(content: Content) -> some View {
        if isHero {
            content.dynamicTypeSize(...DynamicTypeSize.accessibility2)
        } else {
            content
        }
    }
}

nonisolated enum PnLTone {
    case profit, loss, flat

    init(dollarPnl: String) {
        if SignedUsdFormatter.isZero(dollarPnl) || SignedUsdFormatter.parse(dollarPnl) == nil {
            self = .flat
        } else if SignedUsdFormatter.isLoss(dollarPnl) {
            self = .loss
        } else {
            self = .profit
        }
    }
}

extension PnLTone {
    var color: Color {
        switch self {
        case .profit: return MonacoTheme.profit
        case .loss: return MonacoTheme.loss
        case .flat: return MonacoTheme.muted
        }
    }

    var wash: Color {
        switch self {
        case .profit: return MonacoTheme.profitWash
        case .loss: return MonacoTheme.lossWash
        case .flat: return .clear
        }
    }

    /// Text drawn on `wash`. Deeper than `color`, which is derived for bare paper.
    var washColor: Color {
        switch self {
        case .profit: return MonacoTheme.profitOnWash
        case .loss: return MonacoTheme.lossOnWash
        case .flat: return MonacoTheme.muted
        }
    }

    /// Saturated pair for figures drawn on a deep ink hero card.
    var inkCardColor: Color {
        switch self {
        case .profit: return MonacoTheme.profitOnHero
        case .loss: return MonacoTheme.lossOnHero
        case .flat: return MonacoTheme.onHeroMuted
        }
    }

    var inkCardWash: Color {
        switch self {
        case .profit: return MonacoTheme.profitWashOnHero
        case .loss: return MonacoTheme.lossWashOnHero
        case .flat: return .clear
        }
    }
}

/// Words VoiceOver reads for signed figures ("up 48 dollars 20 cents, 9.6 percent").
enum PnLSpeech {
    static func dollars(_ raw: String) -> String {
        guard let value = SignedUsdFormatter.parse(raw) else { return "unavailable" }
        if SignedUsdFormatter.isZero(raw) { return "no change" }
        let magnitude = value < 0 ? -value : value
        let direction = value < 0 ? "down" : "up"
        return "\(direction) \(spokenAmount(magnitude))"
    }

    static func spokenAmount(_ magnitude: Decimal) -> String {
        var cents = Decimal()
        var scaled = magnitude * 100
        NSDecimalRound(&cents, &scaled, 0, .plain)
        let totalCents = (cents as NSDecimalNumber).int64Value
        let dollars = totalCents / 100
        let remainder = totalCents % 100
        var parts: [String] = []
        if dollars > 0 || remainder == 0 {
            parts.append("\(dollars) \(dollars == 1 ? "dollar" : "dollars")")
        }
        if remainder > 0 {
            parts.append("\(remainder) \(remainder == 1 ? "cent" : "cents")")
        }
        return parts.joined(separator: " ")
    }

    static func percentValue(_ formatted: String) -> Double? {
        let normalised =
            formatted
            .replacingOccurrences(of: "\u{2212}", with: "-")
            .replacingOccurrences(of: "%", with: "")
            .replacingOccurrences(of: "+", with: "")
            .replacingOccurrences(of: ",", with: "")
        return Double(normalised)
    }

    static func percent(_ formatted: String) -> String {
        guard let value = percentValue(formatted) else { return "unavailable" }
        if value == 0 { return "0 percent" }
        let magnitude =
            formatted
            .replacingOccurrences(of: "\u{2212}", with: "")
            .replacingOccurrences(of: "+", with: "")
            .replacingOccurrences(of: "%", with: "")
        return "\(value < 0 ? "down" : "up") \(magnitude) percent"
    }

    static func badge(dollarPnl: String, percentReturn: String?) -> String {
        let dollars = Self.dollars(dollarPnl)
        let formatted = PercentReturnFormatter.format(percentReturn)
        guard formatted != "—", percentValue(formatted) != nil else { return dollars }
        let magnitude =
            formatted
            .replacingOccurrences(of: "\u{2212}", with: "")
            .replacingOccurrences(of: "+", with: "")
            .replacingOccurrences(of: "%", with: "")
        return "\(dollars), \(magnitude) percent"
    }
}
