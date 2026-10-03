import MonacoFlows
import XCTest

extension Flow13Outcome: WireOutcome {}

final class Flow13OutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow13Outcome.self)
    }
}
