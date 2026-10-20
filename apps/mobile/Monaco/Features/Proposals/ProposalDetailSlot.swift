import MonacoAPI
import MonacoCore
import SwiftUI

enum ProposalDetailSlot: ProposalSection {
    static let isLive = true

    static func body(for context: ProposalContext) -> some View {
        ProposalDetailSlotView(proposalID: context.proposalID)
    }
}

struct ProposalDetailSlotView: View {
    let proposalID: String
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(ScreenRefresh.self) private var refresh: ScreenRefresh?
    @Environment(\.proposalDetail) private var shared
    @Environment(\.presentsProposalVotes) private var presentsVotes
    @State private var owned: ProposalDetailModel?
    @State private var pause: ProposalPauseModel?
    @State private var showsVotes = false
    @State private var confirmingWithdrawal = false

    init(proposalID: String, model: ProposalDetailModel? = nil) {
        self.proposalID = proposalID
        _owned = State(initialValue: model)
    }

    private var model: ProposalDetailModel? { owned ?? shared }

    var body: some View {
        content
            .navigationTitle(model?.summary?.title(asset: model?.asset) ?? "Proposal")
            .navigationBarTitleDisplayMode(.inline)
            .task { await run() }
            .onScreenVisibilityChange {
                model?.setVisible($0)
                pause?.setVisible($0)
            }
            .onAppear { if presentsVotes { showsVotes = true } }
            .sheet(isPresented: $showsVotes) {
                ProposalVotesSheet(model: model)
            }
            .confirmationDialog(
                "Withdraw this proposal?", isPresented: $confirmingWithdrawal, titleVisibility: .visible
            ) {
                Button("Withdraw", role: .destructive) { withdraw() }
                    .accessibilityIdentifier("proposal-withdraw-confirm")
                Button("Cancel", role: .cancel) {}
                    .accessibilityIdentifier("proposal-withdraw-cancel")
            } message: {
                Text("Votes so far are dropped.")
            }
    }

    @ViewBuilder private var content: some View {
        if let detail = model?.value, let summary = model?.summary, let model {
            loaded(detail, summary, model)
        } else if model?.errorMessage != nil {
            EmptyState(
                title: "Couldn't load this view",
                message: "Your existing information hasn't been changed. Try again when your connection is available.",
                actionTitle: "Try again", systemImage: MonacoIcon.offline.symbol
            ) {
                Task { await model?.load() }
            }
            .accessibilityIdentifier("proposal-detail-error")
        } else {
            skeleton
        }
    }

    private func loaded(_ detail: ProposalDetail, _ summary: ProposalSummary, _ model: ProposalDetailModel)
        -> some View
    {
        let state = summary.state(now: .now)
        return VStack(alignment: .leading, spacing: MonacoTheme.Space.section) {
            ProposalHero(
                summary: summary, asset: model.asset, members: model.members, paused: pause?.isPaused == true,
                authorReturn: detail.proposerReturn
            )
            .padding(.horizontal, MonacoTheme.Space.gutter)
            if let thesis = summary.thesis, !thesis.isEmpty {
                ProposalReasonSection(title: "Thesis", text: thesis)
            }
            votes(detail, model)
            ProposalVoteSection(model: model, summary: summary)
            expected(summary, model)
            if let network = summary.network, summary.status == .open || summary.status == .passed {
                ProposalChainSection(
                    network: network, unavailable: model.livePrice.unavailableReason(for: summary))
            }
            ProposalBestPrice(summary: summary, model: model)
            if !state.isOpen {
                ProposalStatusSection(model: model, summary: summary)
            }
            if model.canWithdraw { withdrawButton }
        }
        .padding(.top, MonacoTheme.Space.m)
    }

