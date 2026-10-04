import MonacoAPI
import MonacoCabal
import MonacoCore
import SwiftUI

/// Fund this cabal: move account balance into one cabal's pot.
///
/// Owns the balance, the cabal's name and the reload toast. `FundCabalContent` is the layout.
struct FundCabalView: View {
    let cabalID: String

    @Environment(AppEnvironment.self) private var environment
    @State private var balanceSource: BalanceSource?
    @State private var cabal: CabalActionsModel?
    @State private var amountText = ""
    @State private var toast: MonacoToast?

    var body: some View {
        FundCabalContent(
            state: balanceSource?.state ?? .loading,
            cabalName: cabal?.cabalName,
            amountText: $amountText,
            onRetry: { Task { await balanceSource?.load() } },
            onAddMoney: {
                environment.navigator.open(DepositRoute(cabalID: cabalID), in: environment.navigator.selectedTab)
            }
        )
        .monacoToast($toast, bottomInset: 72)
        .onChange(of: balanceSource?.failureTick) { _, _ in
            guard balanceSource?.balance != nil, let error = balanceSource?.lastError else { return }
            toast = MonacoToast(message: BalanceSource.message(for: error))
        }
        .task {
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
        let available = "\(UsdAmountFormatter.format(micros: availableMicros)) available"
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

    static let comingSoon = "Funding opens soon."
}

/// Fund this cabal's layout: the amount as the hero, the balance it comes out of, and a button
/// that reads the amount.
///
/// Pure: what the screen knows comes in, what the member does goes out.
struct FundCabalContent: View {
    let state: LoadState<AccountBalance>
    let cabalName: String?
    @Binding var amountText: String
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
        .monacoCanvas()
        .safeAreaInset(edge: .bottom) {
            if stage.showsAmountEntry {
                BottomCTA {
                    VStack(spacing: MonacoTheme.Space.s) {
                        Text(FundCabalForm.comingSoon)
                            .font(MonacoTheme.Typo.caption)
                            .foregroundStyle(MonacoTheme.muted)
                        Button(form.ctaTitle) {}
                            .buttonStyle(.monacoPrimary)
                            .disabled(true)
                    }
                    .accessibilityElement(children: .contain)
                    .accessibilityIdentifier("fund-submit-coming")
                }
            }
        }
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
            EmptyState(title: "Couldn't load your balance.", actionTitle: "Try again", action: onRetry)
                .accessibilityIdentifier("fund-cabal-balance-error")
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
                problem: form.problem,
                showsKeyboardDoneButton: true
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
