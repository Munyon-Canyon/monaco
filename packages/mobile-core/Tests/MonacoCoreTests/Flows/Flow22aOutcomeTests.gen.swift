import MonacoFlows
import XCTest

extension Flow22aOutcome: WireOutcome {}

final class Flow22aOutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow22aOutcome.self)
    }
}
