import MonacoAPI
import MonacoCabal
import MonacoCore
import SwiftUI

/// Fund this cabal: move account balance into one cabal's pot.
///
/// Owns the cabal's name, the fund in flight and its toasts. `FundCabalContent` is the layout.
struct FundCabalView: View {
    let cabalID: String

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(\.dismiss) private var dismiss
    @State private var cabal: CabalActionsModel?
    @State private var pauseState: CashOutModel?
    @State private var funding: Funding?
    @State private var amountText = ""
    @State private var confirmation: FundConfirmation?
    @State private var isVisible = false

    var body: some View {
        FundCabalContent(
            cabalID: cabalID,
            cabalName: cabal?.cabalName,
            state: environment.balance.state,
            amountText: $amountText,
            isSubmitting: funding?.isSubmitting ?? false,
            onSubmit: { Task { await fund() } },
            onRetry: { Task { await environment.balance.load() } },
            onAddMoney: { openDeposit(prefillMicros: nil) },
            onDeposit: { openDeposit(prefillMicros: $0) },
            pause: pauseState?.preview?.pause
        )
        .onAppear { isVisible = true }
        .onDisappear { isVisible = false }
        .sheet(item: $confirmation, onDismiss: { dismiss() }) { confirmation in
            FundConfirmedSheet(confirmation: confirmation)
        }
        .task {
            if funding == nil {
                funding = Funding(cabalID: cabalID, source: FundSource(api: environment.api), hints: environment.hints)
            }
            let cabal =
                self.cabal ?? CabalActionsModel(cabalID: cabalID, api: environment.api, hints: environment.hints)
            self.cabal = cabal
            await cabal.load()
        }
        .task {
            let model = pauseState ?? CashOutModel(cabalID: cabalID, api: environment.api, hints: environment.hints)
            pauseState = model
            await model.load()
            await model.observe()
        }
        .task {
            await environment.balance.load()
        }
        .onScreenVisibilityChange { visible in
            pauseState?.setVisible(visible)
        }
    }

    private func openDeposit(prefillMicros: Int64?) {
        environment.navigator.presentDeposit(prefillMicros: prefillMicros, cabalID: cabalID)
    }

    private func fund() async {
        guard let funding, let micros = AmountEntryText.micros(amountText), micros > 0 else { return }
        let cabalName = cabal?.cabalName
        switch await funding.submit(micros: micros) {
        case .accepted:
            Haptics.success()
            if isVisible {
                confirmation = FundConfirmation(
                    amount: UsdAmountFormatter.format(micros: micros), cabalName: cabalName ?? "your cabal")
            } else {
                if let line = funding.progress.toast(cabalName: cabalName) { toasts.show(success: line) }
                dismiss()
            }
            Task { [toasts] in
                let settled = await funding.settle()
                guard settled.isSettled, let line = settled.toast(cabalName: cabalName) else { return }
                if case .settled = settled {
                    toasts.show(success: line)
                } else {
                    toasts.current = MonacoToast(message: line)
                }
            }
        case .refused(.needsMoney(let message)):
            let shortfall = max(micros - (environment.balance.balance?.availableMicros ?? 0), 0)
            toasts.current = MonacoToast(
                message: message,
                action: MonacoToastAction(title: "Add money") { openDeposit(prefillMicros: shortfall) })
        case .refused(.toast(let message)), .unconfirmed(let message):
            toasts.current = MonacoToast(message: message)
        }
    }
}

/// Which of its states Fund this cabal is in. The amount pad and its button belong together:
/// both show in `.amount` and nowhere else, so a reload never takes one away without the other.
enum FundCabalStage: Equatable {
    /// Nothing to show yet: the first balance read is still running.
    case loading
    /// The first read failed, so there is nothing to keep on screen.
    case failed
    /// Nothing available and nothing on its way: the member has to add money first.
    case needsMoney
    case amount(AccountBalance)

    static func resolve(state: LoadState<AccountBalance>) -> FundCabalStage {
        switch state {
        case .idle, .loading:
            return .loading
        case .failed:
            return .failed
        case .loaded(let balance):
            if balance.availableMicros == 0 && balance.inFlightMicros == 0 { return .needsMoney }
            return .amount(balance)
        }
    }

