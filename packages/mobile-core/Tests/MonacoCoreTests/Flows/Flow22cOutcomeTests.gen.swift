import MonacoFlows
import XCTest

extension Flow22cOutcome: WireOutcome {}

final class Flow22cOutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow22cOutcome.self)
    }
}
