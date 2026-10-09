import MonacoAPI
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
    @Environment(\.cabalModel) private var cabalModel
    @Environment(\.hostMainTab) private var hostMainTab
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?
    @State private var model: ProposalListModel?
    @State private var pause: ProposalPauseModel?
    @State private var voting: ProposalVoteModel?
    @State private var context: ProposalCardContext?
    @State private var expanded = false
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
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
                    ProposalCardSkeleton()
                        .padding(.horizontal, MonacoTheme.Space.gutter)
                case .failed where model.pager.items.isEmpty:
                    MonacoErrorRow(thing: "votes", identifier: "cabal-votes-error") { Task { await model.load() } }
                default:
                    if !model.pager.items.isEmpty, context?.hasLoaded != true {
                        ProposalCardSkeleton()
                            .padding(.horizontal, MonacoTheme.Space.gutter)
                    } else {
                        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                            content(model)
                        }
                        .padding(.horizontal, MonacoTheme.Space.gutter)
                    }
                }
            }
        }
        .task {
            let model = preparedModel()
            refresh?.register("cabal-proposals") { await model.refresh() }
            await model.load()
        }
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
            MonacoSectionHeader("Proposals", trailing: model.pager.items.isEmpty ? nil : "See all", action: openAll)
            let canPropose = cabalModel?.cabal?.me?.canVote == true
            EmptyState(title: "No open votes", message: canPropose ? "Propose the first buy." : nil)
                .accessibilityIdentifier("cabal-proposals-empty")
        } else {
            let split = Self.split(section?.proposals ?? [])
            let hidden = split.rest.count + inProgress.count + recent.count
            MonacoSectionHeader("Proposals", count: section?.count, trailing: "See all", action: openAll)
            Text(
                Self.summary(
                    needsVote: section?.count ?? 0, open: section?.proposals.count ?? 0,
                    inProgress: inProgress.count, closed: recent.count)
            )
            .font(MonacoTheme.Typo.caption)
            .foregroundStyle(MonacoTheme.muted)
            .lineLimit(1)
            .accessibilityIdentifier("cabal-proposals-context")
            if let section {
                self.section(section.title == "Proposals" ? nil : section.title, split.preview, model: model)
            }
            if hidden > 0 {
                disclosure(hidden: hidden)
                if expanded {
                    self.section(nil, split.rest, model: model)
                    if !inProgress.isEmpty {
                        self.section("In progress", inProgress, model: model)
                    }
                    if !recent.isEmpty {
                        self.section("Recently closed", recent, model: model)
                    }
                }
            }
        }
    }

    private func disclosure(hidden: Int) -> some View {
        Button {
            withAnimation(reduceMotion ? nil : .snappy) { expanded.toggle() }
        } label: {
            HStack(spacing: MonacoTheme.Space.s) {
                Text(expanded ? "Show less" : "Show \(hidden) more")
                Image(systemName: "chevron.down")
                    .rotationEffect(.degrees(expanded ? 180 : 0))
                    .accessibilityHidden(true)
                Spacer(minLength: 0)
            }
            .font(MonacoTheme.Typo.calloutStrong)
            .foregroundStyle(MonacoTheme.brand)
            .frame(maxWidth: .infinity, minHeight: 44, alignment: .leading)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityIdentifier("cabal-proposals-disclosure")
        .accessibilityValue(expanded ? "Expanded" : "Collapsed")
        .accessibilityHint("Shows more proposals")
    }

    static let previewLimit = 2

    static func split(_ open: [ProposalSummary]) -> (preview: [ProposalSummary], rest: [ProposalSummary]) {
        (Array(open.prefix(previewLimit)), Array(open.dropFirst(previewLimit)))
    }

    static func summary(needsVote: Int, open: Int, inProgress: Int, closed: Int) -> String {
        var parts: [String] = []
        if needsVote > 0 {
            parts.append("\(needsVote) to vote on")
        } else if open > 0 {
            parts.append("\(open) open")
        }
        if inProgress > 0 { parts.append("\(inProgress) trading") }
        if closed > 0 { parts.append("\(closed) closed recently") }
        return parts.joined(separator: " · ")
    }

    @ViewBuilder private func section(_ label: String?, _ proposals: [ProposalSummary], model: ProposalListModel)
        -> some View
    {
        if let label {
            Text(label)
                .font(MonacoTheme.Typo.captionStrong)
                .foregroundStyle(MonacoTheme.tertiaryText)
                .padding(.top, MonacoTheme.Space.s)
                .accessibilityAddTraits(.isHeader)
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

    private func openAll() {
        environment.navigator.open(
            CabalProposalListRoute(cabalID: cabalID), in: hostMainTab ?? environment.navigator.selectedTab)
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
                    .padding(.horizontal, MonacoTheme.Space.gutter)
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
                        ProposalCardSkeleton()
                            .padding(.horizontal, MonacoTheme.Space.gutter)
                    case .failed:
                        MonacoErrorRow(thing: "proposals", identifier: "cabal-proposals-error") {
                            Task { await model.load() }
                        }
                    default:
                        EmptyState(title: segment.emptyTitle)
                            .padding(.horizontal, MonacoTheme.Space.gutter)
                    }
                }
                ForEach(model.pager.items) { proposal in
                    ProposalVoteCard(
                        proposal: proposal, voting: voting, asset: context.assets[proposal.symbol],
                        members: context.members, onVoted: onVoted
                    )
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                    .onAppear {
                        if proposal.id == model.pager.items.last?.id { Task { await model.pager.loadMore() } }
                    }
                }
            }
            .padding(.vertical, MonacoTheme.Space.m)
        }
        .task { await model.load() }
        .task { await model.observe(cabalID: cabalID) }
        .task(id: model.pager.items.map(\.id)) { await context.load(for: model.pager.items) }
        .onScreenVisibilityChange { model.setVisible($0) }
    }
}

private struct ProposalCardSkeleton: View {
    var body: some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            SkeletonBlock(width: 36, height: 36, radius: 36 * MonacoTheme.Radius.tile / MonacoRowLayout.baseMarkSize)
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                SkeletonBlock(width: 120, height: 14)
                SkeletonBlock(width: 64, height: 11)
            }
        }
        .monacoActionCard()
        .accessibilityHidden(true)
    }
}
