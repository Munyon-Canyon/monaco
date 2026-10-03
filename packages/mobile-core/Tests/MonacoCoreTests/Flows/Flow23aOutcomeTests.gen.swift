import MonacoFlows
import XCTest

extension Flow23aOutcome: WireOutcome {}

final class Flow23aOutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow23aOutcome.self)
    }
}
