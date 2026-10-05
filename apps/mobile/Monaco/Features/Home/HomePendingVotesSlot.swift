import MonacoCore
import SwiftUI

enum HomePendingVotesSlot: HomeSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        HomePendingVotes()
    }
}

private struct HomePendingVotes: View {
    @Environment(AppEnvironment.self) private var environment
    @State private var model: PendingVotesModel?

    var body: some View {
        Group {
            if let model, !model.votes.isEmpty {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                    HStack {
                        MonacoSectionHeader("Needs your vote", count: model.votes.count)
                        Spacer()
                        NavigationLink("See all", value: PendingVotesRoute())
                    }
                    ForEach(model.votes.prefix(3)) { vote in
                        if let detail = model.details[vote.id] {
                            NavigationLink(value: AnyAppRoute(ProposalRoute(proposalID: vote.id))) {
                                ProposalCard(
                                    proposal: detail.summary, asset: nil, members: [],
                                    paused: model.pausedCabals.contains(detail.summary.cabalID))
                            }
                            .buttonStyle(.plain)
                        }
                    }
                }
            }
        }
        .task { await preparedModel().load() }
    }

    private func preparedModel() -> PendingVotesModel {
        if let model { return model }
        let created = PendingVotesModel(repository: ProposalsRepository(api: environment.api), hints: environment.hints)
        model = created
        return created
    }
}
