import MonacoFlows
import XCTest

extension Flow01dOutcome: WireOutcome {}

final class Flow01dOutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow01dOutcome.self)
    }
}