    var showsAmountEntry: Bool {
        if case .amount = self { return true }
        return false
    }

    var showsDepositAction: Bool { showsAmountEntry }

    var showsAddMoneyCTA: Bool { self == .needsMoney }
}

/// What Fund this cabal says and allows for the amount typed against the balance it has.
struct FundCabalForm: Equatable {
    static let overBalance = "Not enough in your account balance."
    static let insufficientNotice = "Not enough cash. Reduce the amount or deposit into your account."
    static let minimumMicros: Int64 = 1_000_000
    static let belowMinimum = "The minimum is $1."

    let amountText: String
    let availableMicros: Int64?
    let inFlightMicros: Int64

    init(amountText: String, balance: AccountBalance?) {
        self.amountText = amountText
        availableMicros = balance?.availableMicros
        inFlightMicros = balance?.inFlightMicros ?? 0
    }

    /// The most the pad takes: the whole balance, $0 when it is all on its way to a cabal. Nil until it loads.
    var maxDollars: Decimal? {
        guard let availableMicros else { return nil }
        return Decimal(availableMicros) / Decimal(1_000_000)
    }

    var canSubmit: Bool {
        guard let micros = AmountEntryText.micros(amountText), micros > 0, availableMicros != nil else { return false }
        return problem == nil
    }

    /// The button reads the amount, so the member sees what they are about to send.
    var ctaTitle: String {
        guard let value = AmountEntryText.decimal(amountText), value > 0 else { return "Enter an amount" }
        let amount = UsdAmountFormatter.format(decimal: value)
        return "Deposit \(amount)"
    }

    /// The line under the figure: what there is to fund with, and what is still on its way out (funds and withdrawals).
    var availability: String? {
        guard let availableMicros else { return nil }
        let available = "\(UsdAmountFormatter.format(flooredMicros: availableMicros)) available"
        guard inFlightMicros > 0 else { return available }
        return "\(available) · \(UsdAmountFormatter.format(micros: inFlightMicros)) pending"
    }

    var cashLine: String? {
        guard let availableMicros else { return nil }
        return MoneyCashLine.text(availableMicros: availableMicros, inFlightMicros: inFlightMicros)
    }

    var isOverBalance: Bool { problem == Self.overBalance }

    var shortfallMicros: Int64? {
        guard let micros = AmountEntryText.micros(amountText), let availableMicros, micros > availableMicros else {
            return nil
        }
        return micros - availableMicros
    }

    var noticeText: String? {
        guard let problem else { return nil }
        return isOverBalance ? Self.insufficientNotice : problem
    }

    /// Replaces the helper, in red, while the amount typed is over the balance or under the minimum.
    var problem: String? {
        guard let micros = AmountEntryText.micros(amountText) else { return nil }
        if let availableMicros, micros > availableMicros { return Self.overBalance }
        if micros > 0, micros < Self.minimumMicros { return Self.belowMinimum }
        return nil
    }
}

struct FundConfirmation: Identifiable, Equatable {
    let id = UUID()
    let amount: String
    let cabalName: String
}

struct FundConfirmedSheet: View {
    let confirmation: FundConfirmation

    @Environment(\.dismiss) private var dismiss

    var body: some View {
        VStack(spacing: MonacoTheme.Space.l) {
            OutcomeView(
                tone: .done, title: "Deposit confirmed",
                message: "\(confirmation.amount) added to your share in \(confirmation.cabalName)")
            Button("Done") { dismiss() }
                .buttonStyle(.monacoPrimary)
                .accessibilityIdentifier("fund-cabal-done-button")
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.m)
        .frame(maxHeight: .infinity, alignment: .top)
        .monacoSheet(title: "Deposit confirmed", detents: .result, showsClose: true)
        .accessibilityIdentifier("fund-cabal-success-sheet")
    }
}

struct FundCabalContent: View {
    let cabalID: String
    var cabalName: String?
    let state: LoadState<AccountBalance>
    @Binding var amountText: String
    let isSubmitting: Bool
    let onSubmit: () -> Void
    let onRetry: () -> Void
    let onAddMoney: () -> Void
    var onDeposit: (Int64?) -> Void = { _ in }
    var pause: CabalPause?

    private var stage: FundCabalStage {
        .resolve(state: state)
    }

