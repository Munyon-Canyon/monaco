import MonacoAPI
import MonacoCore
import SwiftUI

struct WithdrawView: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(\.dismiss) private var dismiss
    @State private var balanceSource: BalanceSource?
    @State private var withdrawing: Withdrawing?
    @State private var destinationAddress = ""
    @State private var amount = WithdrawAmount()
    @State private var refusedAddress: String?
    @State private var showConfirm = false

    private var trimmedAddress: String {
        destinationAddress.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private var amountText: Binding<String> {
        Binding(get: { amount.text }, set: { amount.edit(to: $0) })
    }

    var body: some View {
        WithdrawContent(
            state: balanceSource?.state ?? .loading,
            amountText: amountText,
            destinationAddress: $destinationAddress,
            onMax: { amount.tapMax() },
            refusedAddress: refusedAddress,
            onContinue: { showConfirm = true },
            onRetry: { Task { await balanceSource?.load() } }
        )
        .onChange(of: destinationAddress) { _, _ in refusedAddress = nil }
        .onChange(of: balanceSource?.failureTick) { _, _ in
            guard balanceSource?.balance != nil, let error = balanceSource?.lastError else { return }
            toasts.current = MonacoToast(message: BalanceSource.message(for: error))
        }
        .navigationDestination(isPresented: $showConfirm) {
            WithdrawConfirmView(
                destinationAddress: trimmedAddress,
                amountText: amount.text,
                fullBalanceLabel: amount.fullBalanceLabel(availableMicros: balanceSource?.balance?.availableMicros),
                isSubmitting: withdrawing?.isSubmitting ?? false,
                onWithdraw: { Task { await withdraw() } }
            )
        }
        .task {
            if withdrawing == nil {
                withdrawing = Withdrawing(source: WithdrawSource(api: environment.api), hints: environment.hints)
            }
            let source = balanceSource ?? BalanceSource(api: environment.api, hints: environment.hints)
            balanceSource = source
            await source.load()
            await source.observe()
        }
        .onScreenVisibilityChange { visible in
            balanceSource?.setVisible(visible)
        }
    }

    private func withdraw() async {
        guard let withdrawing, let micros = amount.micros(availableMicros: balanceSource?.balance?.availableMicros),
            micros > 0
        else { return }
        let fullBalanceLabel = amount.fullBalanceLabel(availableMicros: balanceSource?.balance?.availableMicros)
        switch await withdrawing.submit(micros: micros, toAddress: trimmedAddress) {
        case .accepted:
            if let line = Self.message(for: withdrawing.progress, fullBalanceLabel: fullBalanceLabel) {
                toasts.show(success: line)
            }
            showConfirm = false
            dismiss()
            Task { [toasts] in
                let settled = await withdrawing.settle()
                guard settled.isSettled else { return }
                toasts.current = Self.toast(for: settled, fullBalanceLabel: fullBalanceLabel)
            }
        case .refused(.address(let message)):
            refusedAddress = message
            showConfirm = false
        case .refused(.toast(let message)):
            toasts.current = MonacoToast(message: message)
            showConfirm = false
        case .unconfirmed(let message):
            toasts.current = MonacoToast(message: message)
        }
    }

    static func message(for progress: WithdrawProgress, fullBalanceLabel: String?) -> String? {
        guard let line = progress.toast else { return nil }
        let sent: Withdrawal
        switch progress {
        case .submitted(let withdrawal), .confirmed(let withdrawal): sent = withdrawal
        case .idle, .submitting, .failed: return line
        }
        guard let fullBalanceLabel else { return line }
        return line.replacingOccurrences(
            of: UsdAmountFormatter.format(micros: sent.amountMicros), with: fullBalanceLabel)
    }

    static func toast(for progress: WithdrawProgress, fullBalanceLabel: String? = nil) -> MonacoToast? {
        guard let line = message(for: progress, fullBalanceLabel: fullBalanceLabel) else { return nil }
        guard case .confirmed(let withdrawal) = progress else { return MonacoToast(message: line) }
        let link = withdrawal.solscanURL.map { MonacoToastLink(title: "View on Solscan", url: $0) }
        return MonacoToast(message: line, isSuccess: true, link: link)
    }
}

struct WithdrawAmount: Equatable {
    private(set) var text = ""
    private(set) var withdrawAll = false

    mutating func edit(to newText: String) {
        guard newText != text else { return }
        text = newText
        withdrawAll = false
    }

    mutating func tapMax() {
        withdrawAll = true
    }

    func fullBalanceLabel(availableMicros: Int64?) -> String? {
        guard withdrawAll, let availableMicros else { return nil }
        return Self.fullBalanceLabel(micros: availableMicros)
    }

    static func fullBalanceLabel(micros: Int64) -> String {
        "\(UsdAmountFormatter.format(micros: micros - micros % 10_000)) (full balance)"
    }

    func micros(availableMicros: Int64?) -> Int64? {
        if withdrawAll, let availableMicros { return availableMicros }
        return AmountEntryText.micros(text)
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
        return "\(UsdAmountFormatter.format(micros: availableMicros - availableMicros % 10_000)) available"
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
    var onMax: () -> Void = {}
    var refusedAddress: String?
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
                    MonacoErrorRow(thing: "your balance", identifier: "withdraw-balance-error", retry: onRetry)
                case .loaded:
                    AmountEntry(
                        amountText: $amountText,
                        max: form.maxDollars,
                        presets: [.fraction(1, label: "Max")],
                        helper: form.balanceHelper,
                        problem: form.problem,
                        onPreset: { _ in onMax() }
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
                        .disabled(!form.canContinue || refusedAddress != nil)
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
                if let problem = refusedAddress ?? form.addressProblem {
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
        .padding(.horizontal, MonacoTheme.Space.gutter)
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
