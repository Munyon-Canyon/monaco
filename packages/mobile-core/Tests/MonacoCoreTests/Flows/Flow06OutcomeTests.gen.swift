import MonacoFlows
import XCTest

extension Flow06Outcome: WireOutcome {}

final class Flow06OutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow06Outcome.self)
    }
}
