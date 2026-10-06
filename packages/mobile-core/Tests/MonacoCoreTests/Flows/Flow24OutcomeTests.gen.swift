import MonacoFlows
import XCTest

extension Flow24Outcome: WireOutcome {}

final class Flow24OutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow24Outcome.self)
    }
}
