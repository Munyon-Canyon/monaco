import MonacoFlows
import XCTest

extension Flow10bOutcome: WireOutcome {}

final class Flow10bOutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow10bOutcome.self)
    }
}
