import MonacoFlows
import XCTest

extension Flow20Outcome: WireOutcome {}

final class Flow20OutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow20Outcome.self)
    }
}
