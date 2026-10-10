import MonacoCore
import MonacoFlows
import XCTest

#if DEBUG
@MainActor
final class BlockedPeopleFlowScenarioTests: XCTestCase {
    func testEachFlowScenarioRefusesTheUnblockAndKeepsTheRow() async {
        for scenario in Flow20aScenario.allCases {
            let model = BlockedPeopleModel.preview(answering: scenario)
            await model.load()
            guard case .loaded(let people) = model.state, let first = people.first else {
                return XCTFail("\(scenario): want rows, got \(model.state)")
            }

            let outcome = await model.unblock(userID: first.id)

            let expected: Flow20aOutcome
            switch scenario {
            case .cannotBlockSelf: expected = .cannotBlockSelf
            case .userNotFound: expected = .userNotFound
            case .reportTargetNotFound: expected = .reportTargetNotFound
            case .rateLimited: expected = .rateLimited
            case .unauthorized: expected = .unauthorized
            case .interrupted: expected = .interrupted
            }
            XCTAssertEqual(outcome, expected, "\(scenario)")
            guard case .loaded(let after) = model.state else { return XCTFail("\(scenario): lost the list") }
            XCTAssertEqual(after.map(\.id), people.map(\.id), "\(scenario)")
        }
    }

    func testThePreviewUnblocksOnSuccess() async {
        let model = BlockedPeopleModel.preview()
        await model.load()
        guard case .loaded(let people) = model.state, let first = people.first else {
            return XCTFail("want rows, got \(model.state)")
        }

        let outcome = await model.unblock(userID: first.id)

        XCTAssertEqual(outcome, .ok)
        guard case .loaded(let after) = model.state else { return XCTFail("lost the list") }
        XCTAssertEqual(after.count, people.count - 1)
    }

    func testTheEmptyPreviewHasNoRows() async {
        let model = BlockedPeopleModel.preview(empty: true)

        await model.load()

        guard case .loaded(let people) = model.state else { return XCTFail("want rows, got \(model.state)") }
        XCTAssertTrue(people.isEmpty)
    }

    func testFlowScenarioMatchesOnlyItsOwnFlowID() {
        XCTAssertEqual(Flow20aScenario.matching(["-MonacoFlow", "20a", "rateLimited"]), .rateLimited)
        XCTAssertNil(Flow20aScenario.matching(["-MonacoFlow", "20", "rateLimited"]))
        XCTAssertNil(Flow20aScenario.matching(["-MonacoFlow", "20a", "ok"]))
    }
}
#endif
