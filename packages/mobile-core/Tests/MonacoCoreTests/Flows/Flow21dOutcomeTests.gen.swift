import MonacoFlows
import XCTest

extension Flow21dOutcome: WireOutcome {}

final class Flow21dOutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow21dOutcome.self)
    }
}
