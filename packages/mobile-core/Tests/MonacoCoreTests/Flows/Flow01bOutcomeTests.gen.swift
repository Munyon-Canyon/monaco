import MonacoFlows
import XCTest

extension Flow01bOutcome: WireOutcome {}

final class Flow01bOutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow01bOutcome.self)
    }
}
