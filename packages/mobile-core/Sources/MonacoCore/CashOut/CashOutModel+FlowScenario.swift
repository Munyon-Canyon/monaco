#if DEBUG
import Foundation
import MonacoAPI
import MonacoFlows

extension CashOutModel {
    public static func preview(answering scenario: Flow14Scenario) -> CashOutModel {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        let reads = try? encoder.encode(Components.Schemas.CashOutPreview.sample)
        return CashOutModel(
            cabalID: "01890a5d-ac96-774b-bcce-b302099a8059",
            api: FlowScenarioTransport.client(scenario.answer, reads: reads),
            hints: FlowScenarioHints())
    }
}

extension Flow14Scenario {
    fileprivate var answer: FlowScenarioAnswer {
        switch self {
        case .invalidInput:
            .problem(status: 400, code: "invalid_input", message: "The request is not valid.")
        case .insufficientShares:
            .problem(status: 409, code: "insufficient_shares", message: "You hold fewer shares than that.")
        case .cashOutInProgress:
            .problem(status: 409, code: "cash_out_in_progress", message: "A cash out is already running.")
        case .cabalPaused:
            .problem(status: 409, code: "cabal_paused", message: "This cabal is paused.")
        case .priceUnavailable:
            .problem(status: 503, code: "price_unavailable", message: "Prices are unavailable. Try again.")
        case .potValueChanged:
            .problem(status: 409, code: "pot_value_changed", message: "The pot moved. Check the amount.")
        case .privyUnavailable:
            .problem(status: 503, code: "privy_unavailable", message: "Wallet signing is down. Try again.")
        case .rPCUnavailable:
            .problem(status: 503, code: "rpc_unavailable", message: "The network is unavailable. Try again.")
        case .saleShort:
            .problem(status: 409, code: "sale_short", message: "The sale fell short. Try again.")
        case .interrupted:
            .dropped
        }
    }
}
#endif
