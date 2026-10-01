import SwiftUI

enum CabalsTab: TabContent {
    static let title = "Cabals"
    static let systemImage = "person.3"
    static let accessibilityIdentifier = "tab-cabals"

    static func root() -> some View {
        NotMigratedView(screen: "Cabals")
    }
}
