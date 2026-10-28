#if DEBUG
import Foundation
import MonacoAPI
import MonacoFlows

extension Funding {
    public nonisolated static let previewCabalID = "01890a5d-ac96-774b-bcce-b302099a8059"

    public static func preview(answering scenario: Flow07Scenario) -> Funding {
        Funding(
            cabalID: previewCabalID,
            source: FundSource(api: FlowScenarioTransport.client(scenario.answer)),
            hints: FlowScenarioHints())
    }
}

extension Flow07Scenario {
    fileprivate var answer: FlowScenarioAnswer {
        switch self {
        case .invalidInput:
            .problem(status: 400, code: "invalid_input", message: "The request is not valid.")
        case .notCabalMember:
            .problem(status: 403, code: "not_cabal_member", message: "You are not a member of this cabal.")
        case .insufficientFunds:
            .problem(status: 422, code: "insufficient_funds", message: "Not enough in your account balance.")
        case .cabalPaused:
            .problem(status: 409, code: "cabal_paused", message: "This cabal is paused.")
        case .cabalBanned:
            .problem(
                status: 422, code: "cabal_banned",
                message: "This cabal is banned. Members can still cash out and leave.")
        case .potValueZero:
            .problem(status: 409, code: "pot_value_zero", message: "The pot has no value right now.")
        case .privyUnavailable:
            .problem(status: 503, code: "privy_unavailable", message: "Wallet signing is down. Try again.")
        case .interrupted:
            .dropped
        }
    }
}
#endif
