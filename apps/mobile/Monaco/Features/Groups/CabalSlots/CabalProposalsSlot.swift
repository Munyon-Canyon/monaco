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
    @Environment(\.hostMainTab) private var hostMainTab
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?
    @Environment(\.cabalModel) private var cabalModel
    @State private var model: ProposalListModel?
    @Environment(\.analytics) private var analytics
    @State private var assets = CabalProposalAssets()
    @State private var cards = CabalVoteCards()
    private let makeModel: @MainActor (AppEnvironment, String) -> ProposalListModel

    init(
        cabalID: String,
        makeModel: @escaping @MainActor (AppEnvironment, String) -> ProposalListModel = CabalProposals.liveModel
    ) {
        self.cabalID = cabalID
        self.makeModel = makeModel
    }

    var body: some View {
        VStack(spacing: 0) { content }
            .task {
                let model = preparedModel()
                refresh?.register("cabal-proposals") { await model.refresh() }
                await model.load()
            }
            .task(id: Self.open(model?.pager.items ?? []).map(\.symbol)) { await loadAssets() }
            .task { await preparedModel().observe(cabalID: cabalID) }
            .onScreenVisibilityChange { model?.setVisible($0) }
    }

    private var isReadOnly: Bool {
        cabalModel?.cabal.map { CabalClosedModel.isReadOnly($0.status) } ?? false
    }

    @ViewBuilder private var content: some View {
        if let model, !isReadOnly {
            let open = Self.open(model.pager.items)
            if open.isEmpty {
                switch model.pager.phase {
                case .loadingFirst:
                    MonacoRowSkeleton(rows: 1)
                case .failed:
                    MonacoErrorRow(thing: "proposals", identifier: "cabal-votes-error", inset: false) {
                        Task { await model.load() }
                    }
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                default:
                    Color.clear.frame(height: 0)
                }
            } else {
                VStack(spacing: MonacoTheme.Space.m) {
                    ForEach(open) { proposal in
                        HomeVoteCard(
                            model: card(for: proposal), identifierPrefix: Self.cardPrefix,
                            tab: hostMainTab ?? environment.navigator.selectedTab, inCabal: true,
                            canVote: proposal.canVote
                        )
                        .id(proposal.id)
                    }
                }
                .padding(.horizontal, MonacoTheme.Space.gutter)
            }
        }
    }

    static func open(_ proposals: [ProposalSummary]) -> [ProposalSummary] {
        proposals.filter { $0.status == .open }.sorted { $0.createdAt > $1.createdAt }
    }

    static let cardPrefix = "cabal-open-proposal"

    private func card(for proposal: ProposalSummary) -> HomeVoteCardModel {
        let row = Self.pendingVote(proposal, asset: assets.byTicker[proposal.symbol])
        return cards.card(
            for: row, ballot: proposal.myBallot.flatMap(ProposalBallot.init(rawValue:)), environment: environment
        ) {
            [environment, toasts, analytics, weak model] card, outcome in
            HomeVoteCommit.accepted(card, outcome, environment: environment, toasts: toasts, analytics: analytics)
            Task { await model?.refresh() }
        }
    }

    static func pendingVote(_ proposal: ProposalSummary, asset: ProposalAsset?) -> HomePendingVote {
        HomePendingVote(
            id: proposal.id, cabalID: proposal.cabalID, cabalName: proposal.cabalName, kind: proposal.kind,
            rule: proposal.ruleChange?.rule, symbol: proposal.isRule ? nil : proposal.symbol,
            assetName: asset?.displayName, logoURL: asset?.logoURL, usdcMicros: proposal.usdcMicros,
            tokenAmount: proposal.tokenAmount, tokenDecimals: proposal.tokenDecimals, tally: proposal.tally,
            replyCount: proposal.replyCount, expiresAt: proposal.expiresAt)
    }

    private func loadAssets() async {
        await assets.load(
            symbols: Self.open(model?.pager.items ?? []).map(\.symbol),
            repository: ProposalsRepository(api: environment.api))
    }

    private func preparedModel() -> ProposalListModel {
        if let model { return model }
        let created = makeModel(environment, cabalID)
        created.onOutcome = { [assets, toasts] outcome in
            let message = outcome.toast(asset: assets.byTicker[outcome.proposal.symbol])
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

@MainActor
final class CabalVoteCards {
    private var models: [String: HomeVoteCardModel] = [:]
    func card(
        for row: HomePendingVote, ballot: ProposalBallot?, environment: AppEnvironment,
        onCommitted: @escaping @MainActor (HomeVoteCardModel, CastOutcome) -> Void
    ) -> HomeVoteCardModel {
        if let existing = models[row.id] {
            if existing.row != row { existing.update(row) }
            return existing
        }
        let created = HomeVoteCardModel(
            api: environment.api, hints: environment.hints, row: row, myBallot: ballot)
        created.onCommitted = { [weak created] outcome in
            if let created { onCommitted(created, outcome) }
        }
        models[row.id] = created
        return created
    }
}

struct CabalLiveCount: View {
    let yes: Int
    let no: Int

    var body: some View {
        HStack(spacing: MonacoTheme.Space.xs) {
            Image(systemName: "checkmark")
                .foregroundStyle(MonacoTheme.profit)
                .accessibilityHidden(true)
            Text("\(yes)")
            Text("\u{00B7}")
                .foregroundStyle(MonacoTheme.secondaryText)
            Image(systemName: "xmark")
                .foregroundStyle(MonacoTheme.loss)
                .accessibilityHidden(true)
            Text("\(no)")
        }
        .font(MonacoTheme.Typo.rank)
        .foregroundStyle(MonacoTheme.primaryText)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("\(yes) yes, \(no) no")
    }
}
