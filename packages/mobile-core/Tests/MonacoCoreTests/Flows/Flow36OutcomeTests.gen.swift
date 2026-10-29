import MonacoFlows
import XCTest

extension Flow36Outcome: WireOutcome {}

final class Flow36OutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow36Outcome.self)
    }
}
