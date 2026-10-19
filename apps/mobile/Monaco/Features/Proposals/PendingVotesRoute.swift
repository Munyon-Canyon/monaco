import MonacoCore
import SwiftUI

nonisolated struct PendingVotesRoute: AppRoute {
    @MainActor func destination() -> some View { PendingVotesScreen() }
}

struct PendingVotesScreen: View {
    enum Segment: Hashable {
        case open
        case closed
    }

    @Environment(AppEnvironment.self) private var environment
    @State private var model: PendingVotesModel?
    @State private var voting: ProposalVoteModel?
    @State private var segment: Segment
    private let closed: [ProposalListRow]
    private let makeModel: @MainActor (AppEnvironment) -> PendingVotesModel

    init(
        makeModel: @escaping @MainActor (AppEnvironment) -> PendingVotesModel = PendingVotesScreen.liveModel,
        segment: Segment = .open, closed: [ProposalListRow] = []
    ) {
        self.makeModel = makeModel
        self.closed = closed
        _segment = State(initialValue: segment)
    }

    var body: some View {
        content
            .navigationTitle("All proposals")
            .navigationBarTitleDisplayMode(.inline)
            .task {
                let model = preparedModel()
                await model.load()
                await model.observe()
            }
            .onScreenVisibilityChange { model?.setVisible($0) }
    }

    @ViewBuilder private var content: some View {
        switch model?.phase ?? .loading {
        case .loading:
            skeleton
        case .failed:
            EmptyState(
                title: "Couldn't load this view",
                message: "Your existing information hasn't been changed. Try again when your connection is available.",
                actionTitle: "Try again", systemImage: MonacoIcon.offline.symbol
            ) {
                Task { await model?.load() }
            }
            .accessibilityIdentifier("pending-votes-error")
        case .loaded:
            loaded
        }
    }

    private var skeleton: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            SkeletonTitle()
                .padding(.horizontal, MonacoTheme.Space.gutter)
            SkeletonFraction(0.43) { SkeletonBar() }
                .padding(.horizontal, MonacoTheme.Space.gutter)
            MonacoRowSkeleton(rows: 4)
            Spacer(minLength: 0)
        }
        .padding(.top, MonacoTheme.Space.m)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Loading proposals")
    }

    private var loaded: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.l) {
                MonacoSegmented(
                    [Segment.open, .closed], selection: $segment,
                    identifier: { $0 == .open ? "proposals-segment-open" : "proposals-segment-closed" }
                ) {
                    $0 == .open ? "Open" : "Closed"
                }
                .padding(.horizontal, MonacoTheme.Space.gutter)
                switch segment {
                case .open: openProposals
                case .closed: closedProposals
                }
            }
            .padding(.top, MonacoTheme.Space.m)
        }
        .refreshable { await model?.load() }
    }

    @ViewBuilder private var openProposals: some View {
        if let model, !model.votes.isEmpty {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                VStack(spacing: MonacoTheme.Space.sm) {
                    ForEach(model.needsVote) { vote in
                        card(vote, model: model)
                    }
                }
                .padding(.horizontal, MonacoTheme.Space.gutter)
                let rows = inProgressRows(model)
                VStack(spacing: 0) {
                    ForEach(rows) { row in
                        ProposalListRowView(row: row, isLast: row.id == rows.last?.id)
                    }
                }
            }
        } else {
            EmptyState(
                title: "No open proposals",
                message: "New ideas will appear here. Closed decisions remain in the cabal history.",
                actionTitle: "Choose a proposal", systemImage: MonacoIcon.news.symbol
            ) {
                environment.navigator.select(.cabals)
            }
            .accessibilityIdentifier("pending-votes-empty")
        }
    }

    @ViewBuilder private var closedProposals: some View {
        if closed.isEmpty {
            MonacoQuietEmpty("Closed proposals show up here soon.", systemImage: MonacoIcon.clock.symbol)
                .padding(.horizontal, MonacoTheme.Space.gutter)
        } else {
            VStack(spacing: 0) {
                ForEach(closed) { row in
                    ProposalListRowView(row: row, isLast: row.id == closed.last?.id)
                }
            }
        }
    }

    @ViewBuilder private func card(_ vote: PendingVote, model: PendingVotesModel) -> some View {
        if let detail = model.details[vote.id], let voting {
            ProposalVoteCard(
                proposal: detail.summary, voting: voting,
                asset: model.assets[detail.summary.symbol],
                members: model.members[detail.summary.cabalID] ?? [],
                paused: model.pausedCabals.contains(detail.summary.cabalID),
                onVoted: { await model.load() })
        }
    }

    private func inProgressRows(_ model: PendingVotesModel) -> [ProposalListRow] {
        model.inProgress.compactMap { vote in
            guard let summary = model.details[vote.id]?.summary else { return nil }
            let proposer = model.members[summary.cabalID]?.first { $0.id == summary.proposerID }?.name ?? "Member"
            return ProposalListRow(
                summary: summary, asset: model.assets[summary.symbol], proposer: proposer, now: .now)
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
