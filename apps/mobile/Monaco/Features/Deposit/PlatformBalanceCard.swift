import MonacoAPI
import MonacoCore
import SwiftUI

/// The account balance as a line in the ledger: the cash coin, the label, the figure, and —
/// while a fund is on its way into a cabal — how much of it is, under the label.
///
/// It used to be a white card with the figure at 28pt, which made the balance the loudest
/// thing on a money screen whose job is somewhere else (an address to copy, an amount to
/// type). It is the row Home draws (`HomeBalanceRowSection`) now, so the balance reads the
/// same wherever it appears. The type keeps the file's name; it is no longer a card.
///
/// Put it inside a `MonacoGroupedList`, which draws the rules.
struct PlatformBalanceCard: View {
    let display: HomeBalanceDisplay
    var pendingAllocationMicros: Int64 = 0
    var cardProcessing = false
    /// The figure's identifier. Each screen names its own, so a test can tell them apart.
    var valueIdentifier = "platform-balance-value"

    static let coinSize: CGFloat = 44
    static let contentSpacing = MonacoTheme.Space.sm
    static let leadingInset = MonacoTheme.Space.gutter + coinSize + contentSpacing

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    init(
        display: HomeBalanceDisplay, pendingAllocationMicros: Int64 = 0, cardProcessing: Bool = false,
        valueIdentifier: String = "platform-balance-value"
    ) {
        self.display = display
        self.pendingAllocationMicros = pendingAllocationMicros
        self.cardProcessing = cardProcessing
        self.valueIdentifier = valueIdentifier
    }

    init(
        state: LoadState<AccountBalance>, cardProcessing: Bool = false,
        valueIdentifier: String = "platform-balance-value"
    ) {
        var inFlightMicros: Int64 = 0
        if case .loaded(let balance) = state { inFlightMicros = balance.inFlightMicros }
        self.init(
            display: .resolve(state), pendingAllocationMicros: inFlightMicros, cardProcessing: cardProcessing,
            valueIdentifier: valueIdentifier)
    }

    /// "$50.00 funding a cabal", or nil when nothing is on its way.
    static func pendingLine(micros: Int64) -> String? {
        guard micros > 0 else { return nil }
        return "\(UsdAmountFormatter.format(micros: micros)) funding a cabal"
    }

    static func statusLine(cardProcessing: Bool, pendingMicros: Int64) -> String? {
        cardProcessing ? CardDeposit.processingLine : pendingLine(micros: pendingMicros)
    }

    /// At the accessibility sizes the figure drops under the label, the way `MonacoRow` stacks,
    /// so neither the label nor the money is cut.
    private var isStacked: Bool { dynamicTypeSize.isAccessibilitySize }

    var body: some View {
        if display == .loading {
            MonacoRowSkeleton(rows: 1, markShape: .circle)
                .accessibilityElement()
                .accessibilityLabel("Loading your account balance")
                .accessibilityIdentifier("platform-balance-loading")
        } else {
            row
        }
    }

    private var row: some View {
        Group {
            if isStacked {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                    HStack(spacing: Self.contentSpacing) {
                        coin
                        labels
                    }
                    figure
                }
            } else {
                HStack(alignment: .center, spacing: Self.contentSpacing) {
                    coin
                    labels
                    figure
                        .layoutPriority(1)
                }
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.vertical, MonacoTheme.Space.s)
        .frame(minHeight: Self.coinSize + 2 * MonacoTheme.Space.s)
        .accessibilityElement(children: .combine)
    }

    private var coin: some View {
        StockMark(symbol: "USDC")
            .frame(width: Self.coinSize, height: Self.coinSize)
    }

    private var labels: some View {
        VStack(alignment: .leading, spacing: 0) {
            Text("Account balance")
                .font(MonacoTheme.Typo.callout)
                .foregroundStyle(MonacoTheme.muted)
            if let status = Self.statusLine(cardProcessing: cardProcessing, pendingMicros: pendingAllocationMicros) {
                Text(status)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
                    .accessibilityIdentifier("platform-balance-status")
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    @ViewBuilder
    private var figure: some View {
        if case .amount(let micros) = display {
            MoneyText(micros: UsdAmountFormatter.flooredToCents(micros), style: .row)
                .accessibilityIdentifier(valueIdentifier)
        } else {
            // A dash, not a figure: a balance that could not be read is not an empty account.
            Text("—")
                .moneyFont(.row)
                .foregroundStyle(MonacoTheme.muted)
                .accessibilityLabel("Account balance unavailable")
                .accessibilityIdentifier("platform-balance-unavailable")
        }
    }
}
