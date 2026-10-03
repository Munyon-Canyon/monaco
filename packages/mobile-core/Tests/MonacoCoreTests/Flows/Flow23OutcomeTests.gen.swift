import MonacoFlows
import XCTest

extension Flow23Outcome: WireOutcome {}

final class Flow23OutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow23Outcome.self)
    }
}
