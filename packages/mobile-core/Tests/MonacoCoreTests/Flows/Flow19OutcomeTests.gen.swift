import MonacoFlows
import XCTest

extension Flow19Outcome: WireOutcome {}

final class Flow19OutcomeTests: XCTestCase {
    func testEveryWireCodeMapsBackToItsOutcome() {
        assertRoundTrip(Flow19Outcome.self)
    }
}
