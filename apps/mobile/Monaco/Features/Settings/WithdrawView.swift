import MonacoAPI
import MonacoCore
import SwiftUI

/// Withdraw: send available account USDC to an external Solana address.
///
/// Owns the balance, the typed amount and address, and the reload toast. `WithdrawContent` is the
/// entry layout and `WithdrawConfirmView` the step before sending. Nothing is sent from here.
struct WithdrawView: View {
    @Environment(AppEnvironment.self) private var environment
    @State private var balanceSource: BalanceSource?
    @State private var destinationAddress = ""
    @State private var amountText = ""
    @State private var showConfirm = false
    @State private var toast: MonacoToast?

    var body: some View {
        WithdrawContent(
            state: balanceSource?.state ?? .loading,
            amountText: $amountText,
            destinationAddress: $destinationAddress,
            onContinue: { showConfirm = true },
            onRetry: { Task { await balanceSource?.load() } }
        )
        .monacoToast($toast, bottomInset: 72)
        .onChange(of: balanceSource?.failureTick) { _, _ in
            guard balanceSource?.balance != nil, let error = balanceSource?.lastError else { return }
            toast = MonacoToast(message: BalanceSource.message(for: error))
        }
        .navigationDestination(isPresented: $showConfirm) {
            WithdrawConfirmView(
                destinationAddress: destinationAddress.trimmingCharacters(in: .whitespacesAndNewlines),
                amountText: amountText
            )
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

/// What Withdraw says and allows for the amount and address typed against the balance it has.
struct WithdrawForm: Equatable {
    let amountText: String
    let destinationAddress: String
    let availableMicros: Int64?
    /// The member's own deposit address, which is never a destination.
    let ownDepositAddress: String?

    init(amountText: String, destinationAddress: String, balance: AccountBalance?) {
        self.amountText = amountText
        self.destinationAddress = destinationAddress
        availableMicros = balance?.availableMicros
        ownDepositAddress = balance?.depositAddress
    }

    var maxDollars: Decimal? {
        guard let availableMicros, availableMicros > 0 else { return nil }
        return Decimal(availableMicros) / Decimal(1_000_000)
    }

    var addressValidation: Result<String, SolanaAddressProblem> {
        SolanaAddress.validate(destinationAddress, ownDepositAddress: ownDepositAddress)
    }

    var canContinue: Bool {
        guard let micros = AmountEntryText.micros(amountText), micros > 0, problem == nil else { return false }
        if case .success = addressValidation { return true }
        return false
    }

    /// Replaces the helper, in red, while the amount typed is more than the balance holds.
    var problem: String? {
        guard let availableMicros, let micros = AmountEntryText.micros(amountText), micros > availableMicros else {
            return nil
        }
        return Self.overBalance
    }

    static let overBalance = "Not enough in your account balance."

    /// Nothing while the field is empty; otherwise why the pasted address can't be used.
    var addressProblem: String? {
        guard case .failure(let problem) = addressValidation, problem != .empty else { return nil }
        return SolanaAddress.message(for: problem)
    }

    var balanceHelper: String {
        guard let availableMicros else { return "" }
        return "\(UsdAmountFormatter.format(micros: availableMicros)) available"
    }

    /// Under the address field: what kind of address, and that it is final.
    static let caveat = "A Solana address that accepts USDC. Transfers can't be undone."
}

/// Withdraw's entry layout: the amount as the hero, the address it goes to in the market's
/// voice, and the two things to know about that address as captions under it.
///
/// Pure: what the screen knows comes in, what the member does goes out.
struct WithdrawContent: View {
    let state: LoadState<AccountBalance>
    @Binding var amountText: String
    @Binding var destinationAddress: String
    let onContinue: () -> Void
    let onRetry: () -> Void

    private var form: WithdrawForm {
        if case .loaded(let balance) = state {
            return WithdrawForm(amountText: amountText, destinationAddress: destinationAddress, balance: balance)
        }
        return WithdrawForm(amountText: amountText, destinationAddress: destinationAddress, balance: nil)
    }

    /// The amount pad, the destination field and the button belong together: whenever one is on
    /// screen, so are the others. A reload never takes them away mid-entry.
    private var showsForm: Bool {
        if case .loaded = state { return true }
        return false
    }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xl) {
                switch state {
                case .idle, .loading:
                    AmountEntrySkeleton(presetCount: 1)
                        .padding(.horizontal, MonacoTheme.Space.gutter)
                        .accessibilityIdentifier("withdraw-loading")
                case .failed:
                    EmptyState(title: "Couldn't load your balance.", actionTitle: "Try again", action: onRetry)
                        .accessibilityIdentifier("withdraw-balance-error")
                case .loaded:
                    AmountEntry(
                        amountText: $amountText,
                        max: form.maxDollars,
                        presets: [.fraction(1, label: "Max")],
                        helper: form.balanceHelper,
                        problem: form.problem,
                        showsKeyboardDoneButton: true
                    )
                    .padding(.horizontal, MonacoTheme.Space.gutter)

                    destination
                }
            }
            .padding(.top, MonacoTheme.Space.xl)
            .padding(.bottom, MonacoTheme.Space.xl)
        }
        // The decimal pad covers the destination field, and a decimal pad has no return key:
        // dragging the list is the member's way back to the address.
        .scrollDismissesKeyboard(.interactively)
        .monacoCanvas()
        .safeAreaInset(edge: .bottom) {
            if showsForm {
                BottomCTA {
                    Button("Continue", action: onContinue)
                        .buttonStyle(.monacoPrimary)
                        .disabled(!form.canContinue)
                        .accessibilityIdentifier("withdraw-continue-button")
                }
            }
        }
        .navigationTitle("Withdraw")
        .navigationBarTitleDisplayMode(.inline)
    }

    private var destination: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            MonacoSectionHeader("Send to")

            WithdrawAddressField(text: $destinationAddress)

            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                if let problem = form.addressProblem {
                    Text(problem)
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.loss)
                        .fixedSize(horizontal: false, vertical: true)
                        .accessibilityIdentifier("withdraw-address-problem")
                }
                Text(WithdrawForm.caveat)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
        .padding(.horizontal, MonacoTheme.Space.m)
    }
}

/// The destination, in the market's voice: it wraps by character and never hyphenates, so a
/// member can read the whole address back before the confirm step shows it again.
private struct WithdrawAddressField: View {
    @Binding var text: String

    private static let placeholder = "USDC address on Solana"

    var body: some View {
        MonacoAddressField(
            placeholder: Self.placeholder, text: $text, accessibilityIdentifier: "withdraw-address-field")
    }
}
