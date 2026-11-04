import MonacoFlows
import XCTest

extension Flow22dOutcome: WireOutcome {}

final class Flow22dOutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow22dOutcome.self)
    }
}
