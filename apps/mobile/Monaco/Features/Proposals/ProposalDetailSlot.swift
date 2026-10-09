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
    @State private var priorStatus: ProposalStatus?
    @State private var showBurst = false
    @State private var confirmingWithdrawal = false

    init(proposalID: String, model: ProposalDetailModel? = nil) {
        self.proposalID = proposalID
        _model = State(initialValue: model)
    }

    var body: some View {
        Group {
            if let detail = model?.value {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.gutter) {
                    ProposalCard(
                        proposal: detail.summary, asset: model?.asset, members: model?.members ?? [],
                        paused: pause?.isPaused == true,
                        showsThesis: false,
                        isDetail: true
                    ) {
                        choice in
                        Task {
                            if await model?.vote(choice) == true {
                                toasts.show(success: "Vote in")
                            } else if let message = model?.errorMessage {
                                toasts.current = MonacoToast(message: message)
                            }
                        }
                    }
                    .disabled(model?.isVoting == true)
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                    if model?.canWithdraw == true {
                        Button("Withdraw proposal", role: .destructive) {
                            confirmingWithdrawal = true
                        }
                        .buttonStyle(.monacoDestructive)
                        .accessibilityIdentifier("proposal-withdraw")
                        .padding(.horizontal, MonacoTheme.Space.gutter)
                    }
                    votes(detail)
                    reason(detail)
                    expected(detail)
                    status(detail)
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
        .task {
            if model?.value != nil { return }
            let model = preparedModel()
            await model.load()
            guard let cabalID = model.value?.summary.cabalID else { return }
            async let detailHints: Void = model.observe()
            let pause = ProposalPauseModel(
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
            priorStatus = old
            showBurst = old == .open && new == .executed && model?.value?.summary.kind == "buy"
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
                    .lineLimit(1)
                    .minimumScaleFactor(0.8)
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
                        ProposalVotersView(groups: groups)
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

    @ViewBuilder private func expected(_ detail: ProposalDetail) -> some View {
        let summary = detail.summary
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

    private func status(_ detail: ProposalDetail) -> some View {
        let summary = detail.summary
        let stepper = ProposalStepper.make(
            status: summary.status, isSell: summary.kind == "sell", swapFailed: summary.swap?.status == "failed",
            expiresAt: summary.expiresAt, failureMessage: summary.swap?.failureMessage,
            statusMessage: summary.statusMessage)
        return VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            MonacoSectionHeader("Status")
                .padding(.horizontal, MonacoTheme.Space.gutter)
            MonacoRule()
            ProposalStepperView(stepper: stepper)
                .padding(.horizontal, MonacoTheme.Space.gutter)
            MonacoRule()
            if model?.retryableSwapID != nil {
                Button("Retry") {
                    Task {
                        await model?.retry()
                        if model?.didRetry == true {
                            toasts.show(success: "Trying the trade again.")
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
                }
            }
            if showBurst { ProposalCoinBurst() }
        }
    }

    private func preparedModel() -> ProposalDetailModel {
        if let model { return model }
        let repository = ProposalsRepository(api: environment.api)
        let created = ProposalDetailModel(id: proposalID, cabalID: "", repository: repository, hints: environment.hints)
        model = created
        return created
    }
}

#if DEBUG
enum ProposalDetailSampleScenario: String {
    case failedRetryable, failedFinal

    static func matching(_ arguments: [String]) -> Self? {
        guard let index = arguments.firstIndex(of: "-MonacoProposalDetailSample"), arguments.indices.contains(index + 1)
        else { return nil }
        return Self(rawValue: arguments[index + 1])
    }
}

final class ProposalDetailSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard let scenario = ProposalDetailSampleScenario.matching(arguments) else { return nil }
        return AnyView(ProposalDetailSampleHarness(scenario: scenario, auth: auth))
    }
}

private struct ProposalDetailSampleHarness: View {
    @State private var environment: AppEnvironment
    @State private var toasts = ToastCenter()
    @State private var model: ProposalDetailModel

    init(scenario: ProposalDetailSampleScenario, auth: PrivyAuthService) {
        let environment = AppEnvironment(
            auth: auth, hints: SilentSampleHints(), isAuthenticated: { true }, endAuthSession: {})
        _environment = State(initialValue: environment)
        _model = State(
            initialValue: ProposalDetailModel(
                sample: .failedSwap(retryable: scenario == .failedRetryable),
                members: Components.Schemas.Cabal.sampleWithMembers(role: "member").members, asset: .googl,
                repository: ProposalsRepository(api: environment.api), hints: environment.hints))
    }

    var body: some View {
        NavigationStack {
            ProposalDetailSlotView(proposalID: "proposal-1", model: model)
                .navigationDestination(for: AnyAppRoute.self) { $0.destination() }
        }
        .environment(environment)
        .environment(toasts)
    }
}

private nonisolated struct SilentSampleHints: HintConnecting {
    func hints(matching _: HintFilter) -> AsyncStream<Hint> { AsyncStream { $0.finish() } }
    func start() async {}
    func stop() async {}
}
#endif
