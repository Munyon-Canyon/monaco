import MonacoFlows
import XCTest

private protocol WireOutcome: CaseIterable, Equatable {
    static var flowID: String { get }
    static var commands: [String] { get }
    var code: String? { get }
    init?(code: String)
}

extension Flow00Outcome: WireOutcome {}
extension Flow01Outcome: WireOutcome {}
extension Flow01bOutcome: WireOutcome {}
extension Flow01cOutcome: WireOutcome {}
extension Flow01dOutcome: WireOutcome {}
extension Flow01eOutcome: WireOutcome {}
extension Flow02Outcome: WireOutcome {}
extension Flow03Outcome: WireOutcome {}
extension Flow05Outcome: WireOutcome {}
extension Flow06Outcome: WireOutcome {}
extension Flow10Outcome: WireOutcome {}
extension Flow11Outcome: WireOutcome {}
extension Flow13Outcome: WireOutcome {}
extension Flow13aOutcome: WireOutcome {}
extension Flow18Outcome: WireOutcome {}
extension Flow20Outcome: WireOutcome {}
extension Flow23Outcome: WireOutcome {}
extension Flow23aOutcome: WireOutcome {}
extension Flow28Outcome: WireOutcome {}

final class FlowOutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow00Outcome.self)
        assertRoundTrip(Flow01Outcome.self)
        assertRoundTrip(Flow01bOutcome.self)
        assertRoundTrip(Flow01cOutcome.self)
        assertRoundTrip(Flow01dOutcome.self)
        assertRoundTrip(Flow01eOutcome.self)
        assertRoundTrip(Flow02Outcome.self)
        assertRoundTrip(Flow03Outcome.self)
        assertRoundTrip(Flow05Outcome.self)
        assertRoundTrip(Flow06Outcome.self)
        assertRoundTrip(Flow10Outcome.self)
        assertRoundTrip(Flow13Outcome.self)
        assertRoundTrip(Flow13aOutcome.self)
        assertRoundTrip(Flow18Outcome.self)
        assertRoundTrip(Flow20Outcome.self)
        assertRoundTrip(Flow23Outcome.self)
        assertRoundTrip(Flow23aOutcome.self)
        assertRoundTrip(Flow28Outcome.self)
    }

    func testOkAndInterruptedCarryNoWireCode() {
        XCTAssertNil(Flow01Outcome.ok.code)
        XCTAssertNil(Flow01Outcome.interrupted.code)
        XCTAssertEqual(Flow01Outcome.unauthorized.code, "unauthorized")
        XCTAssertEqual(Flow01Outcome.allCases.last, .interrupted)
        XCTAssertEqual(Flow01Outcome.commands, ["OpenSession"])
    }

    func testPlannedFlowWithoutACommandStillMapsItsWireCodes() {
        XCTAssertEqual(Flow11Outcome.commands, [])
        XCTAssertEqual(Flow11Outcome(code: "slippage_exceeded"), .slippageExceeded)
        assertCodesRoundTrip(Flow11Outcome.self)
    }

    private func assertRoundTrip<Outcome: WireOutcome>(
        _ type: Outcome.Type, file: StaticString = #filePath, line: UInt = #line
    ) {
        XCTAssertFalse(type.commands.isEmpty, file: file, line: line)
        XCTAssertFalse(type.commands.contains(where: \.isEmpty), file: file, line: line)
        assertCodesRoundTrip(type, file: file, line: line)
    }

    private func assertCodesRoundTrip<Outcome: WireOutcome>(
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
