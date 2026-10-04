import MonacoAPI
import MonacoCore
import SwiftUI

nonisolated struct PendingVotesRoute: AppRoute {
    @MainActor func destination() -> some View {
        PendingVotesLive()
    }
}

private struct PendingVotesLive: View {
    @Environment(AppEnvironment.self) private var environment
    @State private var model: PendingVotesModel?

    var body: some View {
        Group {
            if let model {
                PendingVotesScreen(model: model)
            } else {
                Color.clear
            }
        }
        .task {
            guard model == nil else { return }
            model = PendingVotesModel(repository: ProposalsRepository(api: environment.api), limit: nil)
        }
    }
}

struct PendingVotesScreen: View {
    let model: PendingVotesModel

    var body: some View {
        ScrollView {
            content
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .padding(.vertical, MonacoTheme.Space.m)
        }
        .monacoCanvas()
        .navigationTitle(HomePendingVotesSection.title)
        .navigationBarTitleDisplayMode(.inline)
        .refreshable { await model.load() }
        .task { await model.load() }
        .proposalToasts(model.voting)
    }

    @ViewBuilder
    private var content: some View {
        switch model.state {
        case .idle, .loading:
            ProposalCardSkeleton()
        case .failed:
            ProposalLoadFailedRow(message: "Couldn't load votes.") {
                Task { await model.load() }
            }
        case .loaded(let section) where section.cards.isEmpty:
            EmptyState(title: "Nothing needs your vote")
                .frame(maxWidth: .infinity)
                .padding(.top, MonacoTheme.Space.l)
        case .loaded(let section):
            ProposalCardStack(cards: section.cards, voting: model.voting) { choice, card in
                Task { await model.vote(choice, on: card) }
            }
        }
    }
}
