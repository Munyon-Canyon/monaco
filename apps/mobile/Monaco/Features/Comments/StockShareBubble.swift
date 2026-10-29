import MonacoAPI
import MonacoCore
import SwiftUI

struct StockShareBubble: View {
    let commentID: String
    let stock: Components.Schemas.StockAttachment

    @Environment(AppEnvironment.self) private var environment: AppEnvironment?

    private static let minTap: CGFloat = 44

    private var price: String { stock.priceMicros.map { UsdAmountFormatter.format(micros: $0) } ?? "—" }

    var body: some View {
        Button(action: open) {
            HStack(spacing: MonacoTheme.Space.sm) {
                StockMark(
                    symbol: stock.symbol, displayName: stock.name, assetKind: stock.isPreIpo ? .preIpo : .stock,
                    size: .row, logoURL: URL(string: stock.logoUrl))
                VStack(alignment: .leading, spacing: 0) {
                    Text(stock.name)
                        .font(MonacoTheme.Typo.subheadStrong)
                        .foregroundStyle(MonacoTheme.primaryText)
                        .lineLimit(1)
                    Text(stock.symbol)
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.secondaryText)
                }
                Spacer(minLength: MonacoTheme.Space.s)
                VStack(alignment: .trailing, spacing: 0) {
                    Text(price)
                        .moneyFigure(.row)
                        .foregroundStyle(MonacoTheme.ink)
                    if !stock.isPreIpo, let bps = stock.changeBps {
                        Text(PercentFormatter.format(basisPoints: Int64(bps), signed: true))
                            .font(MonacoTheme.Typo.caption)
                            .foregroundStyle(bps < 0 ? MonacoTheme.loss : MonacoTheme.profit)
                    }
                }
            }
            .padding(MonacoTheme.Space.s)
            .frame(maxWidth: .infinity, minHeight: Self.minTap, alignment: .leading)
            .background(
                RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous)
                    .fill(MonacoTheme.surfaceSunken)
            )
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityElement(children: .combine)
        .accessibilityHint("Opens the stock")
        .accessibilityIdentifier("attachment-stock-\(commentID)")
    }

    private func open() {
        guard let environment else { return }
        environment.navigator.open(AssetRoute(symbol: stock.symbol), in: environment.navigator.selectedTab)
    }
}
