import Foundation
import MonacoAPI
import MonacoCore
import XCTest

#if DEBUG
@MainActor
final class SystemPingFlowScenarioTests: XCTestCase {
    func testEachFlowScenarioEndsTheSendInItsOutcome() async {
        for scenario in Flow00Scenario.allCases {
            let model = SystemPingModel.preview(answering: scenario)

            await model.send(note: "hi")

            switch scenario {
            case .invalidInput:
                XCTAssertEqual(model.state, .invalidInput(message: "The note is too long."))
            case .unauthorized:
                XCTAssertEqual(model.state, .unauthorized)
            case .interrupted:
                XCTAssertEqual(model.state, .failed(.transport(URLError(.networkConnectionLost))))
            }
            XCTAssertFalse(model.isSending, "\(scenario)")
        }
    }

    func testFlowScenarioMatchesOnlyItsOwnFlowID() {
        XCTAssertEqual(Flow00Scenario.matching(["-MonacoFlow", "00", "unauthorized"]), .unauthorized)
        XCTAssertNil(Flow00Scenario.matching(["-MonacoFlow", "01", "unauthorized"]))
        XCTAssertNil(Flow00Scenario.matching(["-MonacoFlow", "00", "ok"]))
        XCTAssertNil(Flow00Scenario.matching(["-MonacoFlow", "00"]))
        XCTAssertNil(Flow00Scenario.matching(["-MonacoHomeSample", "populated"]))
    }
}
#endif
