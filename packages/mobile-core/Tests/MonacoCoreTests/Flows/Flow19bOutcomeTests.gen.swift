import MonacoFlows
import XCTest

extension Flow19bOutcome: WireOutcome {}

final class Flow19bOutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow19bOutcome.self)
    }
}
