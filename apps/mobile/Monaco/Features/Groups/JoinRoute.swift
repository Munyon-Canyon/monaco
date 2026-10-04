import SwiftUI

nonisolated struct JoinRoute: AppRoute {
    @MainActor func destination() -> some View {
        JoinCabalView()
    }
}
