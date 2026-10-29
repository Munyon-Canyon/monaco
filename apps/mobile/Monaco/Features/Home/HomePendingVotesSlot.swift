import MonacoCore
import SwiftUI

enum HomePendingVotesSlot: HomeSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        HomePendingVotes()
    }
}

enum HomeProposalsMetrics {
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
    @State private var model: HomePendingVotesModel?
    private let makeModel: @MainActor (AppEnvironment) -> HomePendingVotesModel

    init(
        makeModel: @escaping @MainActor (AppEnvironment) -> HomePendingVotesModel = HomePendingVotes.liveModel
    ) {
        self.makeModel = makeModel
    }

    private var isHidden: Bool {
        guard portfolio != nil else { return false }
        let gate = HomeGate(portfolio: portfolio, reads: reads, balance: environment.balance.state)
        return !gate.shows { $0.showsProposals } || !gate.shows(.vote)
    }

    private var waitsForGate: Bool {
        guard let portfolio else { return false }
        return portfolio.summary == nil || !HomeFirstUse.shows(.vote, summary: portfolio.summary)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            if let model {
                MonacoSectionHeader(
                    "Your vote", count: model.waitingCount > 0 ? model.waitingCount : nil, trailing: "All proposals"
                ) {
                    environment.navigator.open(PendingVotesRoute(), in: .home)
                }
                switch model.phase {
                case .loading: loading
                case .failed: failed(model)
                case .loaded: loaded(model)
                }
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .homeSlotHidden(isHidden)
        .animation(reduceMotion ? nil : .snappy, value: model?.cards.map(\.id))
        .task(id: waitsForGate) {
            guard !waitsForGate else { return }
            let model = preparedModel()
            await model.load()
            await model.observe()
        }
        .onScreenVisibilityChange { model?.setVisible($0) }
        .onChange(of: voteSignature, initial: true) { _, _ in
            guard let model else { return }
            voteNeeds?.update(rows: model.rows, votedThisSession: model.votedIDs)
        }
    }

    private var voteSignature: [String] {
        model?.cards.map { "\($0.id)|\($0.isCommitted)" } ?? []
    }

    @ViewBuilder private func loaded(_ model: HomePendingVotesModel) -> some View {
        if let card = model.shown {
            HomeVoteCard(model: card).id(card.id)
        } else {
            quietEmpty
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

    private func failed(_ model: HomePendingVotesModel) -> some View {
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

    private func preparedModel() -> HomePendingVotesModel {
        if let model { return model }
        let created = makeModel(environment)
        model = created
        return created
    }

    static func liveModel(_ environment: AppEnvironment) -> HomePendingVotesModel {
        let model = HomePendingVotesModel(api: environment.api, hints: environment.hints)
        #if DEBUG
        if let scenario = HomeSampleScenario.matching(ProcessInfo.processInfo.arguments) {
            model.cardSetup = scenario.voteCardSetup
        }
        #endif
        return model
    }
}
