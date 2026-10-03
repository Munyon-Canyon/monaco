import MonacoFlows
import XCTest

extension Flow18Outcome: WireOutcome {}

final class Flow18OutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow18Outcome.self)
    }
}
