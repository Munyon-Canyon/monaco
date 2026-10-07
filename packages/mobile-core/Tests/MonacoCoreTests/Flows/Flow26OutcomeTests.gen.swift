import MonacoFlows
import XCTest

extension Flow26Outcome: WireOutcome {}

final class Flow26OutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow26Outcome.self)
    }
}
