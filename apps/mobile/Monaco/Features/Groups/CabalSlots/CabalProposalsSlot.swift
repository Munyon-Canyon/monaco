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
    @State private var model: ProposalListModel?
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
        VStack(spacing: 0) { content }
            .padding(.top, -(SectionStackMetrics.spacing - MonacoTheme.Space.l))
            .task {
                let model = preparedModel()
                refresh?.register("cabal-proposals") { await model.refresh() }
                await model.load()
            }
            .task(id: model?.pager.items.map(\.id)) { await context?.load(for: Self.open(model?.pager.items ?? [])) }
            .task { await preparedModel().observe(cabalID: cabalID) }
            .onScreenVisibilityChange { model?.setVisible($0) }
    }

    @ViewBuilder private var content: some View {
        if let model {
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
                VStack(spacing: 0) {
                    ForEach(Array(open.enumerated()), id: \.element.id) { index, proposal in
                        CabalOpenProposalRow(
                            proposal: proposal, asset: context?.assets[proposal.symbol],
                            isLast: index == open.count - 1, open: { openProposal(proposal) })
                    }
                }
                .accessibilityIdentifier("cabal-open-proposals")
            }
        }
    }

    static func open(_ proposals: [ProposalSummary]) -> [ProposalSummary] {
        proposals.filter { $0.status == .open }.sorted { $0.createdAt > $1.createdAt }
    }

    private func openProposal(_ proposal: ProposalSummary) {
        environment.navigator.open(
            ProposalRoute(proposalID: proposal.id), in: hostMainTab ?? environment.navigator.selectedTab)
    }

    private func preparedModel() -> ProposalListModel {
        if let model { return model }
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
            cabalID: cabalID, filter: .open, repository: ProposalsRepository(api: environment.api),
            hints: environment.hints)
    }
}

struct CabalOpenProposalRow: View {
    let proposal: ProposalSummary
    let asset: ProposalAsset?
    var isLast = false
    let open: () -> Void

    var body: some View {
        Button(action: open) {
            MonacoRow(
                title: Self.title(proposal, asset: asset), subtitle: Self.statusLine(proposal), isLast: isLast,
                isPrivate: true,
                leading: {
                    StockMark(
                        symbol: proposal.symbol, displayName: asset?.displayName, assetKind: asset?.kind ?? .stock,
                        size: MonacoRowLayout.baseMarkSize, logoURL: asset?.logoURL)
                },
                trailing: { CabalLiveCount(yes: proposal.tally.yes, no: proposal.tally.no) })
        }
        .buttonStyle(.monacoRow)
        .accessibilityElement(children: .combine)
        .accessibilityHint("Opens the proposal")
        .accessibilityIdentifier("cabal-open-proposal-\(proposal.id)")
    }

    static func title(_ proposal: ProposalSummary, asset: ProposalAsset?) -> String {
        let isSell = proposal.kind == "sell"
        let amount =
            isSell
            ? ProposalCard.shareLabel(summary: proposal, asset: asset)
            : proposal.usdcMicros.map(CabalHeaderCopy.compactDollars)
        let ticker = AssetSymbolFormatter.display(proposal.symbol, kind: asset?.kind ?? .stock)
        let name = "\(isSell ? "Sell" : "Buy") \(ticker)"
        return [name, amount].compactMap { $0 }.joined(separator: " \u{00B7} ")
    }

    static func statusLine(_ proposal: ProposalSummary, now: Date = .now) -> String {
        ProposalVoteCopy.statusLine(tally: proposal.tally, replies: 0, expiresAt: proposal.expiresAt, now: now)
    }
}

private struct CabalLiveCount: View {
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
