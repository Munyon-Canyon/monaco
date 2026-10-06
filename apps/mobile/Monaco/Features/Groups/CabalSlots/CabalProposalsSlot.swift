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
    @State private var closed: ProposalListModel?
    @State private var pause: ProposalPauseModel?
    @State private var voting: ProposalVoteModel?

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
        .task {
            await preparedModel().load()
            await closed?.load()
        }
        .task {
            let pause = preparedPause()
            await pause.load()
            await pause.observe()
        }
        .onScreenVisibilityChange {
            model?.setVisible($0)
            pause?.setVisible($0)
        }
    }

    @ViewBuilder private func content(_ model: ProposalListModel) -> some View {
        let needsVote = model.pager.items.filter {
            $0.canVote && ($0.myBallot == nil || voting?.ballots[$0.id] != nil)
        }
        if model.pager.items.isEmpty {
            EmptyState(title: "No open votes", message: "Propose the first buy.")
            if closed?.pager.items.isEmpty == false {
                NavigationLink("See all", value: AnyAppRoute(CabalProposalListRoute(cabalID: cabalID)))
            }
        } else {
            HStack {
                MonacoSectionHeader("Needs your vote", count: needsVote.count)
                Spacer()
                NavigationLink("See all", value: AnyAppRoute(CabalProposalListRoute(cabalID: cabalID)))
            }
            ForEach(needsVote) { proposal in
                if let voting {
                    ProposalVoteCard(
                        proposal: proposal, voting: voting, paused: pause?.isPaused == true,
                        onVoted: { await model.pager.refreshFirstPage() })
                }
            }
        }
    }

    private func preparedPause() -> ProposalPauseModel {
        if let pause { return pause }
        let created = ProposalPauseModel(
            cabalID: cabalID, repository: ProposalsRepository(api: environment.api), hints: environment.hints)
        pause = created
        return created
    }

    private func preparedModel() -> ProposalListModel {
        if let model { return model }
        voting = ProposalVoteModel(repository: ProposalsRepository(api: environment.api))
        let created = ProposalListModel(
            cabalID: cabalID, filter: .open, repository: ProposalsRepository(api: environment.api),
            hints: environment.hints)
        closed = ProposalListModel(
            cabalID: cabalID, filter: .closed, repository: ProposalsRepository(api: environment.api),
            hints: environment.hints)
        model = created
        return created
    }
}

private nonisolated struct CabalProposalListRoute: Hashable, AppRoute {
    let cabalID: String
    @MainActor func destination() -> some View { CabalAllProposals(cabalID: cabalID) }
}

private struct CabalAllProposals: View {
    let cabalID: String
    @Environment(AppEnvironment.self) private var environment
    @State private var open: ProposalListModel?
    @State private var closed: ProposalListModel?
    @State private var voting: ProposalVoteModel?

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                if let open, let closed, let voting {
                    ForEach(open.pager.items + closed.pager.items) { proposal in
                        ProposalVoteCard(
                            proposal: proposal, voting: voting,
                            onVoted: { await open.pager.refreshFirstPage() })
                    }
                }
            }
            .padding(MonacoTheme.Space.m)
        }
        .navigationTitle("Proposals")
        .task {
            let models = preparedModels()
            await models.open.load()
            await models.closed.load()
        }
    }

    private func preparedModels() -> (open: ProposalListModel, closed: ProposalListModel) {
        if let open, let closed { return (open, closed) }
        let repository = ProposalsRepository(api: environment.api)
        let created = (
            open: ProposalListModel(cabalID: cabalID, filter: .open, repository: repository, hints: environment.hints),
            closed: ProposalListModel(
                cabalID: cabalID, filter: .closed, repository: repository, hints: environment.hints)
        )
        voting = ProposalVoteModel(repository: repository)
        open = created.open
        closed = created.closed
        return created
    }
}
