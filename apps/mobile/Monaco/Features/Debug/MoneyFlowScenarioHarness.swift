#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI

private enum MoneyFlowScenarioFixture {
    static let address = "7xKXtg2CW87d97TXJSDpbD5jBkheTqA83TZRuJosgAsU"

    @MainActor
    static var balance: LoadState<AccountBalance> {
        guard let balance = try? AccountBalance(MoneyFlowSampleData.balance(available: 500_000_000, inFlight: 0))
        else { return .loading }
        return .loaded(balance)
    }
}

final class FundFlowHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard let scenario = Flow07Scenario.matching(arguments) else { return nil }
        return AnyView(FundFlowScenarioSample(scenario: scenario))
    }
}

private struct FundFlowScenarioSample: View {
    let scenario: Flow07Scenario

    @Environment(ToastCenter.self) private var toasts
    @State private var amountText = "250"

    var body: some View {
        NavigationStack {
            FundCabalContent(
                cabalID: Funding.previewCabalID, cabalName: "Big Ballers", state: MoneyFlowScenarioFixture.balance,
                amountText: $amountText, isSubmitting: false, onSubmit: {}, onRetry: {}, onAddMoney: {})
        }
        .tint(MonacoTheme.ink)
        .task { await submit() }
    }

    private func submit() async {
        switch await Funding.preview(answering: scenario).submit(micros: 250_000_000) {
        case .accepted:
            break
        case .refused(.needsMoney(let message)):
            toasts.current = MonacoToast(message: message, action: MonacoToastAction(title: "Add money") {})
        case .refused(.toast(let message)), .unconfirmed(let message):
            toasts.current = MonacoToast(message: message)
        }
    }
}

final class CashOutFlowHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard let scenario = Flow14Scenario.matching(arguments) else { return nil }
        return AnyView(CashOutFlowScenarioSample(scenario: scenario))
    }
}

private struct CashOutFlowScenarioSample: View {
    let scenario: Flow14Scenario

    @Environment(ToastCenter.self) private var toasts
    @State private var model: CashOutModel
    @State private var amountText = "100"

    init(scenario: Flow14Scenario) {
        self.scenario = scenario
        _model = State(initialValue: CashOutModel.preview(answering: scenario))
    }

    var body: some View {
        NavigationStack {
            CashOutContent(
                model: model, cabalName: "Big Ballers", runningJob: nil, amountText: $amountText, step: .review,
                onSubmit: {}, onFund: {})
        }
        .tint(MonacoTheme.ink)
        .task { await submit() }
    }

    private func submit() async {
        await model.load()
        if case .refused(let message) = await model.submit(enteredMicros: 100_000_000) {
            toasts.current = MonacoToast(message: message)
        }
    }
}

final class WithdrawFlowHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard let scenario = Flow15Scenario.matching(arguments) else { return nil }
        return AnyView(WithdrawFlowScenarioSample(scenario: scenario))
    }
}

private struct WithdrawFlowScenarioSample: View {
    let scenario: Flow15Scenario

    @Environment(ToastCenter.self) private var toasts
    @State private var amountText = "100"
    @State private var destination = MoneyFlowScenarioFixture.address
    @State private var network = WithdrawNetwork.solana
    @State private var refusedAddress: String?

    var body: some View {
        NavigationStack {
            WithdrawContent(
                state: MoneyFlowScenarioFixture.balance, amountText: $amountText, destinationAddress: $destination,
                network: $network, refusedAddress: refusedAddress, onContinue: {}, onRetry: {})
        }
        .tint(MonacoTheme.ink)
        .task { await submit() }
    }

    private func submit() async {
        let withdrawing = Withdrawing.preview(answering: scenario)
        switch await withdrawing.submit(micros: 100_000_000, toAddress: destination, network: network) {
        case .accepted:
            break
        case .refused(.address(let message)):
            refusedAddress = message
        case .refused(.toast(let message)), .unconfirmed(let message):
            toasts.current = MonacoToast(message: message)
        }
    }
}
#endif
