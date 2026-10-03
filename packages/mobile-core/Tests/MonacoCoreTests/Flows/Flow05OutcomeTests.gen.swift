import MonacoFlows
import XCTest

extension Flow05Outcome: WireOutcome {}

final class Flow05OutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow05Outcome.self)
    }
}
