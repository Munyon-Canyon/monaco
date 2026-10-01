import SwiftUI

enum ProfileTab: TabContent {
    static let title = "Profile"
    static let systemImage = "person.crop.circle"
    static let accessibilityIdentifier = "tab-profile"

    static func root() -> some View {
        NotMigratedView(screen: "Profile")
    }
}
