import MonacoCore
import MonacoFlows
import XCTest

final class Flow15WithdrawModelTests: XCTestCase {
    func testEveryContractCodeIsMappedExplicitly() {
        let expected: [Flow15Outcome: WithdrawFailure] = [
            .invalidInput: .toast("The minimum is $1."),
            .invalidAddress: .address("Paste a wallet address. That one can't receive USDC from here."),
            .withdrawToOwnWallet: .address(
                "That's your own deposit address. Paste the address you want to send to."),
            .insufficientFunds: .toast("Not enough in your account balance."),
            .privyUnavailable: .toast("Server says no."),
            .stepUpRequired: .toast("Server says no."),
            .stepUpMismatch: .toast("Server says no."),
        ]
        for outcome in Flow15Outcome.allCases {
            guard let code = outcome.code else { continue }
            let failure = WithdrawFailure(flowProblem(code))
            XCTAssertEqual(failure, expected[outcome], code)
        }
    }

    func testAnInterruptedRequestToastsTheOfflineLine() {
        XCTAssertEqual(
            WithdrawFailure(.transport(URLError(.networkConnectionLost))), .toast("You're offline. Try again."))
    }

    func testACodeOutsideTheContractToastsTheServerMessage() {
        XCTAssertEqual(WithdrawFailure(.signedOut), .toast("Please sign in again."))
    }

    #if DEBUG
    @MainActor
    func testEachFlowScenarioEndsTheWithdrawInItsAction() async {
        for scenario in Flow15Scenario.allCases {
            let withdrawing = Withdrawing.preview(answering: scenario)

            let attempt = await withdrawing.submit(micros: 100_000_000, toAddress: "address")

            switch scenario {
            case .invalidInput:
                XCTAssertEqual(attempt, .refused(.toast("The minimum is $1.")))
            case .invalidAddress:
                XCTAssertEqual(
                    attempt, .refused(.address("Paste a wallet address. That one can't receive USDC from here.")))
            case .withdrawToOwnWallet:
                XCTAssertEqual(
                    attempt,
                    .refused(.address("That's your own deposit address. Paste the address you want to send to.")))
            case .insufficientFunds:
                XCTAssertEqual(attempt, .refused(.toast("Not enough in your account balance.")))
            case .privyUnavailable:
                XCTAssertEqual(attempt, .unconfirmed(retryMessage: "Wallet signing is down. Try again."))
            case .stepUpRequired:
                XCTAssertEqual(attempt, .refused(.toast("Confirm this withdrawal to continue.")))
            case .stepUpMismatch:
                XCTAssertEqual(
                    attempt,
                    .refused(
                        .toast("The amount or address changed after you confirmed. Confirm the withdrawal again.")))
            case .interrupted:
                guard case .unconfirmed = attempt else {
                    XCTFail("expected an unconfirmed attempt, got \(attempt)")
                    continue
                }
            }
        }
    }

    func testFlowScenarioMatchesOnlyItsOwnFlowID() {
        XCTAssertEqual(Flow15Scenario.matching(["-MonacoFlow", "15", "invalidAddress"]), .invalidAddress)
        XCTAssertNil(Flow15Scenario.matching(["-MonacoFlow", "07", "invalidAddress"]))
        XCTAssertNil(Flow15Scenario.matching(["-MonacoFlow", "15", "ok"]))
    }
    #endif
}
