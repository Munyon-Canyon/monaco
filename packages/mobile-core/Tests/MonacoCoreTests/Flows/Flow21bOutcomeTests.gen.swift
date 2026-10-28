import MonacoFlows
import XCTest

extension Flow21bOutcome: WireOutcome {}

final class Flow21bOutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow21bOutcome.self)
    }
}
