import SwiftUI

struct ProposeStockRow: View {
    let stock: ProposeStock
    var logoURL: URL?
    var isLast = false

    var body: some View {
        MonacoRow(
            title: stock.name, subtitle: stock.name == stock.ticker ? nil : stock.ticker,
            chevron: stock.isTradable, isLast: isLast
        ) {
            StockMark(
                symbol: stock.symbol, displayName: stock.name, assetKind: stock.assetKind,
                size: MonacoRowLayout.baseMarkSize, logoURL: logoURL)
        } trailing: {
            if stock.isPaused { PausedTag() }
        }
        .opacity(stock.isTradable ? 1 : 0.45)
    }
}

struct ProposeStockSkeleton: View {
    var rows = 4

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            SkeletonTitle()
            SkeletonBar(width: nil)
            SkeletonCardBlock()
            MonacoRowSkeleton(rows: rows, markShape: .circle, hasTrailing: false)
                .padding(.horizontal, -MonacoTheme.Space.gutter)
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Loading stocks")
    }
}
