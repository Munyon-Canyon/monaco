import SwiftUI

enum StocksTab: TabContent {
    static let title = "Markets"
    static let systemImage = "chart.xyaxis.line"
    static let accessibilityIdentifier = "tab-assets"

    #if DEBUG
    @MainActor static var sampleQuery = ""
    #endif

    static func root() -> some View {
        #if DEBUG
        StocksTabView(query: sampleQuery)
        #else
        StocksTabView()
        #endif
    }
}
