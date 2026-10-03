import MonacoFlows
import XCTest

extension Flow01cOutcome: WireOutcome {}

final class Flow01cOutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow01cOutcome.self)
    }
}
