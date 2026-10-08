import MonacoCore
import SwiftUI

enum CabalsTab: TabContent {
    static let title = "Cabals"
    static let systemImage = "person.3"
    static let accessibilityIdentifier = "tab-cabals"

    static func root() -> some View {
        CabalsTabRoot()
    }
}

private struct CabalsTabRoot: View {
    @Environment(\.accountRestricted) private var accountRestricted
    @State private var showsNewCabal = false
    @State private var refresh = ScreenRefresh()

    var body: some View {
        CabalsTabScreen()
            .environment(refresh)
            .refreshable { await refresh.run() }
            .navigationTitle(CabalsTab.title)
            .navigationBarTitleDisplayMode(.large)
            .toolbar {
                if !accountRestricted {
                    ToolbarItem(placement: .topBarTrailing) {
                        Button {
                            showsNewCabal = true
                        } label: {
                            Image(systemName: "plus")
                                .monacoToolbarIcon()
                                .frame(width: 44, height: 44)
                        }
                        .accessibilityLabel("New cabal")
                        .accessibilityIdentifier("cabals-new-button")
                    }
                }
            }
            .newCabalSheet(isPresented: $showsNewCabal)
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("cabals-root")
            .monacoFrameStats("Cabals")
    }
}
