import MonacoFlows
import XCTest

protocol WireOutcome: CaseIterable, Equatable {
    static var flowID: String { get }
    static var commands: [String] { get }
    var code: String? { get }
    init?(code: String)
}

final class FlowOutcomeTests: XCTestCase {
    func testOkAndInterruptedCarryNoWireCode() {
        XCTAssertNil(Flow01Outcome.ok.code)
        XCTAssertNil(Flow01Outcome.interrupted.code)
        XCTAssertEqual(Flow01Outcome.unauthorized.code, "unauthorized")
        XCTAssertEqual(Flow01Outcome.allCases.last, .interrupted)
        XCTAssertEqual(Flow01Outcome.commands, ["OpenSession"])
    }

    func testPlannedFlowCarriesItsRowCommandAndMapsItsWireCodes() {
        XCTAssertEqual(Flow11Outcome.commands, ["ExecuteTrade"])
        XCTAssertEqual(Flow11Outcome(code: "slippage_exceeded"), .slippageExceeded)
        XCTAssertEqual(Flow11Outcome(code: "insufficient_funds"), .insufficientFunds)
        XCTAssertEqual(Flow11Outcome(code: "cabal_paused"), .cabalPaused)
        assertRoundTrip(Flow11Outcome.self)
    }
}

extension XCTestCase {
    func assertRoundTrip<Outcome: WireOutcome>(
        _ type: Outcome.Type, file: StaticString = #filePath, line: UInt = #line
    ) {
        XCTAssertFalse(type.commands.isEmpty, file: file, line: line)
        XCTAssertFalse(type.commands.contains(where: \.isEmpty), file: file, line: line)
        assertCodesRoundTrip(type, file: file, line: line)
    }

    func assertCodesRoundTrip<Outcome: WireOutcome>(
        _ type: Outcome.Type, file: StaticString = #filePath, line: UInt = #line
    ) {
        XCTAssertFalse(type.flowID.isEmpty, file: file, line: line)
        for outcome in type.allCases {
            if let code = outcome.code {
                XCTAssertEqual(type.init(code: code), outcome, "\(type) \(code)", file: file, line: line)
            }
        }
        XCTAssertNil(type.init(code: "no_such_code"), "\(type)", file: file, line: line)
    }
}
