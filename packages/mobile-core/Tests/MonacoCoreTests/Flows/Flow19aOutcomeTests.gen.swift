import MonacoFlows
import XCTest

extension Flow19aOutcome: WireOutcome {}

final class Flow19aOutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow19aOutcome.self)
    }
}
