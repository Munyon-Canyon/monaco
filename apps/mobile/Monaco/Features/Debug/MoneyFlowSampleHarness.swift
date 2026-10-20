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
    case withdrawReview
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
        if scenario.drawsContentDirectly {
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
        case .fundCabal, .fundCabalFunding, .fundCabalEmpty, .fundCabalLoading, .fundCabalFailed:
            [FundRoute(cabalID: GroupDetailSampleData.cabalID)]
        case .withdraw, .withdrawFailed: [WithdrawRoute()]
        default: []
        }
    }
}

extension MoneyFlowSampleScenario {
    var drawsContentDirectly: Bool {
        switch self {
        case .addMoney, .addMoneyLoading, .addMoneyFailed, .fundCabal, .fundCabalFunding, .fundCabalEmpty,
            .fundCabalLoading, .fundCabalFailed, .withdraw, .withdrawFailed:
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

    @State private var amount: String
    @State private var destination: String
    @State private var network = WithdrawNetwork.solana

    init(scenario: MoneyFlowSampleScenario) {
        self.scenario = scenario
        switch scenario {
        case .fundCabalInsufficient:
            _amount = State(initialValue: "750")
            _destination = State(initialValue: "")
        case .fundCabalSubmitting, .fundCabalConfirmed:
            _amount = State(initialValue: "250")
            _destination = State(initialValue: "")
        case .withdrawInvalid:
            _amount = State(initialValue: "100")
            _destination = State(initialValue: "not-an-address")
        case .withdrawInsufficient:
            _amount = State(initialValue: "700")
            _destination = State(initialValue: MoneyFlowContentFixture.address)
        default:
            _amount = State(initialValue: "")
            _destination = State(initialValue: "")
        }
    }

    var body: some View {
        content
    }

    @ViewBuilder
    private var content: some View {
        switch scenario {
        case .fundCabalInsufficient, .fundCabalSubmitting, .fundCabalConfirmed:
            fund
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
        case .withdrawInvalid, .withdrawInsufficient:
            WithdrawContent(
                state: .loaded(MoneyFlowContentFixture.balance()), amountText: $amount,
                destinationAddress: $destination,
                network: $network, onContinue: {}, onRetry: {})
        case .withdrawReview:
            WithdrawConfirmView(
                destinationAddress: MoneyFlowContentFixture.evmAddress, network: .base, amountText: "100",
                amountLabel: "$100.00", networkCost: "$0.12", isSubmitting: false, onWithdraw: {})
        case .withdrawProcessing, .withdrawCompleted:
            WithdrawConfirmView(
                destinationAddress: MoneyFlowContentFixture.address, amountText: "100", amountLabel: "$100.00",
                outcome: WithdrawOutcomeSummary(
                    amountLabel: "$100.00", status: scenario == .withdrawCompleted ? .completed : .pending),
                isSubmitting: false, onWithdraw: {})
        default:
            EmptyView()
        }
    }

    private var fund: some View {
        FundCabalContent(
            cabalID: GroupDetailSampleData.cabalID, cabalName: "Big Ballers",
            state: .loaded(MoneyFlowContentFixture.balance()), amountText: $amount,
            isSubmitting: scenario == .fundCabalSubmitting, onSubmit: {}, onRetry: {}, onAddMoney: {}
        )
        .sheet(isPresented: .constant(scenario == .fundCabalConfirmed)) {
            FundConfirmedSheet(confirmation: FundConfirmation(amount: "$250.00", cabalName: "Big Ballers"))
        }
    }
}

extension MoneyFlowSampleScenario {
    var script: SampleAPIScript {
        switch self {
        case .addMoneyLoading, .fundCabalLoading: SampleAPIScript(mode: .hang)
        case .addMoneyFailed, .withdrawFailed, .fundCabalFailed: SampleAPIScript(mode: .balanceFails)
        case .fundCabalFunding:
            SampleAPIScript(balance: MoneyFlowSampleData.balance(available: 0, inFlight: 50_000_000))
        case .fundCabalEmpty:
            SampleAPIScript(balance: MoneyFlowSampleData.balance(available: 0, inFlight: 0))
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
