import MonacoAPI
import MonacoCore
import SwiftUI

enum HomePendingVotesSlot: HomeSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        HomePendingVotesLive()
    }
}

private struct HomePendingVotesLive: View {
    @Environment(AppEnvironment.self) private var environment
    @State private var model: PendingVotesModel?

    var body: some View {
        Group {
            if let model {
                HomePendingVotesSection(model: model) { route in
                    environment.navigator.open(route, in: environment.navigator.selectedTab)
                }
            } else {
                HomePendingVotesSection.loading
            }
        }
        .task {
            guard model == nil else { return }
            model = PendingVotesModel(
                repository: ProposalsRepository(api: environment.api), limit: PendingVotesModel.homeLimit)
        }
    }
}

struct HomePendingVotesSection: View {
    static let title = "Needs your vote"

    let model: PendingVotesModel
    let open: (any AppRoute) -> Void

    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?

    static var loading: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            MonacoSectionHeader(title)
            ProposalCardSkeleton()
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
    }

    var body: some View {
        content
            .task {
                refresh?.register("pending-votes") { await model.load() }
                await model.load()
            }
            .proposalToasts(model.voting)
            .accessibilityIdentifier("home-pending-votes")
    }

    @ViewBuilder
    private var content: some View {
        switch model.state {
        case .idle, .loading:
            Self.loading
        case .failed:
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                MonacoSectionHeader(Self.title)
                ProposalLoadFailedRow(message: "Couldn't load votes.") {
                    Task { await model.load() }
                }
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
        case .loaded(let section) where section.count == 0:
            EmptyView()
        case .loaded(let section):
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                MonacoSectionHeader(Self.title, count: section.count, trailing: "See all") {
                    open(PendingVotesRoute())
                }
                .accessibilityIdentifier("home-pending-votes-header")
                ProposalCardStack(cards: section.cards, voting: model.voting) { choice, card in
                    Task { await model.vote(choice, on: card) }
                }
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
        }
    }
}
