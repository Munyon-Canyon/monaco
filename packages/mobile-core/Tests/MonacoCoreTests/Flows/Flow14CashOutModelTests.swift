import MonacoCore
import MonacoFlows
import XCTest

final class Flow14CashOutModelTests: XCTestCase {
    func testEveryContractCodeIsMappedExplicitly() {
        let expected: [Flow14Outcome: CashOutRefusal] = [
            .invalidInput: .invalidRequest,
            .insufficientShares: .sliceChanged,
            .cashOutInProgress: .inProgress,
            .cabalPaused: .paused,
            .priceUnavailable: .sliceChanged,
            .potValueChanged: .sliceChanged,
            .privyUnavailable: .message("Server says no."),
            .rPCUnavailable: .message("Server says no."),
            .saleShort: .message("Server says no."),
        ]
        for outcome in Flow14Outcome.allCases {
            guard let code = outcome.code else { continue }
            XCTAssertEqual(CashOutRefusal(flowProblem(code)), expected[outcome], code)
        }
    }

    func testAnInterruptedRequestIsUnconfirmed() {
        XCTAssertEqual(CashOutRefusal(.transport(URLError(.networkConnectionLost))), .unconfirmed)
        XCTAssertEqual(CashOutRefusal(.decoding("bad body")), .unconfirmed)
    }

    func testACodeOutsideTheContractToastsTheServerMessage() {
        XCTAssertEqual(CashOutRefusal(.signedOut), .message("Please sign in again."))
    }

    #if DEBUG
    @MainActor
    func testEachFlowScenarioEndsTheCashOutInItsAction() async {
        for scenario in Flow14Scenario.allCases {
            let model = CashOutModel.preview(answering: scenario)
            await model.load()

            let result = await model.submit(enteredMicros: 1_000_000)

            let message: String
            switch scenario {
            case .invalidInput: message = CashOutModel.invalidRequest
            case .insufficientShares, .priceUnavailable, .potValueChanged: message = CashOutModel.sliceChanged
            case .cashOutInProgress: message = CashOutModel.inProgress
            case .cabalPaused: message = CabalPause(cause: .ops).message
            case .privyUnavailable: message = "Wallet signing is down. Try again."
            case .rPCUnavailable: message = "The network is unavailable. Try again."
            case .saleShort: message = "The sale fell short. Try again."
            case .interrupted: message = MoneyFlowCopy.unconfirmed.summary
            }
            XCTAssertEqual(result, .refused(message), "\(scenario)")
        }
    }

    func testFlowScenarioMatchesOnlyItsOwnFlowID() {
        XCTAssertEqual(Flow14Scenario.matching(["-MonacoFlow", "14", "saleShort"]), .saleShort)
        XCTAssertNil(Flow14Scenario.matching(["-MonacoFlow", "07", "saleShort"]))
        XCTAssertNil(Flow14Scenario.matching(["-MonacoFlow", "14", "ok"]))
    }
    #endif
}
