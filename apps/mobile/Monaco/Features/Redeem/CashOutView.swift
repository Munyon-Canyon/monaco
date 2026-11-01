import MonacoAPI
import MonacoAnalytics
import MonacoCore
import SwiftUI

enum CashOutStep: Equatable {
    case entry
    case review
}

struct CashOutView: View {
    let cabalID: String

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @State private var model: CashOutModel?
    @State private var cabal: CabalActionsModel?
    @State private var amountText = ""
    @State private var step = CashOutStep.entry
    @State private var startedJob: CashOutJob?

    var body: some View {
        CashOutContent(
            model: model,
            cabalName: cabal?.cabalName,
            runningJob: environment.cashOuts.job(for: cabalID),
            startedJob: startedJob,
            settledNotice: settledNotice,
            amountText: $amountText,
            step: step,
            onReview: { step = .review },
            onBack: { step = .entry },
            onSubmit: { Task { await submit() } },
            onFund: { environment.navigator.open(FundRoute(cabalID: cabalID), in: environment.navigator.selectedTab) },
            onActivity: { environment.navigator.open(AccountActivityRoute(), in: environment.navigator.selectedTab) }
        )
        .analyticsScreen("cash_out", step: .cashOut(.cashOutOpened))
        .task {
            let model =
                self.model
                ?? CashOutModel(
                    cabalID: cabalID, api: environment.api, hints: environment.hints,
                    submission: environment.moneySubmission(slot: CashOutModel.slot(cabalID: cabalID)))
            self.model = model
            await model.load()
            await model.observe()
        }
        .task {
            let cabal =
                self.cabal ?? CabalActionsModel(cabalID: cabalID, api: environment.api, hints: environment.hints)
            self.cabal = cabal
            await cabal.load()
        }
        .onScreenVisibilityChange { model?.setVisible($0) }
    }

    private var settledNotice: CashOutNotice? {
        guard let startedJob, let notice = environment.cashOuts.notice, notice.jobID == startedJob.id else {
            return nil
        }
        return notice
    }

    private func submit() async {
        guard let model else { return }
        switch await model.submit(enteredMicros: AmountEntryText.micros(amountText) ?? 0) {
        case .started(let job):
            environment.cashOuts.track(job)
            Haptics.success()
            toasts.show(success: job.startedToast)
            startedJob = job
        case .refused(let message):
            toasts.current = MonacoToast(message: message)
            step = .review
        case nil:
            break
        }
    }
}

struct CashOutContent: View {
    let model: CashOutModel?
    var cabalName: String?
    var failure: APIError?
    let runningJob: CashOutJob?
    var startedJob: CashOutJob?
    var settledNotice: CashOutNotice?
    @Binding var amountText: String
    var step = CashOutStep.entry
    var onReview: () -> Void = {}
    var onBack: () -> Void = {}
    let onSubmit: () -> Void
    let onFund: () -> Void
    var onActivity: () -> Void = {}

    private var enteredMicros: Int64 { AmountEntryText.micros(amountText) ?? 0 }

    private var state: LoadState<CashOutPreview> {
        if let failure { return .failed(failure) }
        return model?.state ?? .loading
    }

    private var isSubmitting: Bool { model?.isSubmitting ?? false }

    var body: some View {
        Group {
            if let startedJob {
                outcome(startedJob)
            } else {
                form
            }
        }
        .navigationTitle("Cash out your share")
        .navigationBarTitleDisplayMode(.inline)
    }

    private var form: some View {
        ScrollView {
            content
                .padding(.top, MonacoTheme.Space.s)
                .padding(.bottom, MonacoTheme.Space.s)
        }
        .scrollBounceBehavior(.basedOnSize)
        .monacoCanvas()
        .safeAreaInset(edge: .bottom) {
            if let preview = model?.preview, failure == nil, Self.canEnterAmount(preview) {
                dock(preview)
            }
        }
        .navigationBarBackButtonHidden(isSubmitting || step == .review)
        .toolbar {
            if step == .review, !isSubmitting {
                ToolbarItem(placement: .topBarLeading) {
                    MonacoGlassButton(
                        systemImage: "chevron.left", accessibilityLabel: "Back", placement: .toolbar, action: onBack)
                }
            }
        }
    }

    @ViewBuilder private var content: some View {
        switch state {
        case .idle, .loading:
            AmountEntrySkeleton()
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .accessibilityIdentifier("cash-out-loading")
        case .failed:
            MoneyLoadFailure(identifier: "cash-out-error") {
                Task { await model?.load() }
            }
        case .loaded(let preview):
            loaded(preview)
        }
    }

