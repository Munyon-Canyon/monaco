import MonacoAPI
import MonacoAnalytics
import MonacoCore
import SwiftUI

struct WithdrawView: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(\.dismiss) private var dismiss
    @State private var withdrawing: Withdrawing?
    @State private var destinationAddress = ""
    @State private var network = WithdrawNetwork.solana
    @State private var amount = WithdrawAmount()
    @State private var refusedAddress: String?
    @State private var showConfirm = false
    @State private var frozen: WithdrawAmount.Frozen?
    @State private var estimate: WithdrawEstimate?

    private var trimmedAddress: String {
        destinationAddress.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private var amountText: Binding<String> {
        Binding(get: { amount.text }, set: { amount.edit(to: $0) })
    }

    private var previewRequest: WithdrawPreviewRequest? {
        WithdrawForm(
            amountText: amount.text, destinationAddress: destinationAddress, network: network,
            balance: environment.balance.balance
        ).previewRequest
    }

    var body: some View {
        WithdrawContent(
            state: environment.balance.state,
            amountText: amountText,
            destinationAddress: $destinationAddress,
            network: $network,
            estimatedReceiveMicros: estimate?.receiveMicros(for: previewRequest, at: Date()),
            onMax: { amount.tapMax() },
            refusedAddress: refusedAddress,
            onContinue: {
                frozen = amount.frozen(availableMicros: environment.balance.balance?.availableMicros)
                showConfirm = true
            },
            onAddMoney: {
                environment.navigator.open(
                    DepositRoute(prefillMicros: nil, cabalID: nil), in: environment.navigator.selectedTab)
            },
            onRetry: { Task { await environment.balance.load() } }
        )
        .analyticsScreen("withdraw", step: .withdraw(.withdrawOpened))
        .onChange(of: destinationAddress) { _, _ in refusedAddress = nil }
        .onChange(of: network) { _, _ in refusedAddress = nil }
        .onChange(of: showConfirm) { _, shown in
            if !shown { frozen = nil }
        }
        .task(id: previewRequest) {
            guard let previewRequest else { return }
            let source = WithdrawSource(api: environment.api)
            var delay = WithdrawEstimate.debounce
            while !Task.isCancelled {
                let fetched = await WithdrawEstimate.fetch(previewRequest, from: source, after: delay)
                guard !Task.isCancelled else { return }
                estimate = fetched
                guard let expiresAt = fetched?.expiresAt else { return }
                try? await Task.sleep(for: WithdrawEstimate.refetchDelay(until: expiresAt, now: Date()))
                guard !Task.isCancelled else { return }
                estimate = nil
                delay = .zero
            }
        }
        .navigationDestination(isPresented: $showConfirm) {
            WithdrawConfirmView(
                destinationAddress: trimmedAddress,
                network: network,
                amountText: amount.text,
                amountLabel: UsdAmountFormatter.format(flooredMicros: frozen?.micros ?? 0),
                fullBalanceMicros: frozen?.fullBalanceLabel != nil ? frozen?.micros : nil,
                isSubmitting: withdrawing?.isSubmitting ?? false,
                onWithdraw: { Task { await withdraw() } }
            )
        }
        .task {
            if withdrawing == nil {
                withdrawing = Withdrawing(source: WithdrawSource(api: environment.api), hints: environment.hints)
            }
            await environment.balance.load()
        }
    }

    private func withdraw() async {
        guard let withdrawing, let frozen, frozen.micros > 0 else { return }
        let micros = frozen.micros
        let fullBalanceLabel = frozen.fullBalanceLabel
        switch await withdrawing.submit(micros: micros, toAddress: trimmedAddress, network: network) {
        case .accepted:
            Haptics.success()
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
        "\(UsdAmountFormatter.format(flooredMicros: micros)) (full balance)"
    }

    struct Frozen: Equatable {
        let micros: Int64
        let fullBalanceLabel: String?
    }

    func frozen(availableMicros: Int64?) -> Frozen? {
        guard let micros = micros(availableMicros: availableMicros) else { return nil }
        return Frozen(micros: micros, fullBalanceLabel: fullBalanceLabel(availableMicros: availableMicros))
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
    let network: WithdrawNetwork
    let availableMicros: Int64?
    /// The member's own deposit address, which is never a destination.
    let ownDepositAddress: String?
    let estimatedReceiveMicros: Int64?
    private let balance: AccountBalance?

    init(
        amountText: String, destinationAddress: String, network: WithdrawNetwork = .solana,
        estimatedReceiveMicros: Int64? = nil, balance: AccountBalance?
    ) {
        self.balance = balance
        self.amountText = amountText
        self.destinationAddress = destinationAddress
        self.network = network
        self.estimatedReceiveMicros = estimatedReceiveMicros
        availableMicros = balance?.availableMicros
        ownDepositAddress = balance?.depositAddress
    }

    var hasNothingToWithdraw: Bool { balance.map { $0.availableMicros == 0 && $0.inFlightMicros == 0 } ?? false }

    var maxDollars: Decimal? {
        guard let availableMicros, availableMicros > 0 else { return nil }
        return Decimal(availableMicros) / Decimal(1_000_000)
    }

    var addressValidation: Result<String, WithdrawAddressProblem> {
        network.validate(destinationAddress, ownDepositAddress: ownDepositAddress)
    }

    var canContinue: Bool {
        guard let micros = AmountEntryText.micros(amountText), micros > 0, problem == nil else { return false }
        if case .success = addressValidation { return true }
        return false
    }

    /// Replaces the helper, in red, while the amount typed is over the balance or under the minimum.
    var problem: String? {
        guard let micros = AmountEntryText.micros(amountText) else { return nil }
        if let availableMicros, micros > availableMicros { return Self.overBalance }
        if micros > 0, micros < Self.minimumMicros { return Self.belowMinimum }
        return nil
    }

    static let overBalance = "Not enough in your account balance."
    static let minimumMicros: Int64 = 1_000_000
    static let belowMinimum = "The minimum is $1."

    /// Nothing while the field is empty; otherwise why the pasted address can't be used.
    var addressProblem: String? {
        guard case .failure(let problem) = addressValidation, !problem.isEmpty else { return nil }
        return network.message(for: problem)
    }

    var previewRequest: WithdrawPreviewRequest? {
        guard network.isEVM, canContinue, case .success(let address) = addressValidation,
            let micros = AmountEntryText.micros(amountText)
        else { return nil }
        return WithdrawPreviewRequest(micros: micros, toAddress: address, network: network)
    }

    var receiveLine: String? {
        guard let micros = AmountEntryText.micros(amountText), micros > 0, problem == nil else { return nil }
        guard network.isEVM else { return network.receiveLine(receiveMicros: micros) }
        return estimatedReceiveMicros.map { network.receiveLine(receiveMicros: $0) }
    }

    var balanceHelper: String {
        guard let availableMicros else { return "" }
        return "\(UsdAmountFormatter.format(flooredMicros: availableMicros)) available"
    }

    /// Under the address field: what kind of address, and that it is final.
    static let caveat = WithdrawNetwork.solana.caveat
    var addressCaveat: String { network.caveat }
}

struct WithdrawPreviewRequest: Hashable {
    let micros: Int64
    let toAddress: String
    let network: WithdrawNetwork
}

struct WithdrawEstimate: Equatable {
    let request: WithdrawPreviewRequest
    let receiveMicros: Int64
    let expiresAt: Date?

    func receiveMicros(for current: WithdrawPreviewRequest?, at now: Date) -> Int64? {
        guard current == request else { return nil }
        if let expiresAt, now >= expiresAt { return nil }
        return receiveMicros
    }

    static let debounce = Duration.milliseconds(400)

    static func refetchDelay(until expiresAt: Date, now: Date) -> Duration {
        .seconds(max(0, expiresAt.timeIntervalSince(now)))
    }

    static func fetch(
        _ request: WithdrawPreviewRequest, from source: WithdrawSource, after delay: Duration = debounce
    ) async -> WithdrawEstimate? {
        do {
            try await Task.sleep(for: delay)
            let preview = try await source.preview(
                micros: request.micros, toAddress: request.toAddress, network: request.network)
            return WithdrawEstimate(
                request: request, receiveMicros: preview.receiveMicros, expiresAt: preview.expiresAt)
        } catch {
            return nil
        }
    }
}

/// Withdraw's entry layout: the amount as the hero, the address it goes to in the market's
/// voice, and the two things to know about that address as captions under it.
///
/// Pure: what the screen knows comes in, what the member does goes out.
struct WithdrawContent: View {
    let state: LoadState<AccountBalance>
    @Binding var amountText: String
    @Binding var destinationAddress: String
    @Binding var network: WithdrawNetwork
    var estimatedReceiveMicros: Int64?
    var onMax: () -> Void = {}
    var refusedAddress: String?
    let onContinue: () -> Void
    var onAddMoney: () -> Void = {}
    let onRetry: () -> Void

    private var form: WithdrawForm {
        let balance: AccountBalance? = if case .loaded(let balance) = state { balance } else { nil }
        return WithdrawForm(
            amountText: amountText, destinationAddress: destinationAddress, network: network,
            estimatedReceiveMicros: estimatedReceiveMicros, balance: balance)
    }

    /// The amount pad, the destination field and the button belong together: whenever one is on
    /// screen, so are the others. A reload never takes them away mid-entry.
    private var showsForm: Bool {
        if case .loaded = state { return !form.hasNothingToWithdraw }
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
                case .loaded where form.hasNothingToWithdraw:
                    EmptyState(
                        title: "Nothing to withdraw yet", message: "Add money first. Your balance shows up here.",
                        actionTitle: "Add money", action: onAddMoney)
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

                    if let receiveLine = form.receiveLine {
                        Text(receiveLine)
                            .font(MonacoTheme.Typo.subheadStrong)
                            .foregroundStyle(MonacoTheme.ink)
                            .frame(maxWidth: .infinity)
                            .padding(.horizontal, MonacoTheme.Space.gutter)
                            .accessibilityIdentifier("withdraw-receive")
                    }

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
            HStack {
                MonacoSectionHeader("Send to")
                Spacer()
                PasteButton(payloadType: String.self) { strings in
                    guard let first = strings.first else { return }
                    destinationAddress = first.trimmingCharacters(in: .whitespacesAndNewlines)
                }
                .labelStyle(.titleOnly)
                .buttonBorderShape(.capsule)
                .tint(MonacoTheme.brand)
            }

            Picker("Network", selection: $network) {
                ForEach(WithdrawNetwork.allCases) { network in
                    Text(network.name).tag(network)
                }
            }
            .pickerStyle(.segmented)
            .accessibilityIdentifier("withdraw-network-picker")

            WithdrawAddressField(text: $destinationAddress, placeholder: network.placeholder)

            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                if let problem = refusedAddress ?? form.addressProblem {
                    Text(problem)
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.loss)
                        .fixedSize(horizontal: false, vertical: true)
                        .accessibilityIdentifier("withdraw-address-problem")
                }
                Text(form.addressCaveat)
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
    let placeholder: String

    var body: some View {
        MonacoAddressField(
            placeholder: placeholder, text: $text, accessibilityIdentifier: "withdraw-address-field")
    }
}
