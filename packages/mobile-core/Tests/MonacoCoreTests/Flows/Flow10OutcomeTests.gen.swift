import MonacoFlows
import XCTest

extension Flow10Outcome: WireOutcome {}

final class Flow10OutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow10Outcome.self)
    }
}
