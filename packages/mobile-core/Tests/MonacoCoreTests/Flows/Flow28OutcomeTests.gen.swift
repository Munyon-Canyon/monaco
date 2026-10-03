import MonacoFlows
import XCTest

extension Flow28Outcome: WireOutcome {}

final class Flow28OutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow28Outcome.self)
    }
}
