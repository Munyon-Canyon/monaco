import MonacoFlows
import XCTest

extension Flow11Outcome: WireOutcome {}

final class Flow11OutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertCodesRoundTrip(Flow11Outcome.self)
    }
}
