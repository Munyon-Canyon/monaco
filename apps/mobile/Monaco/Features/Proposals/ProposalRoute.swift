import MonacoAnalytics
import SwiftUI

nonisolated struct ProposalRoute: AppRoute {
    let proposalID: String

    @MainActor func destination() -> some View {
        ProposalScreen(proposalID: proposalID)
            .analyticsScreen("proposal", step: .vote(.proposalViewed))
    }
}
