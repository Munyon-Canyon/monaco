#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI

/// Debug-only: the money screens on canned data, with no sign-in and no backend.
/// Launch with `-MonacoMoneyFlowSample <scenario>`:
/// `addMoney` (address and a balance with a fund on its way) · `addMoneyLoading` ·
/// `addMoneyFailed` · `fundCabal` ($50 typed) · `fundCabalFunding` (nothing available, a fund on
/// its way) · `fundCabalEmpty` (nothing to fund with yet) · `fundCabalLoading` · `withdraw` (amount and address typed) ·
/// `withdrawFailed` (the balance could not be read) · `withdrawConfirm`.
///
/// Every screen is the product's own layout — `DepositContent`, `FundCabalContent`,
/// `WithdrawContent`, `WithdrawConfirmView` — fed sample values in
/// place of the network, so what is shot here is what ships.
enum MoneyFlowSampleScenario: String, CaseIterable {
    case addMoney
    case addMoneyLoading
    case addMoneyFailed
    case fundCabal
    case fundCabalFunding
    case fundCabalEmpty
    case fundCabalLoading
    case withdraw
    case withdrawFailed
    case withdrawConfirm

    static let launchArgument = "-MonacoMoneyFlowSample"

    static func matching(_ arguments: [String]) -> MoneyFlowSampleScenario? {
        guard let flag = arguments.firstIndex(of: launchArgument), arguments.indices.contains(flag + 1) else {
            return nil
        }
        return MoneyFlowSampleScenario(rawValue: arguments[flag + 1])
    }
}

struct MoneyFlowSampleHarness: View {
    let scenario: MoneyFlowSampleScenario
    @ObservedObject var auth: PrivyAuthService

    @State private var amountText: String
    @State private var destination: String

    init(scenario: MoneyFlowSampleScenario, auth: PrivyAuthService) {
        self.scenario = scenario
        self.auth = auth
        _amountText = State(initialValue: MoneyFlowSampleData.amountText(for: scenario))
        _destination = State(initialValue: scenario == .withdraw ? MoneyFlowSampleData.destination : "")
    }

    var body: some View {
        NavigationStack {
            root
        }
        .tint(MonacoTheme.ink)
    }

    @ViewBuilder
    private var root: some View {
        switch scenario {
        case .addMoney, .addMoneyLoading, .addMoneyFailed:
            DepositContent(
                address: MoneyFlowSampleData.depositAddress,
                state: MoneyFlowSampleData.depositState(for: scenario),
                onCopy: { _ in }, onRetryAddress: {}, onRetryBalance: {})
        case .fundCabal, .fundCabalFunding, .fundCabalEmpty, .fundCabalLoading:
            FundCabalContent(
                state: MoneyFlowSampleData.fundState(for: scenario),
                cabalName: "Weekend investors",
                amountText: $amountText,
                isSubmitting: false,
                onSubmit: {},
                onRetry: {},
                onAddMoney: {}
            )
        case .withdraw, .withdrawFailed:
            WithdrawContent(
                state: scenario == .withdrawFailed
                    ? .failed(.transport(URLError(.notConnectedToInternet)))
                    : .loaded(MoneyFlowSampleData.balance),
                amountText: $amountText,
                destinationAddress: $destination,
                onContinue: {},
                onRetry: {}
            )
        case .withdrawConfirm:
            WithdrawConfirmView(
                destinationAddress: MoneyFlowSampleData.destination, amountText: "100", isSubmitting: false,
                onWithdraw: {})
        }
    }
}

enum MoneyFlowSampleData {
    /// The member's own deposit address, the same one the other harnesses use.
    static let depositAddress = "7xKXtg2CW87d97TXJSDpbD5jBkheTqA83TZRuJosgAsU"

    /// An outside Solana address that passes `SolanaAddress.validate`.
    static let destination = "9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM"

    static let balance = account(availableMicros: 248_500_000)

    static let emptyBalance = account(availableMicros: 0)

    static let fundingBalance = account(availableMicros: 0, inFlightMicros: 50_000_000)

    private static func account(availableMicros: Int64, inFlightMicros: Int64 = 0) -> AccountBalance {
        AccountBalance(
            availableMicros: availableMicros, onChainMicros: availableMicros + inFlightMicros,
            inFlightMicros: inFlightMicros,
            depositAddress: depositAddress, asOf: Date(timeIntervalSince1970: 1_759_579_200)
        )
    }

    static func depositState(for scenario: MoneyFlowSampleScenario) -> LoadState<AccountBalance> {
        switch scenario {
        case .addMoneyLoading:
            return .loading
        case .addMoneyFailed:
            return .failed(.transport(URLError(.notConnectedToInternet)))
        default:
            return (try? AccountBalance(.sample)).map(LoadState.loaded) ?? .loading
        }
    }

    static func fundState(for scenario: MoneyFlowSampleScenario) -> LoadState<AccountBalance> {
        switch scenario {
        case .fundCabalLoading:
            return .loading
        case .fundCabalEmpty:
            return .loaded(emptyBalance)
        case .fundCabalFunding:
            return .loaded(fundingBalance)
        default:
            return .loaded(balance)
        }
    }

    static func amountText(for scenario: MoneyFlowSampleScenario) -> String {
        switch scenario {
        case .fundCabal: return "50"
        case .withdraw: return "100"
        default: return ""
        }
    }
}

final class MoneyFlowSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard let scenario = MoneyFlowSampleScenario.matching(arguments) else { return nil }
        return AnyView(MoneyFlowSampleHarness(scenario: scenario, auth: auth))
    }
}
#endif