    private var form: FundCabalForm {
        if case .loaded(let balance) = state {
            return FundCabalForm(amountText: amountText, balance: balance)
        }
        return FundCabalForm(amountText: amountText, balance: nil)
    }

    var body: some View {
        ScrollView {
            stageContent
                .padding(.top, stage.showsAmountEntry ? MonacoTheme.Space.s : MonacoTheme.Space.xl)
                .padding(.bottom, MonacoTheme.Space.s)
        }
        .scrollBounceBehavior(.basedOnSize)
        .monacoCanvas()
        .safeAreaInset(edge: .bottom) {
            if stage.showsAmountEntry {
                dock
            }
        }
        .navigationBarBackButtonHidden(isSubmitting)
        .navigationTitle(cabalName.map { "Deposit to \($0)" } ?? "Deposit to cabal")
        .navigationBarTitleDisplayMode(.inline)
        .accessibilityIdentifier("fund-cabal-view")
    }

    private var dock: some View {
        BottomCTA {
            VStack(spacing: MonacoTheme.Space.sm) {
                AmountKeypad(amountText: $amountText)
                Button(action: onSubmit) {
                    SubmitLabel(isWorking: isSubmitting, idle: form.ctaTitle, working: "Processing…")
                }
                .buttonStyle(.monacoPrimary)
                .disabled(!form.canSubmit || isSubmitting)
                .accessibilityIdentifier("fund-cabal-submit-button")
                if form.isOverBalance {
                    Button("Deposit to account") { onDeposit(form.shortfallMicros) }
                        .buttonStyle(.monacoOutline)
                        .disabled(isSubmitting)
                        .accessibilityIdentifier("fund-cabal-deposit-button")
                }
            }
        }
    }

    @ViewBuilder
    private var stageContent: some View {
        switch stage {
        case .loading:
            AmountEntrySkeleton()
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .accessibilityIdentifier("fund-cabal-loading")
        case .failed:
            MoneyLoadFailure(identifier: "fund-cabal-balance-error", retry: onRetry)
        case .needsMoney:
            EmptyState(
                title: "Add money first",
                message: "Send USDC to your deposit address, then fund this cabal.",
                actionTitle: "Add money",
                actionIdentifier: "fund-cabal-add-money-button",
                systemImage: "wallet.bifold",
                action: onAddMoney
            )
            .accessibilityIdentifier("fund-cabal-needs-money")
        case .amount:
            amountEntry
        }
    }

    private var amountEntry: some View {
        VStack(spacing: MonacoTheme.Space.sm) {
            contextCard
            if let pause {
                CabalPauseRow(pause: pause)
            }
            AmountEntry(
                amountText: $amountText,
                max: form.maxDollars,
                presets: [.dollars(25), .dollars(50), .dollars(100), .fraction(1, label: "Max")],
                caption: "Deposit amount",
                overLimitHelper: "",
                input: .keypad
            )
            notices
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
    }

    private var contextCard: some View {
        MonacoCard(style: .tight) {
            HStack(spacing: MonacoTheme.Space.sm) {
                CabalCover(
                    cabalId: cabalID, tint: .forGroupId(cabalID), pictureURL: nil, style: .thumb(34)
                )
                .frame(width: 34, height: 34)
                .accessibilityHidden(true)
                VStack(alignment: .leading, spacing: 0) {
                    Text(cabalName ?? "This cabal")
                        .font(MonacoTheme.Typo.headline)
                        .foregroundStyle(MonacoTheme.ink)
                    if let cashLine = form.cashLine {
                        Text(cashLine)
                            .font(MonacoTheme.Typo.caption)
                            .foregroundStyle(MonacoTheme.muted)
                            .accessibilityIdentifier("fund-cabal-cash-line")
                            .monacoPrivate()
                    }
                }
                .frame(maxWidth: .infinity, alignment: .leading)
            }
            .accessibilityElement(children: .combine)
        }
    }

    @ViewBuilder
    private var notices: some View {
        if let notice = form.noticeText {
            MonacoNotice(.error, message: notice)
        }
        if isSubmitting {
            MonacoNotice(.warn, message: "Processing your deposit…")
        }
        MonacoNotice(.info, message: CabalYield.fundNote)
            .accessibilityIdentifier("fund-cabal-yield-note")
    }
}
