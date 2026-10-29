import MonacoFlows
import XCTest

extension Flow33Outcome: WireOutcome {}

final class Flow33OutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow33Outcome.self)
    }
}
