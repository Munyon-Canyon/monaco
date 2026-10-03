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

            guard case .failed(let error) = model.state else {
                XCTFail("\(scenario): expected a failed send, got \(model.state)")
                continue
            }
            switch scenario {
            case .invalidInput:
                guard case .problem(let problem) = error else {
                    XCTFail("invalidInput: expected a problem, got \(error)")
                    continue
                }
                XCTAssertEqual(problem.status, 422)
                XCTAssertEqual(Flow00Outcome(code: problem.code.wire), .invalidInput)
            case .unauthorized:
                XCTAssertEqual(error, .signedOut)
            case .interrupted:
                XCTAssertEqual(error, .transport(URLError(.networkConnectionLost)))
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
