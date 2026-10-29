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
            .navigationTitle("Proposal")
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
                subtitle: model.asset?.preIpo == nil ? nil : PreIpoCopy.proposalSubtitle
            )
            .padding(.horizontal, MonacoTheme.Space.gutter)
            if let stats = model.stats {
                ProposalStatRow(stats: stats)
            }
            if let preIpo = model.asset?.preIpo, let profile = preIpo.profile {
                ProposalPreIpoLedger(lines: profile.ledger, eligibility: preIpo.eligibility)
            }
            if let thesis = summary.thesis, !thesis.isEmpty {
                ProposalReasonSection(title: "Thesis", text: thesis, style: .plain)
            }
            ProposalVoteSection(model: model, summary: summary)
            if !state.isOpen {
                ProposalStatusSection(model: model, summary: summary)
            }
            if model.canWithdraw { withdrawButton }
        }
        .padding(.top, MonacoTheme.Space.m)
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
