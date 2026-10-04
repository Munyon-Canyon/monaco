import MonacoCore
import SwiftUI

enum CabalProposalsSlot: CabalSection {
    static let isLive = true

    static func body(for context: CabalContext) -> some View {
        CabalProposals(cabalID: context.cabalID)
    }
}

private struct CabalProposals: View {
    let cabalID: String
    @Environment(AppEnvironment.self) private var environment
    @State private var model: ProposalListModel?

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            if let model {
                switch model.pager.phase {
                case .loadingFirst where model.pager.items.isEmpty:
                    SkeletonBlock(width: 280, height: 160, radius: MonacoTheme.Radius.card)
                case .failed where model.pager.items.isEmpty:
                    EmptyState(title: "Couldn't load votes.", actionTitle: "Try again") { Task { await model.load() } }
                default:
                    content(model)
                }
            }
        }
        .task { await preparedModel().load() }
        .onScreenVisibilityChange { model?.setVisible($0) }
    }

    @ViewBuilder private func content(_ model: ProposalListModel) -> some View {
        let needsVote = model.pager.items.filter { $0.canVote && $0.myBallot == nil }
        if model.pager.items.isEmpty {
            EmptyState(title: "No open votes", message: "Propose the first buy.")
        } else {
            HStack {
                MonacoSectionHeader("Needs your vote", count: needsVote.count)
                Spacer()
                NavigationLink("See all", value: CabalProposalListRoute(cabalID: cabalID))
            }
            ForEach(needsVote) { proposal in ProposalCard(proposal: proposal, asset: nil, members: []) }
        }
    }

    private func preparedModel() -> ProposalListModel {
        if let model { return model }
        let created = ProposalListModel(
            cabalID: cabalID, filter: .open, repository: ProposalsRepository(api: environment.api),
            hints: environment.hints)
        model = created
        return created
    }
}

private nonisolated struct CabalProposalListRoute: Hashable, AppRoute {
    let cabalID: String
    @MainActor func destination() -> some View { CabalProposals(cabalID: cabalID) }
}
