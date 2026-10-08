import MonacoAPI
import MonacoCabal
import MonacoCore
import SwiftUI

/// Fund this cabal: move account balance into one cabal's pot.
///
/// Owns the balance, the cabal's name, the fund in flight and its toasts. `FundCabalContent` is the layout.
struct FundCabalView: View {
    let cabalID: String

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(\.dismiss) private var dismiss
    @State private var balanceSource: BalanceSource?
    @State private var cabal: CabalActionsModel?
    @State private var funding: Funding?
    @State private var amountText = ""

    var body: some View {
        FundCabalContent(
            state: balanceSource?.state ?? .loading,
            cabalName: cabal?.cabalName,
            amountText: $amountText,
            isSubmitting: funding?.isSubmitting ?? false,
            onSubmit: { Task { await fund() } },
            onRetry: { Task { await balanceSource?.load() } },
            onAddMoney: { openDeposit(prefillMicros: nil) }
        )
        .onChange(of: balanceSource?.failureTick) { _, _ in
            guard balanceSource?.balance != nil, let error = balanceSource?.lastError else { return }
            toasts.current = MonacoToast(message: BalanceSource.message(for: error))
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
            let source = balanceSource ?? BalanceSource(api: environment.api, hints: environment.hints)
            balanceSource = source
            await source.load()
            await source.observe()
        }
        .onScreenVisibilityChange { visible in
            balanceSource?.setVisible(visible)
        }
    }

    private func openDeposit(prefillMicros: Int64?) {
        environment.navigator.open(
            DepositRoute(prefillMicros: prefillMicros, cabalID: cabalID), in: environment.navigator.selectedTab)
    }

    private func fund() async {
        guard let funding, let micros = AmountEntryText.micros(amountText), micros > 0 else { return }
        let cabalName = cabal?.cabalName
        switch await funding.submit(micros: micros) {
        case .accepted:
            if let line = funding.progress.toast(cabalName: cabalName) { toasts.show(success: line) }
            dismiss()
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
            let shortfall = max(micros - (balanceSource?.balance?.availableMicros ?? 0), 0)
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
}

/// What Fund this cabal says and allows for the amount typed against the balance it has.
struct FundCabalForm: Equatable {
    static let overBalance = "Not enough in your account balance."

    let amountText: String
    let availableMicros: Int64?
    let inFlightMicros: Int64

    init(amountText: String, balance: AccountBalance?) {
        self.amountText = amountText
        availableMicros = balance?.availableMicros
        inFlightMicros = balance?.inFlightMicros ?? 0
    }

    /// The most the pad takes: the whole balance. Nil while there is nothing to fund with.
    var maxDollars: Decimal? {
        guard let availableMicros, availableMicros > 0 else { return nil }
        return Decimal(availableMicros) / Decimal(1_000_000)
    }

    var canSubmit: Bool {
        guard let micros = AmountEntryText.micros(amountText), micros > 0, availableMicros != nil else { return false }
        return problem == nil
    }

    /// The button reads the amount, so the member sees what they are about to send.
    var ctaTitle: String {
        guard let value = AmountEntryText.decimal(amountText), value > 0 else { return "Add money" }
        let isWhole = (AmountEntryText.micros(amountText) ?? 0) % 1_000_000 == 0
        let amount = isWhole ? AmountEntryText.display(amountText) : UsdAmountFormatter.format(decimal: value)
        return "Add \(amount) to the pot"
    }

    /// The line under the figure: what there is to fund with, and what is already on its way.
    var availability: String? {
        guard let availableMicros else { return nil }
        let available = "\(UsdAmountFormatter.format(flooredMicros: availableMicros)) available"
        guard inFlightMicros > 0 else { return available }
        return "\(available) · \(UsdAmountFormatter.format(micros: inFlightMicros)) funding"
    }

    /// Replaces the helper, in red, while the amount typed is more than the balance holds.
    var problem: String? {
        guard let availableMicros, let micros = AmountEntryText.micros(amountText), micros > availableMicros else {
            return nil
        }
        return Self.overBalance
    }

    /// What funding does, under the pad. Cash out's says the same thing the other way round.
    static func note(into cabalName: String?) -> String {
        guard let cabalName, !cabalName.isEmpty else {
            return "The money leaves your account balance and joins the pot. Your slice grows by the same amount."
        }
        return
            "The money leaves your account balance and joins the \(cabalName) pot. Your slice grows by the same amount."
    }

    static let treasuryNote =
        "To add money to this cabal, use Fund. Sending USDC straight to the treasury will be returned and pauses the cabal's trading."
}

/// Fund this cabal's layout: the amount as the hero, the balance it comes out of, and a button
/// that reads the amount.
///
/// Pure: what the screen knows comes in, what the member does goes out.
struct FundCabalContent: View {
    let state: LoadState<AccountBalance>
    let cabalName: String?
    @Binding var amountText: String
    let isSubmitting: Bool
    let onSubmit: () -> Void
    let onRetry: () -> Void
    let onAddMoney: () -> Void

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
                // The figure starts where Cash out's does, so the two read as one pair.
                .padding(.top, MonacoTheme.Space.xl)
                .padding(.bottom, MonacoTheme.Space.xl)
        }
        .scrollDismissesKeyboard(.interactively)
        .monacoCanvas()
        .safeAreaInset(edge: .bottom) {
            if stage.showsAmountEntry {
                BottomCTA {
                    Button(action: onSubmit) {
                        HStack(spacing: MonacoTheme.Space.s) {
                            if isSubmitting {
                                ProgressView().tint(MonacoTheme.primaryButtonLabel)
                                Text("Adding…")
                            } else {
                                Text(form.ctaTitle)
                            }
                        }
                        .frame(maxWidth: .infinity)
                    }
                    .buttonStyle(.monacoPrimary)
                    .disabled(!form.canSubmit || isSubmitting)
                    .accessibilityIdentifier("fund-cabal-submit-button")
                }
            }
        }
        .navigationBarBackButtonHidden(isSubmitting)
        .navigationTitle("Fund this cabal")
        .navigationBarTitleDisplayMode(.inline)
        .accessibilityIdentifier("fund-cabal-view")
    }

    @ViewBuilder
    private var stageContent: some View {
        switch stage {
        case .loading:
            AmountEntrySkeleton()
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .accessibilityIdentifier("fund-cabal-loading")
        case .failed:
            MonacoErrorRow(thing: "your balance", identifier: "fund-cabal-balance-error", retry: onRetry)
        case .needsMoney:
            EmptyState(
                title: "Add money first",
                message: "Send USDC on Solana to your deposit address, then fund this cabal.",
                actionTitle: "Add money",
                action: onAddMoney
            )
            .accessibilityIdentifier("fund-cabal-needs-money")
        case .amount:
            amountEntry
        }
    }

    private var amountEntry: some View {
        VStack(spacing: MonacoTheme.Space.l) {
            AmountEntry(
                amountText: $amountText,
                max: form.maxDollars,
                presets: [.dollars(25), .dollars(50), .dollars(100), .fraction(1, label: "Max")],
                helper: form.availability,
                problem: form.problem
            )
            AmountEntryNote(FundCabalForm.note(into: cabalName))
            Text(FundCabalForm.treasuryNote)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
                .multilineTextAlignment(.center)
                .fixedSize(horizontal: false, vertical: true)
                .accessibilityIdentifier("fund-cabal-treasury-note")
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
    }
}
