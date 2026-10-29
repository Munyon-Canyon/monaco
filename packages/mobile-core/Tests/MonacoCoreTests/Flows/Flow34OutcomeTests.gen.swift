import MonacoFlows
import XCTest

extension Flow34Outcome: WireOutcome {}

final class Flow34OutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow34Outcome.self)
    }
}
