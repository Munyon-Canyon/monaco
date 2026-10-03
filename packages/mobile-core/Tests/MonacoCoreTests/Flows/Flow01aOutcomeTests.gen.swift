import MonacoFlows
import XCTest

extension Flow01aOutcome: WireOutcome {}

final class Flow01aOutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow01aOutcome.self)
    }
}
