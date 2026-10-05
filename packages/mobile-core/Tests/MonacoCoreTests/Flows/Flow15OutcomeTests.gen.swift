import MonacoFlows
import XCTest

extension Flow15Outcome: WireOutcome {}

final class Flow15OutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow15Outcome.self)
    }
}
