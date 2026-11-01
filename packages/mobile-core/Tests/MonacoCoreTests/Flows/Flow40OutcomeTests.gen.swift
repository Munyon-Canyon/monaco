import MonacoFlows
import XCTest

extension Flow40Outcome: WireOutcome {}

final class Flow40OutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow40Outcome.self)
    }
}
