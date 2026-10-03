import MonacoFlows
import XCTest

extension Flow11Outcome: WireOutcome {}

final class Flow11OutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow11Outcome.self)
    }
}
