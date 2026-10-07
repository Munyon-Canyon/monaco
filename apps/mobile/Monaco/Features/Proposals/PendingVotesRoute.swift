import MonacoCore
import SwiftUI

nonisolated struct PendingVotesRoute: AppRoute {
    @MainActor func destination() -> some View { PendingVotesScreen() }
}

private struct PendingVotesScreen: View {
    @Environment(AppEnvironment.self) private var environment
    @State private var model: PendingVotesModel?
    @State private var voting: ProposalVoteModel?

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                if let model {
                    section("Needs your vote", model.needsVote, model: model)
                    section("In progress", model.inProgress, model: model)
                }
            }
            .padding(MonacoTheme.Space.m)
        }
        .navigationTitle("Needs your vote")
        .task { await preparedModel().load() }
    }

    @ViewBuilder private func section(_ title: String, _ votes: [PendingVote], model: PendingVotesModel) -> some View {
        if !votes.isEmpty {
            MonacoSectionHeader(title, count: votes.count)
            ForEach(votes) { vote in
                if let detail = model.details[vote.id], let voting {
                    ProposalVoteCard(
                        proposal: detail.summary, voting: voting,
                        paused: model.pausedCabals.contains(detail.summary.cabalID),
                        onVoted: { await model.load() })
                }
            }
        }
    }

    private func preparedModel() -> PendingVotesModel {
        if let model { return model }
        let created = PendingVotesModel(repository: ProposalsRepository(api: environment.api), hints: environment.hints)
        voting = ProposalVoteModel(repository: ProposalsRepository(api: environment.api))
        model = created
        return created
    }
}
