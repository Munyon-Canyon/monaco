import SwiftUI

struct ProposeStockRow: View {
    let stock: ProposeStock
    var logoURL: URL?
    var isLast = false

    static let markSize: CGFloat = StockListRow.markSize

    var body: some View {
        MarketStockRow(
            name: stock.name,
            subtitle: stock.name == stock.ticker ? nil : stock.ticker,
            mark: StockMark(
                symbol: stock.symbol, displayName: stock.name, assetKind: stock.assetKind, logoURL: logoURL),
            isAvailable: stock.isTradable,
            isLast: isLast
        ) {
            if stock.isPaused { PausedTag() }
            if let micros = stock.priceMicros {
                MoneyText(micros: micros, style: .row)
                if stock.isTradable, stock.change24h != nil {
                    DayChangePill(change24h: stock.change24h, priceUsdcMicros: micros)
                }
            }
        }
    }
}

struct ProposeStockSkeleton: View {
    var rows = 4

    var body: some View {
        MonacoGroupedList {
            ForEach(0..<rows, id: \.self) { index in
                HStack(spacing: MonacoTheme.Space.sm) {
                    SkeletonBlock(
                        width: ProposeStockRow.markSize, height: ProposeStockRow.markSize,
                        radius: ProposeStockRow.markSize / 2)
                    VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                        SkeletonBlock(width: 64, height: 14)
                        SkeletonBlock(width: 96, height: 12)
                    }
                    Spacer(minLength: MonacoTheme.Space.s)
                    VStack(alignment: .trailing, spacing: MonacoTheme.Space.s) {
                        SkeletonBlock(width: 72, height: 14)
                        SkeletonBlock(width: 52, height: 20, radius: 10)
                    }
                }
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .frame(minHeight: MonacoRowLayout.minHeight)
                .overlay(alignment: .bottom) {
                    if index < rows - 1 {
                        MonacoRule().padding(.leading, StockListRow.textLeading)
                    }
                }
            }
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Loading stocks")
    }
}
