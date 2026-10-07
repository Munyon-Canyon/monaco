import MonacoFlows
import XCTest

extension Flow27Outcome: WireOutcome {}

final class Flow27OutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow27Outcome.self)
    }
}
