import MonacoCore
import SwiftUI

/// How a day-change pill reads. Members use the two very differently: a percent
/// compares one stock against another, a dollar figure answers "what did that do to
/// my money". Both come from figures already on the row, so switching costs nothing.
enum DayChangeMode: String, CaseIterable {
    case percent
    case dollars

    var next: DayChangeMode { self == .percent ? .dollars : .percent }

    /// What VoiceOver offers on the row, naming what a tap would switch *to*.
    var switchActionName: String {
        next == .dollars ? "Show day change in dollars" : "Show day change as a percent"
    }
}

/// The one storage key behind every pill, so the whole app flips together — as it
/// does on Robinhood. A per-row choice would mean a list where some rows are
/// percents and some are dollars, which is unreadable.
enum DayChangeModeStorage {
    static let key = "monaco.stocks.dayChangeMode"
}

/// The tone of a day change, read off the figure that is actually displayed.
///
/// Deliberately not `PnLTone(dollarPnl:)`: that one calls anything under half a cent
/// flat, and a +0.45% day is a rise even though the ratio "0.0045" rounds to $0.00.
extension PnLTone {
    init(change24h: String?) {
        let formatted = PercentReturnFormatter.format(change24h)
        if formatted == "—" || formatted == "0.0%" {
            self = .flat
        } else if formatted.hasPrefix("\u{2212}") {
            self = .loss
        } else {
            self = .profit
        }
    }

    /// The tone of a row's sparkline, which is not always the tone of its pill.
    ///
    /// The line is Pyth's underlying equity and the pill is Jupiter's price for the
    /// xStock token, and those two genuinely diverge. When the row knows they are
    /// different instruments it tints the line from the line, so the colour is
    /// about the picture on screen rather than about a number measured somewhere
    /// else.
    init(sparkTint: SparkTint, change24h: String?) {
        switch sparkTint {
        case .reportedDayChange:
            self.init(change24h: change24h)
        case .series(let rising, let flat):
            self = flat ? .flat : (rising ? .profit : .loss)
        }
    }
}

/// The filled capsule on the right of a market row: "+1.24%", or "+$2.84" once
/// tapped. Tapping toggles every pill in the app.
struct DayChangePill: View {
    let change24h: String?
    let priceUsdcMicros: Int64?
    var style: MoneyStyle = .caption

    @AppStorage(DayChangeModeStorage.key) private var storedMode = DayChangeMode.percent.rawValue

    private var mode: DayChangeMode { DayChangeMode(rawValue: storedMode) ?? .percent }
    private var tone: PnLTone { PnLTone(change24h: change24h) }

    private var percentText: String { DayChangeFigures.percentText(change24h: change24h) }

    /// Dollars when the member asked for them and they can be worked out; the
    /// percent otherwise. A row with a change but no price keeps its percent rather
    /// than blanking when the mode flips.
    private var label: String {
        guard mode == .dollars,
            let dollars = DayChangeFigures.dollarText(change24h: change24h, priceUsdcMicros: priceUsdcMicros)
        else { return percentText }
        return dollars
    }

    private var isReadable: Bool { percentText != "—" }

    /// How far the hit area reaches past the drawn capsule on each side.
    ///
    /// The capsule is about 22pt tall, which is the right size to read and the
    /// wrong size to hit — under the HIG's 44pt minimum, and the UI test that opens
    /// a stock had to aim a quarter of the way into the row to miss it. The padding
    /// is applied for hit testing and then taken straight back off, so the target
    /// grows without the row growing with it.
    private static let tapTargetPadding: CGFloat = 11

    var body: some View {
        Text(label)
            .moneyFont(style, weight: .semibold, voice: .market)
            .foregroundStyle(isReadable ? tone.washColor : MonacoTheme.muted)
            .lineLimit(1)
            .minimumScaleFactor(0.8)
            .padding(.horizontal, style == .caption ? 8 : 12)
            .padding(.vertical, style == .caption ? 4 : 6)
            .background(Capsule().fill(isReadable ? tone.wash : MonacoTheme.surfaceSunken))
            .padding(DayChangePill.tapTargetPadding)
            .contentShape(Rectangle())
            // A plain tap gesture inside a row-sized Button is swallowed by the row.
            // A high-priority one is not, which is what lets the pill be its own
            // control without the row being taken apart into two hit areas.
            .highPriorityGesture(TapGesture().onEnded { toggle() })
            .padding(-DayChangePill.tapTargetPadding)
            .accessibilityLabel("Day change")
            .accessibilityValue(
                DayChangeSpeech.value(change24h: change24h, priceUsdcMicros: priceUsdcMicros, mode: mode))
    }

    private func toggle() {
        guard isReadable else { return }
        Haptics.selection()
        storedMode = mode.next.rawValue
    }
}

/// What VoiceOver reads for a day change.
enum DayChangeSpeech {
    static func value(change24h: String?, priceUsdcMicros: Int64?, mode: DayChangeMode) -> String {
        let percent = PnLSpeech.percent(PercentReturnFormatter.format(change24h))
        guard mode == .dollars,
            let dollars = DayChangeFigures.dollarDelta(change24h: change24h, priceUsdcMicros: priceUsdcMicros)
        else { return percent }
        return PnLSpeech.dollars(dollars)
    }
}

/// The measures a stock row shares with the lists that mirror it, so the separator
/// inset and the second line's limit have one home.
enum StockListRow {
    /// The mark on a market row: `MonacoRow`'s, so the Stocks list lines up with the cabal list.
    static let markSize = MonacoRowLayout.baseMarkSize

    /// Two lines before the second line gives up. Its last words are the member's own
    /// slice ("your slice $77.38"), and a list that truncates a member's money to fit a
    /// row is the wrong way round — the row grows by a line instead.
    static let subtitleLineLimit = 2
}

struct StockRowSkeleton: View {
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var rows: Int = 6

    var body: some View {
        VStack(spacing: 0) {
            ForEach(0..<rows, id: \.self) { index in
                HStack(spacing: MonacoTheme.Space.sm) {
                    SkeletonBlock(
                        width: StockListRow.markSize, height: StockListRow.markSize,
                        radius: StockListRow.markSize / 2)
                    VStack(alignment: .leading, spacing: 6) {
                        SkeletonBlock(width: 120, height: 14)
                        SkeletonBlock(width: 48, height: 11)
                    }
                    Spacer(minLength: MonacoTheme.Space.s)
                    VStack(alignment: .trailing, spacing: 6) {
                        SkeletonBlock(width: 64, height: 14)
                        SkeletonBlock(width: 56, height: 22, radius: 11)
                    }
                }
                .padding(.horizontal, MonacoTheme.Space.m)
                .padding(.vertical, 8)
                .frame(minHeight: MonacoRowLayout.minHeight)
                .overlay(alignment: .bottom) {
                    if index < rows - 1 {
                        MonacoRule().padding(
                            .leading,
                            MonacoRowLayout(dynamicTypeSize: dynamicTypeSize)
                                .separatorLeadingInset(markSize: StockListRow.markSize))
                    }
                }
            }
        }
        .overlay(alignment: .top) { MonacoRule() }
        .overlay(alignment: .bottom) { MonacoRule() }
        .accessibilityHidden(true)
    }
}
