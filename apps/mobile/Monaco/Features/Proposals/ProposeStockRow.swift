import SwiftUI

struct ProposeStockRow: View {
    let stock: ProposeStock
    var logoURL: URL?
    var isLast = false

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @ScaledMetric(relativeTo: .body) private var titleWidthFloor = MonacoRowLayout.baseMinimumTitleWidth

    static let markSize: CGFloat = StockListRow.markSize

    private var layout: MonacoRowLayout {
        MonacoRowLayout(dynamicTypeSize: dynamicTypeSize, scaledTitleWidthFloor: titleWidthFloor)
    }

    private var caption: String? {
        if !stock.isTradable { return "\(stock.name) · Can't buy right now" }
        return stock.name == stock.ticker ? nil : stock.name
    }

    var body: some View {
        content
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.vertical, 8)
            .frame(minHeight: MonacoRowLayout.minHeight)
            .contentShape(Rectangle())
            .overlay(alignment: .bottom) {
                if !isLast {
                    MonacoRule()
                        .padding(.leading, layout.separatorLeadingInset(markSize: Self.markSize))
                }
            }
            .accessibilityElement(children: .combine)
    }

    @ViewBuilder
    private var content: some View {
        if layout.isStacked {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                HStack(spacing: MonacoTheme.Space.sm) {
                    mark
                    labels
                }
                if hasFigures {
                    HStack(spacing: MonacoTheme.Space.s) {
                        figures
                    }
                }
            }
        } else {
            HStack(spacing: MonacoTheme.Space.sm) {
                mark
                labels.frame(minWidth: layout.minimumTitleWidth, alignment: .leading)
                if hasFigures {
                    VStack(alignment: .trailing, spacing: MonacoTheme.Space.xs) {
                        figures
                    }
                    .layoutPriority(1)
                }
            }
        }
    }

    private var mark: some View {
        StockMark(
            symbol: stock.symbol, displayName: stock.name, assetKind: stock.assetKind, size: Self.markSize,
            logoURL: logoURL
        )
        .frame(width: Self.markSize, height: Self.markSize)
        .opacity(stock.isTradable ? 1 : 0.45)
    }

    private var labels: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            Text(stock.ticker)
                .font(MonacoTheme.Typo.ticker)
                .foregroundStyle(stock.isTradable ? MonacoTheme.ink : MonacoTheme.disabledLabel)
                .lineLimit(layout.titleLineLimit)
                .truncationMode(.tail)
            if let caption {
                Text(caption)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
                    .lineLimit(layout.subtitleLineLimit)
                    .truncationMode(.tail)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private var hasFigures: Bool {
        stock.priceMicros != nil
    }

    @ViewBuilder
    private var figures: some View {
        if let micros = stock.priceMicros {
            MoneyText(
                micros: micros,
                style: .row,
                color: stock.isTradable ? MonacoTheme.ink : MonacoTheme.disabledLabel,
                voice: .market
            )
            if stock.isTradable, stock.change24h != nil {
                DayChangePill(change24h: stock.change24h, priceUsdcMicros: micros)
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
                .frame(minHeight: 64)
                .overlay(alignment: .bottom) {
                    if index < rows - 1 {
                        MonacoRule().padding(
                            .leading, MonacoTheme.Space.m + ProposeStockRow.markSize + MonacoTheme.Space.sm)
                    }
                }
            }
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Loading stocks")
    }
}
