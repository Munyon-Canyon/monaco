import MonacoFlows
import XCTest

extension Flow39Outcome: WireOutcome {}

final class Flow39OutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow39Outcome.self)
    }
}
