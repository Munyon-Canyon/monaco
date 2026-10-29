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
    case fundCabalInsufficient
    case fundCabalSubmitting
    case fundCabalConfirmed
    case fundCabalPending
    case fundCabalNotSent
    case fundCabalFailed
    case addMoneyUnavailable
    case cardFunding
    case cardFundingWorking
    case cardFundingReturned
    case cardFundingReceived
    case cardFundingCancelled
    case networks
    case depositAddress
    case depositWaiting
    case depositReceived
    case withdrawInvalid
    case withdrawInsufficient
    case withdrawProcessing
    case withdrawCompleted

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
        if let state = scenario.fundState {
            FundCabalSampleHarness(state: state)
        } else if let state = scenario.withdrawState {
            WithdrawSampleHarness(state: state)
        } else if scenario.drawsContentDirectly {
            NavigationStack {
                MoneyFlowContentSample(scenario: scenario)
            }
            .tint(MonacoTheme.ink)
        } else {
            SampleAppFrame(
                auth: auth, tab: .home, routes: routes, presentsDeposit: scenario == .addMoney,
                session: SampleAppFrame.signedIn)
        }
    }

    private var routes: [any AppRoute] {
        switch scenario {
        case .addMoneyLoading, .addMoneyFailed: [DepositAddressRoute()]
        default: []
        }
    }
}

extension MoneyFlowSampleScenario {
    var fundState: FundCabalSampleState? {
        switch self {
        case .fundCabal: .empty
        case .fundCabalFunding, .fundCabalEmpty: .noCash
        case .fundCabalLoading: .loading
        case .fundCabalFailed: .unavailable
        case .fundCabalInsufficient: .invalid
        case .fundCabalSubmitting: .submitting
        case .fundCabalConfirmed: .settled
        case .fundCabalPending: .pending
        case .fundCabalNotSent: .failed
        default: nil
        }
    }

    var withdrawState: WithdrawSampleState? {
        switch self {
        case .withdraw: .empty
        case .withdrawFailed: .feeFailed
        case .withdrawInvalid: .invalidAddress
        case .withdrawInsufficient: .invalid
        case .withdrawProcessing: .submitting
        case .withdrawCompleted: .settled
        default: nil
        }
    }

    var drawsContentDirectly: Bool {
        switch self {
        case .addMoney, .addMoneyLoading, .addMoneyFailed:
            false
        default:
            true
        }
    }
}

private enum MoneyFlowContentFixture {
    static let address = "7xKXtg2CW87d97TXJSDpbD5jBkheTqA83TZRuJosgAsU"
    static let evmAddress = "0x52908400098527886E0F7030069857D2E4169EE7"

    @MainActor
    static func balance(
        available: Int64 = 500_000_000, inFlight: Int64 = 0, arriving: Int64 = 0
    ) -> AccountBalance {
        var wire = MoneyFlowSampleData.balance(available: available, inFlight: inFlight)
        wire.arrivingMicros = arriving > 0 ? String(arriving) : nil
        return (try? AccountBalance(wire))
            ?? AccountBalance(
                availableMicros: available, onChainMicros: available, inFlightMicros: inFlight,
                depositAddress: address, asOf: Date())
    }
}

private struct MoneyFlowContentSample: View {
    let scenario: MoneyFlowSampleScenario

    var body: some View {
        content
    }

    @ViewBuilder
    private var content: some View {
        switch scenario {
        case .addMoneyUnavailable:
            DepositChooser(state: .loaded(MoneyFlowContentFixture.balance()), cardStartFailed: true)
        case .cardFunding:
            CardFundingContent(screen: .handoff(working: false), onContinue: {}, onBack: {})
        case .cardFundingWorking:
            CardFundingContent(screen: .handoff(working: true), onContinue: {}, onBack: {})
        case .cardFundingReturned:
            CardFundingContent(screen: .checking, onContinue: {}, onBack: {})
        case .cardFundingReceived:
            CardFundingContent(screen: .received, onContinue: {}, onBack: {})
        case .cardFundingCancelled:
            CardFundingContent(screen: .cancelled(CardDepositState.didNotGoThrough), onContinue: {}, onBack: {})
        case .networks:
            DepositNetworksView(networks: [
                DepositNetwork(id: DepositNetwork.solanaID, address: MoneyFlowContentFixture.address),
                DepositNetwork(id: "base", address: MoneyFlowContentFixture.evmAddress),
                DepositNetwork(id: "arbitrum", address: MoneyFlowContentFixture.evmAddress),
                DepositNetwork(id: "ethereum", address: MoneyFlowContentFixture.evmAddress),
            ])
        case .depositAddress, .depositWaiting, .depositReceived:
            DepositContent(
                address: MoneyFlowContentFixture.address,
                state: .loaded(MoneyFlowContentFixture.balance(arriving: scenario == .depositWaiting ? 4_500_000 : 0)),
                onCopy: { _ in }, onRetryAddress: {}, onRetryBalance: {},
                receivedMicros: scenario == .depositReceived ? 25_000_000 : nil)
        default:
            EmptyView()
        }
    }
}

extension MoneyFlowSampleScenario {
    var script: SampleAPIScript {
        switch self {
        case .addMoneyLoading: SampleAPIScript(mode: .hang)
        case .addMoneyFailed: SampleAPIScript(mode: .balanceFails)
        default: SampleAPIScript()
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
