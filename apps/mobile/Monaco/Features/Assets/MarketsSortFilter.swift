import MonacoCore
import SwiftUI

struct MarketsSortFilterRow: View {
    let browse: MonacoCore.StocksTabModel.Browse
    let select: (MonacoCore.StocksTabModel.Browse) -> Void

    var body: some View {
        ViewThatFits(in: .horizontal) {
            HStack(spacing: MonacoTheme.Space.sm) {
                MarketsSortMenu()
                filter
            }
            VStack(spacing: MonacoTheme.Space.sm) {
                MarketsSortMenu()
                filter
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
    }

    private var filter: some View {
        MonacoMenuSelect(
            title: "Filter stocks",
            selection: Binding(get: { browse }, set: { select($0) }),
            options: MonacoCore.StocksTabModel.Browse.allCases,
            label: \.title
        )
        .fillsWidth()
        .accessibilityIdentifier("stocks-filter")
    }
}

private struct MarketsSortMenu: View {
    private typealias Sort = MonacoCore.StocksTabModel.Sort

    var body: some View {
        Menu {
            Section {
                Button {
                } label: {
                    Label(Sort.standard.title, systemImage: "checkmark")
                }
            }
            Section(Sort.unavailableNote) {
                ForEach(Sort.allCases.filter { !$0.isAvailable }, id: \.self) { sort in
                    Button(sort.title) {}.disabled(true)
                }
            }
        } label: {
            HStack(spacing: MonacoTheme.Space.s) {
                Text(Sort.standard.title)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.primaryText)
                    .lineLimit(1)
                Spacer(minLength: 0)
                Image(systemName: "chevron.down")
                    .font(MonacoTheme.Typo.captionStrong)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .accessibilityHidden(true)
            }
            .padding(.horizontal, MonacoTheme.Space.sm)
            .frame(
                minWidth: MonacoMenuSelectMetrics.minWidth, maxWidth: .infinity,
                minHeight: MonacoMenuSelectMetrics.minHeight
            )
            .background(Capsule().fill(MonacoTheme.surface))
            .overlay(Capsule().strokeBorder(MonacoTheme.border, lineWidth: MonacoMenuSelectMetrics.borderWidth))
            .contentShape(Capsule())
        }
        .accessibilityLabel("Sort stocks, \(Sort.standard.title)")
        .accessibilityIdentifier("stocks-sort")
    }
}
