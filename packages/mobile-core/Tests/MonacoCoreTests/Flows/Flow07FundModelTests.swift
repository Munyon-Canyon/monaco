import MonacoCore
import MonacoFlows
import XCTest

final class Flow07FundModelTests: XCTestCase {
    func testEveryContractCodeIsMappedExplicitly() {
        let expected: [Flow07Outcome: FundFailure] = [
            .invalidInput: .toast("The minimum is $1."),
            .notCabalMember: .toast("Join this cabal to fund it."),
            .insufficientFunds: .needsMoney("Not enough in your account balance."),
            .cabalPaused: .toast("Trading in this cabal is paused. You can fund it again once it resumes."),
            .potValueZero: .toast("Server says no."),
            .privyUnavailable: .toast("Server says no."),
        ]
        for outcome in Flow07Outcome.allCases {
            guard let code = outcome.code else { continue }
            XCTAssertEqual(FundFailure(flowProblem(code)), expected[outcome], code)
        }
    }

    func testAnInterruptedRequestToastsTheOfflineLine() {
        XCTAssertEqual(
            FundFailure(.transport(URLError(.networkConnectionLost))), .toast("You're offline. Try again."))
    }

    func testACodeOutsideTheContractToastsTheServerMessage() {
        let failure = FundFailure(flowProblem("rate_limited", status: 429, message: "Slow down."))
        XCTAssertEqual(failure, .toast("Slow down."))
    }

    #if DEBUG
    @MainActor
    func testEachFlowScenarioEndsTheFundInItsAction() async {
        for scenario in Flow07Scenario.allCases {
            let funding = Funding.preview(answering: scenario)

            let attempt = await funding.submit(micros: 250_000_000)

            switch scenario {
            case .insufficientFunds:
                XCTAssertEqual(attempt, .refused(.needsMoney("Not enough in your account balance.")))
            case .invalidInput:
                XCTAssertEqual(attempt, .refused(.toast("The minimum is $1.")))
            case .notCabalMember:
                XCTAssertEqual(attempt, .refused(.toast("Join this cabal to fund it.")))
            case .cabalPaused:
                XCTAssertEqual(
                    attempt,
                    .refused(.toast("Trading in this cabal is paused. You can fund it again once it resumes.")))
            case .potValueZero:
                XCTAssertEqual(attempt, .refused(.toast("The pot has no value right now.")))
            case .privyUnavailable:
                XCTAssertEqual(attempt, .unconfirmed(retryMessage: "Wallet signing is down. Try again."))
            case .interrupted:
                guard case .unconfirmed = attempt else {
                    XCTFail("expected an unconfirmed attempt, got \(attempt)")
                    continue
                }
            }
        }
    }

    func testFlowScenarioMatchesOnlyItsOwnFlowID() {
        XCTAssertEqual(Flow07Scenario.matching(["-MonacoFlow", "07", "potValueZero"]), .potValueZero)
        XCTAssertNil(Flow07Scenario.matching(["-MonacoFlow", "14", "potValueZero"]))
        XCTAssertNil(Flow07Scenario.matching(["-MonacoFlow", "07", "ok"]))
    }
    #endif
}
