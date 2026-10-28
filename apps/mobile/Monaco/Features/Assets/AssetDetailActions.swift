import MonacoCore
import SwiftUI

struct AssetDetailActions: View {
    let detail: AssetDetailPresentation
    let canSell: Bool
    let propose: (ProposeKind) -> Void

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        let layout =
            dynamicTypeSize.isAccessibilitySize
            ? AnyLayout(VStackLayout(spacing: MonacoTheme.Space.sm))
            : AnyLayout(HStackLayout(spacing: MonacoTheme.Space.sm))
        layout {
            Button {
                propose(.buy)
            } label: {
                Label(AssetDetailBuyCTA.title(tradable: detail.isTradable), systemImage: "arrow.up.right")
            }
            .buttonStyle(.monacoPrimary)
            .disabled(!AssetDetailBuyCTA.canBuy(tradable: detail.isTradable, quotable: detail.isQuotable))
            .accessibilityIdentifier("asset-detail-propose-buy")
            if canSell {
                Button {
                    propose(.sell)
                } label: {
                    Label(AssetDetailBuyCTA.sellTitle, systemImage: "arrow.down.right")
                }
                .buttonStyle(.monacoSecondary)
                .accessibilityIdentifier("asset-detail-propose-sell")
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
    }
}

enum AssetDetailBuyCTA {
    static let sellTitle = "Propose sell"

    static func title(tradable: Bool) -> String {
        tradable ? "Propose buy" : "Unavailable"
    }

    static func canBuy(tradable: Bool, quotable: Bool) -> Bool { tradable && quotable }

    static func open(
        symbol: String, kind: ProposeKind, tab: MainTab, navigator: (ProposeFromAssetRoute, MainTab) -> Void
    ) {
        navigator(ProposeFromAssetRoute(symbol: symbol, kind: kind), tab)
    }
}
