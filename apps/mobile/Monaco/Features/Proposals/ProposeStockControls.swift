import MonacoCore
import SwiftUI

enum ProposeSort: CaseIterable, Hashable {
    case volume, name, change

    var title: String {
        switch self {
        case .volume: ProposeFlowCopy.sortVolume
        case .name: ProposeFlowCopy.sortName
        case .change: ProposeFlowCopy.sortChange
        }
    }

    func apply(_ assets: [MarketAsset]) -> [MarketAsset] {
        switch self {
        case .volume: assets
        case .name: assets.sorted { $0.name.localizedCaseInsensitiveCompare($1.name) == .orderedAscending }
        case .change: assets.sorted { abs($0.changeBasisPoints ?? 0) > abs($1.changeBasisPoints ?? 0) }
        }
    }

    func apply(_ holdings: [ProposeHolding]) -> [ProposeHolding] {
        switch self {
        case .volume, .change: holdings.sorted { $0.valueMicros > $1.valueMicros }
        case .name: holdings.sorted { $0.name.localizedCaseInsensitiveCompare($1.name) == .orderedAscending }
        }
    }
}

struct ProposeStockControls: View {
    @Binding var sort: ProposeSort
    @Binding var filter: StocksTabModel.Browse
    var sorts: [ProposeSort] = ProposeSort.allCases
    var filters: [StocksTabModel.Browse] = StocksTabModel.Browse.allCases

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        let layout =
            dynamicTypeSize.isAccessibilitySize
            ? AnyLayout(VStackLayout(spacing: MonacoTheme.Space.s))
            : AnyLayout(HStackLayout(spacing: MonacoTheme.Space.s))
        layout {
            MonacoMenuSelect(title: "Sort stocks", selection: $sort, options: sorts, label: \.title)
                .fillsWidth()
                .accessibilityIdentifier("propose-select-sort")
            MonacoMenuSelect(title: "Filter stocks", selection: $filter, options: filters, label: \.title)
                .fillsWidth()
                .accessibilityIdentifier("propose-select-filter")
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
    }
}
