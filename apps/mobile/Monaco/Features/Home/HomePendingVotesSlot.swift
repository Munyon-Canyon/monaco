import MonacoCore
import SwiftUI

enum HomePendingVotesSlot: HomeSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        HomePendingVotes()
    }
}

enum HomeProposalsMetrics {
    static let shownLimit = 2
    static let loadingTitleFraction: CGFloat = 0.46
    static let loadingSubtitleFraction: CGFloat = 0.72
    static let loadingFooterFraction: CGFloat = 0.65
}

struct HomePendingVotes: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(PortfolioModel.self) private var portfolio: PortfolioModel?
    @Environment(\.homeReads) private var reads
    @Environment(\.homeVoteNeeds) private var voteNeeds
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @State private var model: PendingVotesModel?
    @State private var voting: ProposalVoteModel?
    private let makeModel: @MainActor (AppEnvironment) -> PendingVotesModel

    init(makeModel: @escaping @MainActor (AppEnvironment) -> PendingVotesModel = HomePendingVotes.liveModel) {
        self.makeModel = makeModel
    }

    private var isHidden: Bool {
        guard portfolio != nil else { return false }
        let gate = HomeGate(portfolio: portfolio, reads: reads, balance: environment.balance.state)
        return !gate.shows { $0.showsProposals }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            if let model {
                let shown = shownVotes(model)
                MonacoSectionHeader("Proposals", count: waiting(shown, model), trailing: "See all") {
                    environment.navigator.open(PendingVotesRoute(), in: .home)
                }
                switch model.phase {
                case .loading: loading
                case .failed: failed(model)
                case .loaded: loaded(shown, model)
                }
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .homeSlotHidden(isHidden)
        .animation(reduceMotion ? nil : .snappy, value: model?.votes.map(\.id))
        .task {
            let model = preparedModel()
            await model.load()
            await model.observe()
        }
        .onScreenVisibilityChange { model?.setVisible($0) }
        .onChange(of: voteSignature, initial: true) { _, _ in
            guard let model else { return }
            voteNeeds?.update(model, votedThisSession: voting?.votedIDs ?? [])
        }
    }

    private var voteSignature: [String] {
        guard let model else { return [] }
        let voted = voting?.votedIDs ?? []
        return model.votes.map { "\($0.id)|\(model.details[$0.id]?.summary.myBallot != nil)|\(voted.contains($0.id))" }
    }

    private func shownVotes(_ model: PendingVotesModel) -> [PendingVote] {
        Array((model.needsVote + model.inProgress).prefix(HomeProposalsMetrics.shownLimit))
    }

    private func waiting(_ shown: [PendingVote], _ model: PendingVotesModel) -> Int? {
        let voted = voting?.votedIDs ?? []
        let needing = Set(model.needsVote.map(\.id))
        let count = shown.filter {
            needing.contains($0.id) && !voted.contains($0.id) && model.details[$0.id]?.summary.myBallot == nil
        }.count
        return count > 0 ? count : nil
    }

    @ViewBuilder private func loaded(_ shown: [PendingVote], _ model: PendingVotesModel) -> some View {
        if shown.isEmpty {
            quietEmpty
        } else if let voting {
            VStack(spacing: MonacoTheme.Space.sm) {
                ForEach(shown) { vote in
                    if let detail = model.details[vote.id] {
                        ProposalVoteCard(
                            proposal: detail.summary, voting: voting,
                            asset: model.assets[detail.summary.symbol],
                            members: model.members[detail.summary.cabalID] ?? [],
                            paused: model.pausedCabals.contains(detail.summary.cabalID),
                            onVoted: { await model.load() })
                    }
                }
            }
        }
    }

    private var quietEmpty: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            MonacoQuietEmpty("No proposals need your attention.", systemImage: MonacoIcon.checkCircle.symbol)
            Text("Closed decisions remain in proposal history.")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("home-proposals-empty")
    }

    private var loading: some View {
        MonacoCard {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
                SkeletonFraction(HomeProposalsMetrics.loadingTitleFraction) { SkeletonBar() }
                    .frame(maxWidth: .infinity, alignment: .leading)
                HStack(spacing: MonacoTheme.Space.sm) {
                    SkeletonBlock(
                        width: MonacoRowLayout.baseMarkSize, height: MonacoRowLayout.baseMarkSize,
                        radius: MonacoRowLayout.baseMarkSize / 2)
                    VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                        SkeletonBlock(height: SkeletonMetrics.lineHeight, radius: SkeletonMetrics.lineRadius)
                        SkeletonFraction(HomeProposalsMetrics.loadingSubtitleFraction) {
                            SkeletonBlock(height: SkeletonMetrics.lineHeight, radius: SkeletonMetrics.lineRadius)
                        }
                        .frame(maxWidth: .infinity, alignment: .leading)
                    }
                }
                SkeletonFraction(HomeProposalsMetrics.loadingFooterFraction) { SkeletonBar() }
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
        }
        .accessibilityElement()
        .accessibilityLabel("Loading proposals")
        .accessibilityIdentifier("home-proposals-loading")
    }

    private func failed(_ model: PendingVotesModel) -> some View {
        MonacoCard {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                HStack(alignment: .firstTextBaseline, spacing: MonacoTheme.Space.sm) {
                    Image(monaco: .offline)
                        .foregroundStyle(MonacoTheme.secondaryText)
                        .accessibilityHidden(true)
                    Text("Proposals couldn't load")
                        .font(MonacoTheme.Typo.headline)
                        .foregroundStyle(MonacoTheme.primaryText)
                }
                Text("Your balances and other sections are still available.")
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
                HStack {
                    Spacer(minLength: 0)
                    Button("Try again") { Task { await model.load() } }
                        .buttonStyle(.monacoCompact)
                        .accessibilityIdentifier("home-proposals-retry")
                }
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("home-proposals-error")
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
