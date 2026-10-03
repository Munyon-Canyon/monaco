import MonacoFlows
import XCTest

extension Flow00Outcome: WireOutcome {}

final class Flow00OutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow00Outcome.self)
    }
}
