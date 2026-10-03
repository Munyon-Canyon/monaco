import MonacoFlows
import XCTest

extension Flow22Outcome: WireOutcome {}

final class Flow22OutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow22Outcome.self)
    }
}
