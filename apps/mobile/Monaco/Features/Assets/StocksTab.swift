import SwiftUI

enum StocksTab: TabContent {
    static let title = "Stocks"
    static let systemImage = "chart.line.uptrend.xyaxis"
    static let accessibilityIdentifier = "tab-assets"

    static func root() -> some View {
        NotMigratedView(screen: "Stocks")
    }
}
