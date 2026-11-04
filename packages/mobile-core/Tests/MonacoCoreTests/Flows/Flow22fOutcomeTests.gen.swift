import MonacoFlows
import XCTest

extension Flow22fOutcome: WireOutcome {}

final class Flow22fOutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow22fOutcome.self)
    }
}
