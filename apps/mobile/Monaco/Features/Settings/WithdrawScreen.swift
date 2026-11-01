import MonacoAnalytics
import MonacoCore
import SwiftUI

enum WithdrawNoteCopy {
    static let title = "Before you send"
}

struct WithdrawScreen: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(\.dismiss) private var dismiss
    @State private var model: WithdrawModel?

    var body: some View {
        Group {
            if let model {
                WithdrawKeypadView(model: model, onClose: { dismiss() })
            } else {
                MonacoTheme.canvas.ignoresSafeArea()
            }
        }
        .toolbar(.hidden, for: .tabBar)
        .toolbar(.hidden, for: .navigationBar)
        .navigationBarBackButtonHidden()
        .onDisappear { model?.dismiss() }
        .analyticsScreen("withdraw", step: .withdraw(.withdrawOpened))
        .task { await open() }
    }

    private func open() async {
        guard model == nil else { return }
        if environment.balance.balance == nil { await environment.balance.load() }
        guard !Task.isCancelled else { return }
        let next = WithdrawModel(
            api: environment.api, hints: environment.hints,
            submission: environment.moneySubmission(slot: Withdrawing.slot), settlements: environment.settlements,
            ownDepositAddress: environment.balance.balance?.depositAddress,
            clock: ContinuousClock(), balance: environment.balance)
        model = next
        next.load()
    }
}

struct WithdrawKeypadView: View {
    let model: WithdrawModel
    let onClose: () -> Void

    @Environment(ToastCenter.self) private var toasts
    @State private var showsNote = false
    @State private var showsBalances = false
    @State private var showsScanner = false
    @State private var focusRequests = 0

    private var amountText: Binding<String> {
        Binding(get: { model.amountText }, set: { edit($0) })
    }

    private var keypadState: AmountKeypadState {
        switch model.state {
        case .empty: .empty
        case .invalid(let max, let over):
            .invalid(
                limitLine: WithdrawCopy.limitLine(maxMicros: max, overMicros: over), slot: model.limitSlot ?? "")
        case .settled: .settled(line: model.line ?? "")
        case .typed, .belowMinimum, .needsAddress, .submitting, .processing, .failed: .typed
        }
    }

    private var isInvalid: Bool {
        if case .invalid = model.state { return true }
        return false
    }

    private var failure: String? {
        if case .failed(let message) = model.state { return message }
        return nil
    }

    private var isLocked: Bool {
        switch model.state {
        case .settled, .submitting, .processing: true
        default: false
        }
    }

    private var line: AmountKeypadLine? {
        model.line.map { AmountKeypadLine(text: $0, info: { showsNote = true }) }
    }

    private var action: AmountKeypadAction? {
        if let check = model.checkLine {
            return AmountKeypadAction(title: check, settledTitle: check, perform: {})
        }
        switch model.state {
        case .submitting:
            return AmountKeypadAction(title: model.buttonTitle ?? "", isWorking: true, keepsOpen: true, perform: {})
        case .processing:
            return AmountKeypadAction(title: model.buttonTitle ?? "", isWorking: true, perform: {})
        default:
            break
        }
        guard let title = model.buttonTitle else { return nil }
        return AmountKeypadAction(title: title, perform: perform)
    }

    private var max: Decimal? {
        model.maxMicros.map { AmountText.dollars(micros: $0) }
    }

    private var cashLabel: String {
        if case .settled(_, _, _, _, let after?) = model.state { return FundSourceCopy.cash(micros: after) }
        guard let available = model.quote?.availableMicros else { return "Cash" }
        return FundSourceCopy.cash(micros: available)
    }

