import MonacoCore
import SwiftUI

nonisolated struct ProposalContext: Sendable, Hashable {
    let proposalID: String
}

extension EnvironmentValues {
    @Entry var proposalDetail: ProposalDetailModel? = nil
    @Entry var presentedProposalVotes: PresentationDetent? = nil
}

@MainActor
final class ProposalDetailBox {
    private var model: ProposalDetailModel?

    func model(proposalID: String, environment: AppEnvironment) -> ProposalDetailModel {
        if let model { return model }
        let created = ProposalDetailModel(
            id: proposalID, cabalID: "", repository: ProposalsRepository(api: environment.api),
            hints: environment.hints)
        model = created
        return created
    }
}
