import SwiftUI

enum HomeTab: TabContent {
    static let title = "Home"
    static let systemImage = "house"
    static let accessibilityIdentifier = "tab-home"

    static func root() -> some View {
        NotMigratedView(screen: "Home")
    }
}
