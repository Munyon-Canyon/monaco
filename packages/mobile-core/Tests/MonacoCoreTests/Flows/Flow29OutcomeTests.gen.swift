import MonacoFlows
import XCTest

extension Flow29Outcome: WireOutcome {}

final class Flow29OutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow29Outcome.self)
    }
}
