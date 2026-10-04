import SwiftUI

enum FeedTab: TabContent {
    static let title = "Feed"
    static let systemImage = "newspaper"
    static let accessibilityIdentifier = "tab-feed"

    static func root() -> some View {
        FeedView()
    }
}
