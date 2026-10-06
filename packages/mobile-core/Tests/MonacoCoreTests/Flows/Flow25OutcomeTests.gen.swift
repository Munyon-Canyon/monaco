import MonacoFlows
import XCTest

extension Flow25Outcome: WireOutcome {}

final class Flow25OutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow25Outcome.self)
    }
}
