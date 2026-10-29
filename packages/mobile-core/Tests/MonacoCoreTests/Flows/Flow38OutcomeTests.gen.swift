import MonacoFlows
import XCTest

extension Flow38Outcome: WireOutcome {}

final class Flow38OutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow38Outcome.self)
    }
}
