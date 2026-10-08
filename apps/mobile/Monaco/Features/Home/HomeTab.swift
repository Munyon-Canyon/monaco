import MonacoCore
import SwiftUI

enum HomeTab: TabContent {
    static let title = "Home"
    static let systemImage = "house"
    static let accessibilityIdentifier = "tab-home"

    static func root() -> some View {
        HomeTabRoot()
    }
}

private struct HomeTabRoot: View {
    @State private var refresh = ScreenRefresh()

    var body: some View {
        HomeScreen()
            .environment(refresh)
            .refreshable { await refresh.run() }
            .monacoFrameStats("Home")
    }
}
