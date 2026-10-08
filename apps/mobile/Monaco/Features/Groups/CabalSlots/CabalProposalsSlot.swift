import MonacoCore
import SwiftUI

enum CabalProposalsSlot: CabalSection {
    static let isLive = true

    static func body(for context: CabalContext) -> some View {
        CabalProposals(cabalID: context.cabalID)
    }
}

struct CabalProposals: View {
    let cabalID: String
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @State private var model: ProposalListModel?
    @State private var pause: ProposalPauseModel?
    @State private var voting: ProposalVoteModel?
    @State private var context: ProposalCardContext?
    private let makeModel: @MainActor (AppEnvironment, String) -> ProposalListModel

    init(
        cabalID: String,
        makeModel: @escaping @MainActor (AppEnvironment, String) -> ProposalListModel = CabalProposals.liveModel
    ) {
        self.cabalID = cabalID
        self.makeModel = makeModel
    }

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
        .task(id: model?.pager.items.map(\.id)) { await context?.load(for: model?.pager.items ?? []) }
        .task { await preparedModel().observe(cabalID: cabalID) }
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
        let votedThisSession = voting?.votedIDs ?? []
        let section = model.cabalSection(votedThisSession: votedThisSession)
        let inProgress = model.trading
        let recent = model.recentOutcomes(now: .now)
        if section == nil && inProgress.isEmpty && recent.isEmpty {
            EmptyState(title: "No open votes", message: "Propose the first buy.")
            if !model.pager.items.isEmpty {
                NavigationLink("See all", value: AnyAppRoute(CabalProposalListRoute(cabalID: cabalID)))
            }
        } else {
            if let section {
                self.section(section.title, section.proposals, count: section.count, showsSeeAll: true, model: model)
            }
            if !inProgress.isEmpty {
                self.section(
                    "In progress", inProgress, count: inProgress.count, showsSeeAll: section == nil, model: model)
            }
            if !recent.isEmpty {
                self.section(
                    "Recently closed", recent, count: nil, showsSeeAll: section == nil && inProgress.isEmpty,
                    model: model)
            }
        }
    }

    @ViewBuilder private func section(
        _ title: String, _ proposals: [ProposalSummary], count: Int?, showsSeeAll: Bool, model: ProposalListModel
    ) -> some View {
        HStack {
            MonacoSectionHeader(title, count: count)
            Spacer()
            if showsSeeAll {
                NavigationLink("See all", value: AnyAppRoute(CabalProposalListRoute(cabalID: cabalID)))
            }
        }
        ForEach(proposals) { proposal in
            if let voting {
                ProposalVoteCard(
                    proposal: proposal, voting: voting, asset: context?.assets[proposal.symbol],
                    members: context?.members ?? [], paused: pause?.isPaused == true,
                    onVoted: { await model.refresh() })
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
        context = ProposalCardContext(cabalID: cabalID, repository: ProposalsRepository(api: environment.api))
        let created = makeModel(environment, cabalID)
        created.onOutcome = { [weak context, toasts] outcome in
            let asset = context?.assets[outcome.proposal.symbol]
            let message = outcome.toast(asset: asset)
            if outcome.proposal.status == .executed {
                toasts.show(success: message)
            } else {
                toasts.current = MonacoToast(message: message)
            }
        }
        model = created
        return created
    }

    static func liveModel(_ environment: AppEnvironment, _ cabalID: String) -> ProposalListModel {
        ProposalListModel(
            cabalID: cabalID, filter: .all, repository: ProposalsRepository(api: environment.api),
            hints: environment.hints)
    }
}

private nonisolated struct CabalProposalListRoute: Hashable, AppRoute {
    let cabalID: String
    @MainActor func destination() -> some View { CabalAllProposals(cabalID: cabalID) }
}

private struct CabalAllProposals: View {
    let cabalID: String
    @Environment(AppEnvironment.self) private var environment
    @State private var segments: ProposalSegmentsModel?
    @State private var voting: ProposalVoteModel?
    @State private var context: ProposalCardContext?

    var body: some View {
        VStack(spacing: MonacoTheme.Space.s) {
            if let segments, let voting, let context {
                MonacoSegmented(ProposalSegment.allCases, selection: Bindable(segments).selected) { $0.title }
                    .padding(.horizontal, MonacoTheme.Space.m)
                CabalProposalSegmentList(
                    segment: segments.selected, model: segments.model(for: segments.selected), voting: voting,
                    context: context, cabalID: cabalID, onVoted: { await segments.refreshLoaded() }
                )
                .id(segments.selected)
            }
        }
        .navigationTitle("Proposals")
        .task { prepare() }
    }

    private func prepare() {
        guard segments == nil else { return }
        let repository = ProposalsRepository(api: environment.api)
        segments = ProposalSegmentsModel(cabalID: cabalID, repository: repository, hints: environment.hints)
        voting = ProposalVoteModel(repository: repository)
        context = ProposalCardContext(cabalID: cabalID, repository: repository)
    }
}

private struct CabalProposalSegmentList: View {
    let segment: ProposalSegment
    let model: ProposalListModel
    let voting: ProposalVoteModel
    let context: ProposalCardContext
    let cabalID: String
    let onVoted: () async -> Void

    var body: some View {
        ScrollView {
            LazyVStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                if model.pager.items.isEmpty {
                    switch model.pager.phase {
                    case .idle, .loadingFirst:
                        SkeletonBlock(width: 280, height: 160, radius: MonacoTheme.Radius.card)
                    case .failed:
                        EmptyState(title: "Couldn't load proposals.", actionTitle: "Try again") {
                            Task { await model.load() }
                        }
                    default:
                        EmptyState(title: segment.emptyTitle)
                    }
                }
                ForEach(model.pager.items) { proposal in
                    ProposalVoteCard(
                        proposal: proposal, voting: voting, asset: context.assets[proposal.symbol],
                        members: context.members, onVoted: onVoted
                    )
                    .onAppear {
                        if proposal.id == model.pager.items.last?.id { Task { await model.pager.loadMore() } }
                    }
                }
            }
            .padding(MonacoTheme.Space.m)
        }
        .task { await model.load() }
        .task { await model.observe(cabalID: cabalID) }
        .task(id: model.pager.items.map(\.id)) { await context.load(for: model.pager.items) }
        .onScreenVisibilityChange { model.setVisible($0) }
    }
}