    private func votes(_ detail: ProposalDetail, _ model: ProposalDetailModel) -> some View {
        let groups = ProposalVoterGroups(voters: detail.voters, members: model.members)
        return VStack(alignment: .leading, spacing: 0) {
            MonacoSectionHeader("Votes", trailing: "See all", actionIdentifier: "proposal-votes-see-all") {
                showsVotes = true
            }
            VoteBar(
                yes: groups.yes.count, no: groups.no.count, notVoted: groups.notVoted.count,
                needed: detail.summary.tally.needed, isClosed: detail.summary.status != .open)
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("proposal-votes")
    }

    @ViewBuilder private func expected(_ summary: ProposalSummary, _ model: ProposalDetailModel) -> some View {
        let isPending = summary.status == .open || summary.status == .passed
        if isPending, summary.swap?.status != "failed",
            let line = ProposalCardCopy.expected(
                isSell: summary.kind == "sell", quoteOut: summary.quoteOutAmount, usdcMicros: summary.usdcMicros,
                decimals: model.asset?.decimals ?? AssetCatalogDefaults.decimals, kind: model.asset?.kind ?? .stock)
        {
            VStack(alignment: .leading, spacing: 0) {
                MonacoSectionHeader("Expected")
                MonacoCard(style: .inset) {
                    ReceiptLine(label: "Cabal gets", value: .words(line), isLast: true)
                }
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .accessibilityIdentifier("proposal-expected")
        }
    }

    private var withdrawButton: some View {
        Button {
            confirmingWithdrawal = true
        } label: {
            Text("Withdraw proposal").foregroundStyle(MonacoTheme.loss)
        }
        .buttonStyle(.monacoOutline)
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .accessibilityIdentifier("proposal-withdraw")
    }

    private var skeleton: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            HStack {
                SkeletonBlock(
                    width: StockMarkSize.review.rawValue, height: StockMarkSize.review.rawValue,
                    radius: MonacoTheme.Radius.tile)
                Spacer()
                SkeletonBar(width: 96)
            }
            SkeletonTitle()
            SkeletonFraction(0.43) { SkeletonBar() }
            SkeletonCardBlock()
            MonacoRowSkeleton(rows: 3)
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.m)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Loading proposal")
    }

    private func run() async {
        let model = preparedModel()
        refresh?.register("proposal-detail") { await model.refresh() }
        if model.value == nil { await model.load() }
        guard let cabalID = model.value?.summary.cabalID else { return }
        async let detailHints: Void = model.observe()
        let pause =
            self.pause
            ?? ProposalPauseModel(
                cabalID: cabalID, repository: ProposalsRepository(api: environment.api), hints: environment.hints)
        self.pause = pause
        await pause.load()
        await pause.observe()
        await detailHints
    }

    private func withdraw() {
        Task {
            let model = preparedModel()
            await model.withdraw()
            if model.didWithdraw {
                toasts.show(success: "Proposal withdrawn.")
            } else if let message = model.errorMessage {
                toasts.current = MonacoToast(message: message)
            }
        }
    }

    private func preparedModel() -> ProposalDetailModel {
        if let model { return model }
        let created = ProposalDetailModel(
            id: proposalID, cabalID: "", repository: ProposalsRepository(api: environment.api),
            hints: environment.hints)
        owned = created
        return created
    }
}

private struct ProposalBestPrice: View {
    let summary: ProposalSummary
    let model: ProposalDetailModel

    var body: some View {
        let decimals = model.asset?.decimals ?? AssetCatalogDefaults.decimals
        if LiveBestPriceModel.isLive(summary) {
            TimelineView(.periodic(from: .now, by: 1)) { context in
                if model.livePrice.isFindingPrice(for: summary, decimals: decimals, now: context.date) {
                    BestPriceLoading()
                } else if let row = model.livePrice.row(for: summary, decimals: decimals, now: context.date) {
                    BestPriceSection(row: row)
                }
            }
        } else if let row = model.livePrice.row(for: summary, decimals: decimals, now: .now) {
            BestPriceSection(row: row)
        }
    }
}

private struct BestPriceLoading: View {
    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            MonacoSectionHeader("Price")
            MonacoCard(style: .inset) {
                HStack(spacing: MonacoTheme.Space.s) {
                    ProgressView()
                        .controlSize(.small)
                    Text(ProposeAmountModel.findingPrice)
                        .font(MonacoTheme.Typo.subhead)
                        .foregroundStyle(MonacoTheme.muted)
                    Spacer(minLength: 0)
                }
                .padding(.vertical, MonacoTheme.Space.sm)
                .frame(minHeight: ReceiptLineMetrics.minHeight)
                .accessibilityElement(children: .combine)
                .accessibilityIdentifier("best-price-loading")
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
    }
}

private struct ProposalChainSection: View {
    let network: Components.Schemas.Network
    let unavailable: String?

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            MonacoSectionHeader("Network")
            MonacoCard(style: .inset) {
                VStack(spacing: 0) {
                    ReceiptLine(label: "Chain", value: .words(BestPrice.chainName(network)), isLast: unavailable == nil)
                    if let unavailable {
                        ReceiptLine(label: "Price", value: .words(unavailable), isLast: true)
                    }
                }
                .accessibilityIdentifier("proposal-chain-row")
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
    }
}
