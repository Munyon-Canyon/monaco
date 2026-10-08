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
        content
            .navigationTitle("Needs your vote")
            .task { await preparedModel().load() }
    }

    @ViewBuilder private var content: some View {
        switch model?.phase ?? .loading {
        case .loading:
            BoardRowSkeleton(rows: 3)
                .accessibilityElement(children: .ignore)
                .accessibilityLabel("Loading open votes")
        case .failed:
            EmptyState(title: "Couldn't load your votes.", actionTitle: "Try again") {
                Task { await model?.load() }
            }
            .accessibilityIdentifier("pending-votes-error")
        case .loaded:
            if let model, !model.votes.isEmpty {
                ScrollView {
                    VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                        section(nil, model.needsVote, model: model)
                        section("In progress", model.inProgress, model: model)
                    }
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                    .padding(.vertical, MonacoTheme.Space.m)
                }
            } else {
                EmptyState(title: "No open votes", message: "Propose the first buy.")
                    .accessibilityIdentifier("pending-votes-empty")
            }
        }
    }

    @ViewBuilder private func section(_ title: String?, _ votes: [PendingVote], model: PendingVotesModel) -> some View {
        if !votes.isEmpty {
            if let title { MonacoSectionHeader(title, count: votes.count) }
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
