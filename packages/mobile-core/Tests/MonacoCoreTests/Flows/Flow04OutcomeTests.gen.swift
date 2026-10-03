import MonacoFlows
import XCTest

extension Flow04Outcome: WireOutcome {}

final class Flow04OutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow04Outcome.self)
    }
}
