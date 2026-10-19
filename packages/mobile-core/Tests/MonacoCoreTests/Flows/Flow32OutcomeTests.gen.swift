import MonacoFlows
import XCTest

extension Flow32Outcome: WireOutcome {}

final class Flow32OutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow32Outcome.self)
    }
}
