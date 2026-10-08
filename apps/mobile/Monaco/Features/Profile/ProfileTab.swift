import MonacoCore
import SwiftUI

enum ProfileTab: TabContent {
    static let title = "Profile"
    static let systemImage = "person.crop.circle"
    static let accessibilityIdentifier = "tab-profile"

    static func root() -> some View {
        ProfileTabRoot()
    }
}

private struct ProfileTabRoot: View {
    @State private var refresh = ScreenRefresh()

    var body: some View {
        ProfileScreen()
            .environment(refresh)
            .refreshable { await refresh.run() }
            .monacoTopLevelHeader(title: ProfileTab.title)
    }
}
