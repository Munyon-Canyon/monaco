import MonacoFlows
import XCTest

extension Flow20aOutcome: WireOutcome {}

final class Flow20aOutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow20aOutcome.self)
    }
}
