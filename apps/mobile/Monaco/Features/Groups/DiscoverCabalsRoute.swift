import SwiftUI

nonisolated struct DiscoverCabalsRoute: AppRoute {
    var query = ""

    @MainActor func destination() -> some View {
        DiscoverCabalsView(query: query)
    }
}
