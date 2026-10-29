import MonacoAnalytics
import MonacoCore
import SwiftUI

nonisolated struct ProposalRoute: AppRoute {
    let proposalID: String
    var focus: ProposalFocus = .none

    @MainActor func destination() -> some View {
        ProposalScreen(proposalID: proposalID)
            .environment(\.proposalFocus, focus)
            .analyticsScreen("proposal", step: .vote(.proposalViewed))
    }
}
