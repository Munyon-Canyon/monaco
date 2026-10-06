import MonacoFlows
import XCTest

extension Flow21Outcome: WireOutcome {}

final class Flow21OutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow21Outcome.self)
    }
}
