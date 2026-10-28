import MonacoCore
import SwiftUI

enum CabalsTab: TabContent {
    static let title = "Cabals"
    static let systemImage = "person.2"
    static let accessibilityIdentifier = "tab-cabals"

    static func root() -> some View {
        CabalsTabRoot()
    }
}

private struct CabalsTabRoot: View {
    @State private var refresh = ScreenRefresh()

    var body: some View {
        CabalsTabScreen()
            .environment(refresh)
            .refreshable { await refresh.run() }
            .monacoCanvas()
            .navigationBarTitleDisplayMode(.inline)
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("cabals-root")
            .monacoFrameStats("Cabals")
    }
}
