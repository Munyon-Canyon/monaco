import MonacoFlows
import XCTest

extension Flow14Outcome: WireOutcome {}

final class Flow14OutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow14Outcome.self)
    }
}
