import MonacoCore
import XCTest

#if DEBUG
@MainActor
final class LeaveCabalFlowScenarioTests: XCTestCase {
    func testEachFlowScenarioEndsTheLeaveInItsAction() async {
        for scenario in Flow04Scenario.allCases {
            let model = LeaveCabalModel.preview(answering: scenario)
            await model.load()

            let outcome = await model.leave()

            switch scenario {
            case .leaveHoldsShares:
                XCTAssertEqual(
                    outcome, .cashOutFirst(message: "Cash out your share of the pot before you leave this cabal."))
            case .leaveLastMemberPotNotEmpty:
                XCTAssertEqual(
                    outcome, .refused(message: "The pot still holds money, so the last member cannot leave yet."))
            case .leaveCreatorWithMembers:
                XCTAssertEqual(outcome, .refused(message: "The creator cannot leave while other members remain."))
            case .notCabalMember:
                XCTAssertEqual(outcome, .refused(message: "You are not a member of this cabal."))
            case .priceUnavailable:
                XCTAssertEqual(
                    outcome, .refused(message: "Prices are temporarily unavailable. Try again in a moment."))
            case .unauthorized:
                XCTAssertEqual(outcome, .refused(message: "Please sign in again."))
            case .interrupted:
                XCTAssertEqual(outcome, .refused(message: "You're offline. Try again."))
            }
        }
    }

    func testFlowScenarioMatchesOnlyItsOwnFlowID() {
        XCTAssertEqual(Flow04Scenario.matching(["-MonacoFlow", "04", "leaveHoldsShares"]), .leaveHoldsShares)
        XCTAssertNil(Flow04Scenario.matching(["-MonacoFlow", "00", "leaveHoldsShares"]))
        XCTAssertNil(Flow04Scenario.matching(["-MonacoFlow", "04", "ok"]))
        XCTAssertNil(Flow04Scenario.matching(["-MonacoFlow", "04"]))
        XCTAssertNil(Flow04Scenario.matching(["-MonacoCabalLeaveSample", "member"]))
    }

    func testThePreviewHasNoHintsToWaitOn() async {
        let model = LeaveCabalModel.preview(role: "member", memberCount: 2)

        await model.observe()

        XCTAssertEqual(model.standing, .idle)
    }

    func testTheSampleMemberLeaves() async {
        let model = LeaveCabalModel.preview(role: "member", memberCount: 2)
        await model.load()

        let outcome = await model.leave()

        XCTAssertEqual(outcome, .left(cabalName: "QA pot"))
    }

    func testTheSampleCreatorWithMembersCannotLeave() async {
        let model = LeaveCabalModel.preview(role: "creator", memberCount: 2)

        await model.load()

        XCTAssertEqual(model.standing, .loaded(LeaveStanding(cabalName: "QA pot", canLeave: false)))
    }
}
#endif
