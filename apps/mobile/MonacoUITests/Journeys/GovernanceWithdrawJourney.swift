import XCTest

enum GovernanceWithdrawJourney {
    static let id = "governance/withdraw"
    static let version = 2

    static let screenTimeout: TimeInterval = 15
    static let checkTimeout: TimeInterval = 10

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func proposerWithdraws(
        _ app: XCUIApplication, cabalName: String, proposalID: String, recorder: JourneyRecorder
    ) {
        let card = GovernanceVoteJourney.card(app, proposalID)

        recorder.step("S1.1", "the proposer opens the cabal") {
            JoinJourney.openBySearch(app, cabalName, step: "S1.1")
        }

        recorder.step("S1.2", "the proposer opens the proposal from the cabal") {
            app.scrollIntoReach(card)
            XCTAssertTrue(
                card.waitForExistence(timeout: screenTimeout),
                "S1.2: proposal-card-\(proposalID) is not in the cabal's proposals within \(Int(screenTimeout)) s")
            card.tap()
            GovernanceVoteJourney.waitForProposalScreen(app, step: "S1.2")
        }

        recorder.step("S1.3", "Withdraw proposal asks to confirm") {
            let withdraw = app.buttons["Withdraw proposal"]
            app.scrollIntoReach(withdraw)
            XCTAssertTrue(
                withdraw.waitForExistence(timeout: checkTimeout),
                "S1.3: the Proposal screen shows no \"Withdraw proposal\" within \(Int(checkTimeout)) s")
            withdraw.tap()
            GovernanceVoteJourney.expectText(app, "Withdraw this proposal?", step: "S1.3")
            GovernanceVoteJourney.expectText(app, "Votes so far are dropped.", step: "S1.3")
        }

        recorder.step("S1.4", "confirming withdraws it") {
            let confirm = app.buttons.matching(NSPredicate(format: "label == 'Withdraw'")).firstMatch
            XCTAssertTrue(confirm.waitForExistence(timeout: checkTimeout), "S1.4: the confirm shows no \"Withdraw\"")
            confirm.tap()
            JoinJourney.waitForToast(app, "Proposal withdrawn.", step: "S1.4")
            GovernanceVoteJourney.expectText(card, "Withdrawn", step: "S1.4")
        }
    }

    static func voterNoLongerAsked(_ app: XCUIApplication, proposalID: String, recorder: JourneyRecorder) {
        recorder.step("S1.5", "the withdrawn proposal is not on B's Needs your vote") {
            XCTAssertTrue(
                app.tab("Home").waitForExistence(timeout: screenTimeout),
                "S1.5: the tab bar did not show within \(Int(screenTimeout)) s")
            app.tab("Home").tap()
            XCTAssertTrue(
                GovernanceVoteJourney.waitForGone(GovernanceVoteJourney.card(app, proposalID), timeout: screenTimeout),
                "S1.5: proposal-card-\(proposalID) is still on B's Home after the withdraw")
        }
    }
}
