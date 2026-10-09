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
    @State private var model: ProposalDetailModel?
    @State private var pause: ProposalPauseModel?
    @State private var showBurst = false
    @State private var confirmingWithdrawal = false

    init(proposalID: String, model: ProposalDetailModel? = nil) {
        self.proposalID = proposalID
        _model = State(initialValue: model)
    }

    var body: some View {
        Group {
            if let detail = model?.value, let summary = model?.summary {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.l) {
                    ProposalCard(
                        proposal: summary, asset: model?.asset, members: model?.members ?? [],
                        paused: pause?.isPaused == true,
                        showsThesis: false,
                        isDetail: true,
                        showsTracker: false
                    ) {
                        choice in
                        Task {
                            if await model?.vote(choice) == true {
                                toasts.show(success: "Vote recorded.")
                            } else if let message = model?.errorMessage {
                                toasts.current = MonacoToast(message: message)
                            }
                        }
                    }
                    .disabled(model?.isVoting == true)
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                    votes(detail)
                    reason(detail)
                    expected(summary)
                    status(summary)
                    if model?.canWithdraw == true {
                        Button("Withdraw proposal", role: .destructive) {
                            confirmingWithdrawal = true
                        }
                        .buttonStyle(.monacoDestructive)
                        .accessibilityIdentifier("proposal-withdraw")
                        .padding(.horizontal, MonacoTheme.Space.gutter)
                    }
                }
                .padding(.top, MonacoTheme.Space.m)
            } else if model?.errorMessage != nil {
                MonacoErrorRow(thing: "this proposal", identifier: "proposal-detail-error") {
                    Task { await model?.load() }
                }
            } else {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                    SkeletonBlock(width: 180, height: 24)
                    SkeletonBlock(width: 120, height: 28)
                    SkeletonBlock(width: 160, height: 12)
                }
                .padding(MonacoTheme.Space.m)
            }
        }
        .navigationTitle(title)
        .navigationBarTitleDisplayMode(.inline)
        .task {
            let model = preparedModel()
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
        .onScreenVisibilityChange {
            model?.setVisible($0)
            pause?.setVisible($0)
        }
        .onChange(of: model?.value?.summary.status) { old, new in
            showBurst = bursts(from: old, to: new)
        }
        .confirmationDialog("Withdraw this proposal?", isPresented: $confirmingWithdrawal, titleVisibility: .visible) {
            Button("Withdraw", role: .destructive) {
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
            .accessibilityIdentifier("proposal-withdraw-confirm")
            Button("Cancel", role: .cancel) {}
                .accessibilityIdentifier("proposal-withdraw-cancel")
        } message: {
            Text("Votes so far are dropped.")
        }
    }

    private var title: String {
        guard let summary = model?.summary else { return "Proposal" }
        let ticker = AssetSymbolFormatter.display(summary.symbol, kind: model?.asset?.kind ?? .stock)
        return "\(summary.kind == "sell" ? "Sell" : "Buy") \(ticker)"
    }

    private func votes(_ detail: ProposalDetail) -> some View {
        let groups = ProposalVoterGroups(voters: detail.voters, members: model?.members ?? [])
        let tally = detail.summary.tally
        let needed = max(tally.needed, 1)
        return VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            MonacoSectionHeader("Votes")
                .padding(.horizontal, MonacoTheme.Space.gutter)
            MonacoRule()
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                Text("\(groups.yes.count) yes · \(groups.no.count) no · \(groups.notVoted.count) not voted")
                    .font(MonacoTheme.Typo.calloutStrong)
                ProgressView(value: CGFloat(min(groups.yes.count, needed)), total: CGFloat(needed))
                    .tint(MonacoTheme.brandFill)
                    .accessibilityLabel("\(groups.yes.count) of \(tally.needed) yes votes to pass")
                HStack {
                    HStack(spacing: -10) {
                        ForEach(groups.summaryAvatars) { entry in
                            MonacoAvatar(
                                photoURL: entry.photoURL?.absoluteString, displayName: entry.name, size: 32,
                                seed: entry.id
                            )
                            .overlay { Circle().strokeBorder(MonacoTheme.canvas, lineWidth: 2) }
                        }
                    }
                    Spacer(minLength: MonacoTheme.Space.s)
                    NavigationLink {
                        ProposalVotersView(model: model)
                    } label: {
                        Text("See all")
                            .font(MonacoTheme.Typo.calloutStrong)
                            .foregroundStyle(MonacoTheme.brand)
                            .frame(minHeight: 44)
                            .contentShape(Rectangle())
                    }
                    .accessibilityIdentifier("proposal-votes-see-all")
                }
                .frame(height: 44)
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            MonacoRule()
        }
        .accessibilityIdentifier("proposal-votes")
    }

    @ViewBuilder private func reason(_ detail: ProposalDetail) -> some View {
        if let thesis = detail.summary.thesis, !thesis.isEmpty {
            ProposalReasonSection(title: detail.summary.kind == "sell" ? "Why sell" : "Why buy", text: thesis)
        }
    }

    @ViewBuilder private func expected(_ summary: ProposalSummary) -> some View {
        let isPending = summary.status == .open || summary.status == .passed
        if isPending, summary.swap?.status != "failed",
            let line = ProposalCardCopy.expected(
                isSell: summary.kind == "sell", quoteOut: summary.quoteOutAmount, usdcMicros: summary.usdcMicros,
                decimals: model?.asset?.decimals ?? AssetCatalogDefaults.decimals, kind: model?.asset?.kind ?? .stock)
        {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                MonacoSectionHeader("Expected")
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                MonacoGroupedList {
                    ReceiptLine(label: "Cabal gets", value: .words(line), isLast: true)
                }
            }
            .accessibilityIdentifier("proposal-expected")
        }
    }

    private func status(_ summary: ProposalSummary) -> some View {
        let stepper = ProposalStepper.make(
            status: summary.status, isSell: summary.kind == "sell", swapFailed: summary.swap?.status == "failed",
            expiresAt: summary.expiresAt, failureMessage: summary.swap?.failureMessage,
            statusMessage: summary.statusMessage, priceMoved: summary.swap?.isPriceMoved == true)
        return VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            MonacoSectionHeader("Status")
                .padding(.horizontal, MonacoTheme.Space.gutter)
            MonacoRule()
            ProposalStepperView(stepper: stepper)
                .padding(.horizontal, MonacoTheme.Space.gutter)
            MonacoRule()
            if let move = model?.priceMove, summary.swap?.isPriceMoved == true {
                priceMoved(move, summary)
            }
            if model?.canRetry(viewerID: environment.viewer?.userID) == true {
                Button(model?.retriesAtCurrentPrice == true ? "Buy at current price" : "Retry") {
                    Task {
                        await model?.retry()
                        if model?.didRetry == true {
                            toasts.show(success: "Retrying the trade.")
                        } else if let message = model?.errorMessage {
                            toasts.current = MonacoToast(message: message)
                        }
                    }
                }
                .buttonStyle(.monacoSecondary)
                .disabled(model?.isRetrying == true)
                .accessibilityIdentifier("proposal-retry")
            }
            if let swapID = summary.swap?.id {
                NavigationLink(value: AnyAppRoute(TransactionRoute(cabalID: summary.cabalID, transactionID: swapID))) {
                    Label("View transaction", systemImage: "arrow.up.right.square")
                        .font(MonacoTheme.Typo.calloutStrong)
                        .foregroundStyle(MonacoTheme.brand)
                        .frame(minHeight: 44)
                        .contentShape(Rectangle())
                }
            }
            if showBurst { ProposalCoinBurst() }
        }
    }

    private func priceMoved(_ move: ProposalPriceMove, _ summary: ProposalSummary) -> some View {
        let isSell = summary.kind == "sell"
        let decimals = model?.asset?.decimals ?? AssetCatalogDefaults.decimals
        let kind = model?.asset?.kind ?? .stock
        func words(_ quote: Int64) -> String {
            ProposalCardCopy.expected(
                isSell: isSell, quoteOut: quote, usdcMicros: summary.usdcMicros, decimals: decimals, kind: kind) ?? "-"
        }
        return MonacoGroupedList {
            ReceiptLine(label: "Voted for", value: .words(words(move.votedQuote)))
            ReceiptLine(label: "Now", value: .words(words(move.currentQuote)))
            ReceiptLine(label: "Price moved", value: .words("\(move.percentText) since the vote"), isLast: true)
        }
        .accessibilityIdentifier("proposal-price-moved")
    }

    private func bursts(from old: ProposalStatus?, to new: ProposalStatus?) -> Bool {
        guard let old, let new else { return false }
        return new == .executed && old != .executed && model?.value?.summary.kind == "buy"
    }

    private func preparedModel() -> ProposalDetailModel {
        if let model { return model }
        let repository = ProposalsRepository(api: environment.api)
        let created = ProposalDetailModel(id: proposalID, cabalID: "", repository: repository, hints: environment.hints)
        model = created
        return created
    }
}
