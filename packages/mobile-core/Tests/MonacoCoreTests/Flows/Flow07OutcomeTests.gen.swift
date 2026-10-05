import MonacoFlows
import XCTest

extension Flow07Outcome: WireOutcome {}

final class Flow07OutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow07Outcome.self)
    }
}
