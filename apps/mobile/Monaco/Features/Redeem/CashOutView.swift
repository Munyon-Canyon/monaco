import MonacoAPI
import MonacoCore
import SwiftUI

struct CashOutView: View {
    let cabalID: String

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(\.dismiss) private var dismiss
    @State private var model: CashOutModel?
    @State private var amountText = ""

    var body: some View {
        CashOutContent(
            model: model,
            runningJob: environment.cashOuts.job(for: cabalID),
            amountText: $amountText,
            onSubmit: { Task { await submit() } }
        )
        .task {
            let model = self.model ?? CashOutModel(cabalID: cabalID, api: environment.api, hints: environment.hints)
            self.model = model
            await model.load()
            await model.observe()
        }
        .onScreenVisibilityChange { model?.setVisible($0) }
    }

    private func submit() async {
        guard let model else { return }
        switch await model.submit(enteredMicros: AmountEntryText.micros(amountText) ?? 0) {
        case .started(let job):
            environment.cashOuts.track(job)
            Haptics.success()
            toasts.show(success: job.startedToast)
            dismiss()
        case .refused(let message):
            toasts.current = MonacoToast(message: message)
        case nil:
            break
        }
    }
}

struct CashOutContent: View {
    let model: CashOutModel?
    let runningJob: CashOutJob?
    @Binding var amountText: String
    let onSubmit: () -> Void

    private var enteredMicros: Int64 { AmountEntryText.micros(amountText) ?? 0 }

    var body: some View {
        ScrollView {
            content
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .padding(.top, MonacoTheme.Space.s)
                .padding(.bottom, MonacoTheme.Space.s)
        }
        .scrollBounceBehavior(.basedOnSize)
        .monacoCanvas()
        .safeAreaInset(edge: .bottom) {
            if runningJob == nil, let preview = model?.preview, canEnterAmount(preview) {
                submitBar(preview)
            }
        }
        .navigationBarBackButtonHidden(model?.isSubmitting ?? false)
        .navigationTitle("Cash out")
        .navigationBarTitleDisplayMode(.inline)
    }

    @ViewBuilder private var content: some View {
        if let runningJob, let progress = runningJob.progress {
            VStack(spacing: MonacoTheme.Space.m) {
                AmountEntrySkeleton()
                Text(progress)
                    .font(MonacoTheme.Typo.rowTitle)
                    .foregroundStyle(MonacoTheme.ink)
            }
            .frame(maxWidth: .infinity)
            .padding(.top, MonacoTheme.Space.xl)
            .accessibilityElement(children: .combine)
            .accessibilityIdentifier("cash-out-progress")
        } else {
            switch model?.state ?? .loading {
            case .idle, .loading:
                AmountEntrySkeleton()
                    .accessibilityIdentifier("cash-out-loading")
            case .failed:
                MonacoErrorRow(thing: "your slice", identifier: "cash-out-error") {
                    Task { await model?.load() }
                }
            case .loaded(let preview):
                loaded(preview)
            }
        }
    }

    private func loaded(_ preview: CashOutPreview) -> some View {
        VStack(spacing: MonacoTheme.Space.l) {
            if let pause = preview.pause {
                CabalPauseRow(pause: pause)
            }
            if !preview.hasStake {
                EmptyState(
                    title: "Nothing to cash out yet",
                    message: "Add money to this cabal first. Your slice shows up here."
                )
                .accessibilityIdentifier("cash-out-empty")
            } else if preview.sliceIsBelowMinimum {
                EmptyState(
                    title: CashOutAmountRule.tooSmall,
                    message:
                        "Your slice is worth \(UsdAmountFormatter.format(micros: preview.sliceMicros)). "
                        + "Cash out starts at \(UsdAmountFormatter.format(micros: preview.minMicros)), "
                        + "so this one has to grow first."
                )
                .accessibilityIdentifier("cash-out-below-minimum")
            } else {
                amountEntry(preview)
            }
        }
    }

    private func amountEntry(_ preview: CashOutPreview) -> some View {
        let verdict = preview.verdict(enteredMicros: enteredMicros)
        return AmountEntry(
            amountText: $amountText,
            max: AmountEntryText.dollars(micros: preview.sliceMicros),
            presets: [
                .fraction(0.25, label: "25%"),
                .fraction(0.5, label: "50%"),
                .fraction(1, label: "All"),
            ],
            helper: CashOutAmountRule.helper(for: verdict, sliceMicros: preview.sliceMicros),
            overLimitHelper: "More than your slice",
            problem: CashOutAmountRule.problem(for: verdict),
            input: .keypad
        ) {
            AmountEntryNote(CashOutAmountRule.explainer(for: verdict))
                .accessibilityIdentifier("cash-out-explainer")
        }
        .accessibilityIdentifier("cash-out-amount")
    }

    private func canEnterAmount(_ preview: CashOutPreview) -> Bool {
        preview.hasStake && !preview.sliceIsBelowMinimum
    }

    private func submitBar(_ preview: CashOutPreview) -> some View {
        let verdict = preview.verdict(enteredMicros: enteredMicros)
        let isSubmitting = model?.isSubmitting ?? false
        return BottomCTA {
            Button(action: onSubmit) {
                HStack(spacing: MonacoTheme.Space.s) {
                    if isSubmitting {
                        ProgressView().tint(MonacoTheme.primaryButtonLabel)
                    }
                    Text(
                        CashOutAmountRule.submitTitle(
                            for: verdict, enteredMicros: enteredMicros, sliceMicros: preview.sliceMicros))
                }
                .frame(maxWidth: .infinity)
            }
            .buttonStyle(.monacoPrimary)
            .disabled(isSubmitting || !verdict.maySubmit || preview.pause != nil)
            .accessibilityIdentifier("cash-out-submit")
        }
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

    private var runningJob: CashOutJob? {
        mode == "selling"
            ? CashOutJob(id: "j", cabalID: "c", status: .selling, payoutMicros: 1_000_000, resultCode: nil) : nil
    }

    var body: some View {
        NavigationStack {
            CashOutContent(model: model, runningJob: runningJob, amountText: $amountText, onSubmit: {})
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
