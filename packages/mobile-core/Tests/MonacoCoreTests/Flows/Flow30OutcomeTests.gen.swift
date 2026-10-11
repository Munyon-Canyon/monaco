import MonacoFlows
import XCTest

extension Flow30Outcome: WireOutcome {}

final class Flow30OutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertCodesRoundTrip(Flow30Outcome.self)
    }
}
