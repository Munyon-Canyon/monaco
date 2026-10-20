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
        VStack(spacing: MonacoTheme.Space.s) {
            layout {
                Button(AssetDetailBuyCTA.title(tradable: detail.isTradable)) { propose(.buy) }
                    .buttonStyle(.monacoPrimary)
                    .disabled(!AssetDetailBuyCTA.canBuy(tradable: detail.isTradable, quotable: detail.isQuotable))
                    .accessibilityIdentifier("asset-detail-propose-buy")
                if canSell {
                    Button("Propose a sell") { propose(.sell) }
                        .buttonStyle(.monacoOutline)
                        .accessibilityIdentifier("asset-detail-propose-sell")
                }
            }
            Text(
                AssetDetailBuyCTA.caption(
                    tradable: detail.isTradable, quotable: detail.isQuotable, canSell: canSell)
            )
            .font(MonacoTheme.Typo.caption)
            .foregroundStyle(MonacoTheme.secondaryText)
            .multilineTextAlignment(.center)
            .frame(maxWidth: .infinity)
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
    }
}

enum AssetDetailBuyCTA {
    static func title(tradable: Bool) -> String {
        tradable ? "Propose a buy" : "Unavailable"
    }

    static func canBuy(tradable: Bool, quotable: Bool) -> Bool { tradable && quotable }

    static func caption(tradable: Bool, quotable: Bool = true, canSell: Bool) -> String {
        if !tradable { return "Not available to buy yet." }
        if !quotable { return "Trading paused right now" }
        if canSell { return "Your cabal votes before anything is bought or sold" }
        return "Your cabal votes before anything is bought"
    }

    static func open(
        symbol: String, kind: ProposeKind, tab: MainTab, navigator: (ProposeFromAssetRoute, MainTab) -> Void
    ) {
        navigator(ProposeFromAssetRoute(symbol: symbol, kind: kind), tab)
    }
}
