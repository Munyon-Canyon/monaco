import MonacoFlows
import XCTest

extension Flow02Outcome: WireOutcome {}

final class Flow02OutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow02Outcome.self)
    }
}
