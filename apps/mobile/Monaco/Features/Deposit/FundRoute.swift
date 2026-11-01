import SwiftUI

nonisolated struct FundRoute: AppRoute {
    let cabalID: String
    var cabalName: String?

    @MainActor func destination() -> some View {
        FundCabalScreen(cabalID: cabalID, cabalName: cabalName)
    }
}
