import MonacoFlows
import XCTest

extension Flow22bOutcome: WireOutcome {}

final class Flow22bOutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow22bOutcome.self)
    }
}
