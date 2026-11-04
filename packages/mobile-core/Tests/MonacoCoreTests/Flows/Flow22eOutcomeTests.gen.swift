import MonacoFlows
import XCTest

extension Flow22eOutcome: WireOutcome {}

final class Flow22eOutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow22eOutcome.self)
    }
}
