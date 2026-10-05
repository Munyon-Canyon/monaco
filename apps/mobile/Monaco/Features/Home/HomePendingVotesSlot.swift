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
                VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                    HStack {
                        MonacoSectionHeader("Needs your vote", count: model.votes.count)
                        Spacer()
                        NavigationLink("See all", value: PendingVotesRoute())
                    }
                    ForEach(model.votes.prefix(3)) { vote in
                        if let detail = model.details[vote.id], let voting {
                            ProposalVoteCard(
                                proposal: detail.summary, voting: voting,
                                paused: model.pausedCabals.contains(detail.summary.cabalID),
                                onVoted: { await model.load() })
                        }
                    }
                }
            }
        }
        .task { await preparedModel().load() }
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
