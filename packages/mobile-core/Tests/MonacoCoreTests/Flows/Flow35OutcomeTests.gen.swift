import MonacoFlows
import XCTest

extension Flow35Outcome: WireOutcome {}

final class Flow35OutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow35Outcome.self)
    }
}
