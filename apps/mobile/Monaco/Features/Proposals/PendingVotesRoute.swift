import MonacoCore
import SwiftUI

nonisolated struct PendingVotesRoute: AppRoute {
    @MainActor func destination() -> some View { PendingVotesScreen() }
}

private struct PendingVotesScreen: View {
    @Environment(AppEnvironment.self) private var environment
    @State private var model: PendingVotesModel?

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                if let model {
                    ForEach(model.votes) { vote in
                        if let detail = model.details[vote.id] {
                            NavigationLink(value: AnyAppRoute(ProposalRoute(proposalID: vote.id))) {
                                ProposalCard(proposal: detail.summary, asset: nil, members: [])
                            }
                            .buttonStyle(.plain)
                        }
                    }
                }
            }
            .padding(MonacoTheme.Space.m)
        }
        .navigationTitle("Needs your vote")
        .task { await preparedModel().load() }
    }

    private func preparedModel() -> PendingVotesModel {
        if let model { return model }
        let created = PendingVotesModel(repository: ProposalsRepository(api: environment.api), hints: environment.hints)
        model = created
        return created
    }
}
