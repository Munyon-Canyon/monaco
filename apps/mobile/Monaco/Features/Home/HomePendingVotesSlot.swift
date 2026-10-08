import MonacoCore
import SwiftUI

enum HomePendingVotesSlot: HomeSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        HomePendingVotes()
    }
}

struct HomePendingVotes: View {
    @Environment(AppEnvironment.self) private var environment
    @State private var model: PendingVotesModel?
    @State private var voting: ProposalVoteModel?
    private let makeModel: @MainActor (AppEnvironment) -> PendingVotesModel

    init(makeModel: @escaping @MainActor (AppEnvironment) -> PendingVotesModel = HomePendingVotes.liveModel) {
        self.makeModel = makeModel
    }

    var body: some View {
        VStack(spacing: 0) {
            if let model, !model.votes.isEmpty {
                let needsVote = model.needsVote
                let inProgress = model.inProgress
                VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                    if !needsVote.isEmpty {
                        section("Needs your vote", needsVote, showsSeeAll: true, model: model)
                    }
                    if !inProgress.isEmpty {
                        section("In progress", inProgress, showsSeeAll: needsVote.isEmpty, model: model)
                    }
                }
                .padding(.horizontal, MonacoTheme.Space.gutter)
            }
        }
        .task { await preparedModel().load() }
    }

    @ViewBuilder private func section(
        _ title: String, _ votes: [PendingVote], showsSeeAll: Bool, model: PendingVotesModel
    ) -> some View {
        HStack {
            MonacoSectionHeader(title, count: votes.count)
            Spacer()
            if showsSeeAll {
                NavigationLink("See all", value: AnyAppRoute(PendingVotesRoute()))
            }
        }
        ForEach(votes.prefix(3)) { vote in
            if let detail = model.details[vote.id], let voting {
                ProposalVoteCard(
                    proposal: detail.summary, voting: voting,
                    paused: model.pausedCabals.contains(detail.summary.cabalID),
                    onVoted: { await model.load(keeping: voting.votedIDs) })
            }
        }
    }

    private func preparedModel() -> PendingVotesModel {
        if let model { return model }
        let created = makeModel(environment)
        voting = ProposalVoteModel(repository: ProposalsRepository(api: environment.api))
        model = created
        return created
    }

    static func liveModel(_ environment: AppEnvironment) -> PendingVotesModel {
        PendingVotesModel(repository: ProposalsRepository(api: environment.api), hints: environment.hints)
    }
}
