import MonacoFlows
import XCTest

extension Flow37Outcome: WireOutcome {}

final class Flow37OutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow37Outcome.self)
    }
}
