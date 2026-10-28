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

    var body: some View {
        FundCabalSampleHarness(amountText: "250", submits: true, server: scenario.server)
    }
}

extension Flow07Scenario {
    fileprivate var server: FundCabalSampleServer {
        switch self {
        case .invalidInput:
            FundCabalSampleServer(
                fund: .refused(status: 400, code: "invalid_input", message: "The request is not valid."))
        case .notCabalMember:
            FundCabalSampleServer(
                fund: .refused(status: 403, code: "not_cabal_member", message: "You are not a member of this cabal."))
        case .insufficientFunds:
            FundCabalSampleServer(
                fund: .refused(status: 422, code: "insufficient_funds", message: "Not enough in your account balance."))
        case .cabalPaused:
            FundCabalSampleServer(fund: .refused(status: 409, code: "cabal_paused", message: "This cabal is paused."))
        case .cabalBanned:
            FundCabalSampleServer(
                fund: .refused(
                    status: 422, code: "cabal_banned",
                    message: "This cabal is banned. Members can still cash out and leave."))
        case .potValueZero:
            FundCabalSampleServer(
                fund: .refused(status: 409, code: "pot_value_zero", message: "The pot has no value right now."))
        case .privyUnavailable:
            FundCabalSampleServer(
                fund: .refused(status: 503, code: "privy_unavailable", message: "Wallet signing is down. Try again."))
        case .interrupted:
            FundCabalSampleServer(fund: .dropped)
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
