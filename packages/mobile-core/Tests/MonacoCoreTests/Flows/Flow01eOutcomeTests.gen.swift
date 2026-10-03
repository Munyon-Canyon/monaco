import MonacoFlows
import XCTest

extension Flow01eOutcome: WireOutcome {}

final class Flow01eOutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow01eOutcome.self)
    }
}
