import MonacoFlows
import XCTest

extension Flow13aOutcome: WireOutcome {}

final class Flow13aOutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow13aOutcome.self)
    }
}
