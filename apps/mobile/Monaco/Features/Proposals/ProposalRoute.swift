import SwiftUI

nonisolated struct ProposalRoute: AppRoute {
    let proposalID: String

    @MainActor func destination() -> some View {
        ProposalScreen(proposalID: proposalID)
    }
}
