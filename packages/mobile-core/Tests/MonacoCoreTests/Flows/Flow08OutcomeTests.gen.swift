import MonacoFlows
import XCTest

extension Flow08Outcome: WireOutcome {}

final class Flow08OutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow08Outcome.self)
    }
}