    private func loaded(_ preview: CashOutPreview) -> some View {
        VStack(spacing: MonacoTheme.Space.sm) {
            if !preview.hasStake {
                EmptyState(
                    title: "Nothing to cash out yet",
                    message: "Fund this cabal first. Your share shows up here.",
                    actionTitle: "Fund this cabal",
                    systemImage: "tray",
                    action: onFund
                )
                .accessibilityIdentifier("cash-out-empty")
            } else if preview.sliceIsBelowMinimum {
                EmptyState(
                    title: CashOutAmountRule.tooSmall,
                    message: Self.belowMinimumMessage(preview)
                )
                .accessibilityIdentifier("cash-out-below-minimum")
            } else if step == .review {
                shareStat(preview)
                review(preview)
            } else {
                MonacoNotice(
                    .info,
                    message:
                        "Cash out of this cabal to your cash balance. External wallet withdrawal is a separate step."
                )
                .accessibilityIdentifier("cash-out-explainer")
                if let pause = preview.pause {
                    CabalPauseRow(pause: pause)
                }
                shareStat(preview)
                amountEntry(preview)
                entryNotices(preview)
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
    }

    static let finalProceedsValue = "Known when it settles"

    static let reviewNotice =
        "Selling assets to fill this request can change what you receive. "
        + "The final amount is known when the request settles, and no arrival time is promised. "
        + "You stay in the cabal."

    static func belowMinimumMessage(_ preview: CashOutPreview) -> String {
        "Your slice is worth \(UsdAmountFormatter.format(flooredMicros: preview.sliceMicros)). "
            + "Cash out starts at \(UsdAmountFormatter.format(micros: preview.minMicros)), "
            + "so this one has to grow first."
    }

    private func shareStat(_ preview: CashOutPreview) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            Text("Your share in \(cabalName ?? "this cabal")")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
            MoneyText(micros: UsdAmountFormatter.flooredToCents(preview.sliceMicros), style: .card)
                .foregroundStyle(MonacoTheme.ink)
                .accessibilityIdentifier("cash-out-share-value")
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.vertical, MonacoTheme.Space.m)
        .overlay(alignment: .top) { MonacoRule() }
        .overlay(alignment: .bottom) { MonacoRule() }
        .accessibilityElement(children: .combine)
    }

    private func amountEntry(_ preview: CashOutPreview) -> some View {
        let verdict = preview.verdict(enteredMicros: enteredMicros)
        let slice = UsdAmountFormatter.format(flooredMicros: preview.sliceMicros)
        return AmountEntry(
            amountText: $amountText,
            max: AmountEntryText.dollars(micros: preview.sliceMicros),
            presets: [
                .fraction(0.25, label: "25%"),
                .fraction(0.5, label: "50%"),
                .fraction(1, label: "All"),
            ],
            caption: "Cash-out request",
            helper: verdict == .sellsWholeSlice
                ? CashOutAmountRule.explainer(for: verdict) : "\(slice) current share value",
            overLimitHelper: "",
            input: .keypad
        )
        .accessibilityIdentifier("cash-out-amount")
    }

    @ViewBuilder
    private func entryNotices(_ preview: CashOutPreview) -> some View {
        let verdict = preview.verdict(enteredMicros: enteredMicros)
        if verdict == .overSlice {
            MonacoNotice(.error, message: "The request exceeds your current share value.")
        } else if let problem = CashOutAmountRule.problem(for: verdict, minimumMicros: preview.minMicros) {
            MonacoNotice(.error, message: problem)
                .accessibilityIdentifier("cash-out-below-minimum")
        }
    }

    private func review(_ preview: CashOutPreview) -> some View {
        let verdict = preview.verdict(enteredMicros: enteredMicros)
        let requested = CashOutAmountRule.effectiveMicros(
            for: verdict, enteredMicros: enteredMicros, sliceMicros: preview.sliceMicros)
        return VStack(spacing: MonacoTheme.Space.sm) {
            MoneyKeyRowCard(rows: [
                MoneyKeyRow(
                    label: "Requested amount", value: .data(UsdAmountFormatter.format(flooredMicros: requested))),
                MoneyKeyRow(label: "Destination", value: .words("Your own cash")),
                MoneyKeyRow(label: "Final proceeds", value: .words(Self.finalProceedsValue)),
            ])
            MonacoNotice(.warn, message: Self.reviewNotice)
        }
    }

    static func canEnterAmount(_ preview: CashOutPreview) -> Bool {
        preview.pause == nil && preview.hasStake && !preview.sliceIsBelowMinimum
    }

    private func dock(_ preview: CashOutPreview) -> some View {
        BottomCTA {
            VStack(spacing: MonacoTheme.Space.s) {
                if step == .entry {
                    AmountKeypad(amountText: $amountText)
                    Button("Review cash-out", action: onReview)
                        .buttonStyle(.monacoPrimary)
                        .disabled(!(model?.maySubmit(enteredMicros: enteredMicros) ?? false))
                        .accessibilityIdentifier("cash-out-review")
                } else {
                    Button(action: onSubmit) {
                        SubmitLabel(isWorking: isSubmitting, idle: "Request cash-out", working: "Requesting…")
                    }
                    .buttonStyle(.monacoPrimary)
                    .disabled(isSubmitting || !(model?.maySubmit(enteredMicros: enteredMicros) ?? false))
                    .accessibilityIdentifier("cash-out-submit")
                }
            }
        }
    }

