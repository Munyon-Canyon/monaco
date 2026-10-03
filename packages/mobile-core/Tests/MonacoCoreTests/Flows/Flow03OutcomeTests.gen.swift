import MonacoFlows
import XCTest

extension Flow03Outcome: WireOutcome {}

final class Flow03OutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow03Outcome.self)
    }
}
