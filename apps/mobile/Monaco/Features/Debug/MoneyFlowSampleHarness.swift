#if DEBUG
import MonacoAPI
import MonacoCore
import SwiftUI

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

    var body: some View {
        SampleAppFrame(
            auth: auth, tab: .home, routes: [route],
            session: SampleAppFrame.signedIn)
    }

    private var route: any AppRoute {
        switch scenario {
        case .addMoney: DepositRoute()
        case .addMoneyLoading, .addMoneyFailed: DepositAddressRoute()
        case .fundCabal, .fundCabalFunding, .fundCabalEmpty, .fundCabalLoading:
            FundRoute(cabalID: GroupDetailSampleData.cabalID)
        case .withdraw, .withdrawFailed: WithdrawRoute()
        }
    }
}

extension MoneyFlowSampleScenario {
    var script: SampleAPIScript {
        switch self {
        case .addMoneyLoading, .fundCabalLoading: SampleAPIScript(mode: .hang)
        case .addMoneyFailed, .withdrawFailed: SampleAPIScript(mode: .balanceFails)
        case .fundCabalFunding:
            SampleAPIScript(balance: MoneyFlowSampleData.balance(available: 0, inFlight: 50_000_000))
        case .fundCabalEmpty:
            SampleAPIScript(balance: MoneyFlowSampleData.balance(available: 0, inFlight: 0))
        case .addMoney, .fundCabal, .withdraw: SampleAPIScript()
        }
    }
}

enum MoneyFlowSampleData {
    static func balance(available: Int64, inFlight: Int64) -> Components.Schemas.Balance {
        var balance = Components.Schemas.Balance.sample
        balance.availableMicros = String(available)
        balance.inFlightMicros = String(inFlight)
        balance.onChainMicros = String(available + inFlight)
        return balance
    }
}

final class MoneyFlowSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        guard let scenario = MoneyFlowSampleScenario.matching(arguments) else { return nil }
        SampleAPIProtocol.install(scenario.script)
        return AnyView(MoneyFlowSampleHarness(scenario: scenario, auth: auth))
    }
}
#endif
