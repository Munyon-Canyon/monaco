import MonacoFlows
import XCTest

extension Flow09Outcome: WireOutcome {}

final class Flow09OutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow09Outcome.self)
    }
}
