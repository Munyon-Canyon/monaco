import MonacoFlows
import XCTest

extension Flow01Outcome: WireOutcome {}

final class Flow01OutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow01Outcome.self)
    }
}
