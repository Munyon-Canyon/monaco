import MonacoFlows
import XCTest

extension Flow10aOutcome: WireOutcome {}

final class Flow10aOutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow10aOutcome.self)
    }
}