    var body: some View {
        AmountKeypadScreen(
            amountText: amountText,
            state: keypadState,
            line: line,
            max: max,
            action: action,
            onClose: onClose,
            onSource: isLocked ? nil : { showsBalances = true },
            source: { sourcePill },
            header: { header }
        )
        .sensoryFeedback(.error, trigger: isInvalid) { was, now in !was && now }
        .onChange(of: failure) { _, message in
            guard let message else { return }
            toasts.current = MonacoToast(message: message)
        }
        .sheet(isPresented: $showsNote) {
            WithdrawNoteSheet(
                caveat: model.network.caveat,
                arrival: model.quote.map {
                    WithdrawCopy.arrivalNote(network: model.network, arrivalSeconds: $0.arrivalSeconds)
                })
        }
        .sheet(isPresented: $showsScanner) {
            AddressScannerSheet { text in
                WithdrawScan.apply(text, setAddress: { model.setAddress($0) }, setNetwork: { model.setNetwork($0) })
            }
        }
        .sheet(isPresented: $showsBalances) {
            NavigationStack { BalancesRoute().destination() }
                .presentationDetents([.fraction(MonacoSheetDetents.halfFraction), .large])
        }
    }

    private func scan() {
        guard AddressScanner.isAvailable else {
            toasts.current = MonacoToast(message: AddressScannerCopy.unavailable)
            return
        }
        showsScanner = true
    }

    private func edit(_ text: String) {
        if let maxMicros = model.maxMicros,
            text == AmountText.plain(AmountText.roundDownToCents(AmountText.dollars(micros: maxMicros))),
            text != model.amountText
        {
            model.useMax()
        } else {
            model.setAmountText(text)
        }
    }

    private func perform() {
        if model.quoteFailed {
            model.retryQuote()
        } else if model.state == .needsAddress {
            focusRequests += 1
        } else {
            Task { await model.submit() }
        }
    }

    @ViewBuilder
    private var sourcePill: some View {
        Image(systemName: "creditcard")
            .font(MonacoTheme.Typo.headline)
            .foregroundStyle(MonacoTheme.secondaryText)
            .accessibilityHidden(true)
        Text(cashLabel)
            .font(MonacoTheme.Typo.headline)
            .foregroundStyle(MonacoTheme.primaryText)
            .monacoPrivate()
    }

    private var header: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            WithdrawAddressField(
                text: Binding(get: { model.address }, set: { model.setAddress($0) }),
                placeholder: model.network.placeholder, isInvalid: model.addressProblem != nil,
                focusRequests: focusRequests, onScan: { scan() }
            )
            .disabled(isLocked)
            if let problem = model.addressProblem {
                Text(problem)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.loss)
                    .fixedSize(horizontal: false, vertical: true)
                    .accessibilityIdentifier("withdraw-address-problem")
            }
            WithdrawNetworkChips(networks: model.networks, selected: model.network) { model.setNetwork($0) }
                .allowsHitTesting(!isLocked)
        }
    }
}

private struct WithdrawNetworkChips: View {
    let networks: [WithdrawNetwork]
    let selected: WithdrawNetwork
    let select: (WithdrawNetwork) -> Void

    var body: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: MonacoTheme.Space.s) {
                ForEach(networks) { network in
                    let isSelected = network == selected
                    Button {
                        guard !isSelected else { return }
                        Haptics.selection()
                        select(network)
                    } label: {
                        Text(network.name).monacoChipLabel(
                            isSelected: isSelected, leadingSymbol: .check, horizontalPadding: MonacoTheme.Space.sm)
                    }
                    .buttonStyle(.plain)
                    .accessibilityAddTraits(isSelected ? [.isSelected] : [])
                    .accessibilityIdentifier("withdraw-network-\(network.id)")
                }
            }
        }
    }
}

struct WithdrawNoteSheet: View {
    let caveat: String
    let arrival: String?

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
            Text(caveat)
            if let arrival { Text(arrival) }
        }
        .font(MonacoTheme.Typo.subhead)
        .foregroundStyle(MonacoTheme.secondaryText)
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.bottom, MonacoTheme.Space.m)
        .frame(maxWidth: .infinity, alignment: .leading)
        .monacoFittedSheet(title: WithdrawNoteCopy.title, showsClose: true)
    }
}
