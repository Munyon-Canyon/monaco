import MonacoFlows
import XCTest

extension Flow12Outcome: WireOutcome {}

final class Flow12OutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow12Outcome.self)
    }
}
