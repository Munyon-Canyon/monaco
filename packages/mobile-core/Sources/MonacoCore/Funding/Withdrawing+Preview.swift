#if DEBUG
import Foundation
import MonacoAPI
import MonacoFlows

extension Withdrawing {
    public static func preview(answering scenario: Flow15Scenario) -> Withdrawing {
        Withdrawing(
            source: WithdrawSource(api: FlowScenarioTransport.client(scenario.answer)),
            hints: FlowScenarioHints())
    }
}

extension Flow15Scenario {
    fileprivate var answer: FlowScenarioAnswer {
        switch self {
        case .invalidInput:
            .problem(status: 400, code: "invalid_input", message: "The request is not valid.")
        case .invalidAddress:
            .problem(status: 422, code: "invalid_address", message: "That address is not valid.")
        case .withdrawToOwnWallet:
            .problem(status: 422, code: "withdraw_to_own_wallet", message: "That is your own wallet.")
        case .insufficientFunds:
            .problem(status: 422, code: "insufficient_funds", message: "Not enough in your account balance.")
        case .privyUnavailable:
            .problem(status: 503, code: "privy_unavailable", message: "Wallet signing is down. Try again.")
        case .interrupted:
            .dropped
        }
    }
}
#endif