    @ViewBuilder
    private func outcome(_ job: CashOutJob) -> some View {
        let rows = { (status: String) in
            [
                MoneyKeyRow(label: "From", value: .words(cabalName ?? "This cabal")),
                MoneyKeyRow(label: "To", value: .words("Your cash")),
                MoneyKeyRow(
                    label: "Requested value", value: .data(UsdAmountFormatter.format(flooredMicros: job.payoutMicros))),
                MoneyKeyRow(label: "Status", value: .words(status)),
            ]
        }
        if let settledNotice, let message = settledNotice.partialMessage(requestedMicros: job.payoutMicros),
            let raised = settledNotice.raisedMicros
        {
            MoneyOutcome(
                tone: .pending, title: "Cash-out partly done", message: message,
                rows: partialRows(job, raisedMicros: raised), identifier: "cash-out-progress"
            ) {
                Button("View activity", action: onActivity).buttonStyle(.monacoPrimary)
            }
        } else if let settledNotice, settledNotice.isSuccess {
            MoneyOutcome(
                tone: .done, title: "Cash-out complete",
                message: "Settled proceeds are now in your cash balance.", rows: rows("Completed"),
                identifier: "cash-out-progress"
            ) {
                Button("View activity", action: onActivity).buttonStyle(.monacoPrimary)
            }
        } else if let settledNotice {
            MoneyOutcome(
                tone: .error, title: "Cash-out didn't go through", message: settledNotice.message, rows: rows("Failed"),
                identifier: "cash-out-progress"
            ) {
                Button("View activity", action: onActivity).buttonStyle(.monacoPrimary)
            }
        } else {
            MoneyOutcome(
                tone: .pending, title: "Cash-out requested",
                message: "Your share redemption is processing. Final proceeds will appear when it settles.",
                rows: rows("Processing"),
                notice: "Timing and final proceeds depend on the cabal's holdings and execution.",
                identifier: "cash-out-progress"
            ) {
                Button("View activity", action: onActivity).buttonStyle(.monacoPrimary)
            }
        }
    }
}

extension CashOutContent {
    private func partialRows(_ job: CashOutJob, raisedMicros: Int64) -> [MoneyKeyRow] {
        [
            MoneyKeyRow(label: "From", value: .words(cabalName ?? "This cabal")),
            MoneyKeyRow(label: "To", value: .words("Your cash")),
            MoneyKeyRow(
                label: "Requested value", value: .data(UsdAmountFormatter.format(flooredMicros: job.payoutMicros))),
            MoneyKeyRow(label: "Cashed out", value: .data(UsdAmountFormatter.format(flooredMicros: raisedMicros))),
            MoneyKeyRow(label: "Status", value: .words("Partly done")),
        ]
    }
}

#if DEBUG
final class CashOutSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard let flag = arguments.firstIndex(of: "-cashOutHarness") else { return nil }
        let mode = arguments.indices.contains(flag + 1) ? arguments[flag + 1] : "slice"
        let amount = arguments.indices.contains(flag + 2) ? arguments[flag + 2] : ""
        return AnyView(CashOutHarnessScreen(mode: mode, amountText: amount))
    }
}

private struct CashOutHarnessScreen: View {
    let mode: String
    @State var amountText: String

    @State private var model: CashOutModel?

    private var preview: Components.Schemas.CashOutPreview {
        switch mode {
        case "none": .sampleNoStake
        case "paused": .sampleOpsPause
        case "deposit": .sampleDepositPause
        default: .sample
        }
    }

    private var step: CashOutStep { mode == "review" ? .review : .entry }

    private var startedJob: CashOutJob? {
        switch mode {
        case "selling", "processing", "completed":
            CashOutJob(
                id: "j", cabalID: "c", status: mode == "completed" ? .completed : .selling, payoutMicros: 250_000_000,
                resultCode: nil)
        default: nil
        }
    }

    private var settledNotice: CashOutNotice? {
        mode == "completed" ? startedJob?.outcome : nil
    }

    private var failure: APIError? {
        mode == "error" ? .transport(URLError(.notConnectedToInternet)) : nil
    }

    var body: some View {
        NavigationStack {
            CashOutContent(
                model: model, cabalName: "Big Ballers", failure: failure, runningJob: nil, startedJob: startedJob,
                settledNotice: settledNotice, amountText: $amountText, step: step, onSubmit: {}, onFund: {}
            )
            .task {
                let created = model ?? CashOutModel.preview(preview)
                model = created
                await created.load()
            }
        }
        .tint(MonacoTheme.ink)
    }
}
#endif
