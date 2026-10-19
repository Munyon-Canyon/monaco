import XCTest

enum GovernanceVoteJourney {
    static let id = "governance/vote"
    static let version = 4

    static let screenTimeout: TimeInterval = 15
    static let checkTimeout: TimeInterval = 10

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func card(_ app: XCUIApplication, _ proposalID: String) -> XCUIElement {
        app.element("proposal-card-\(proposalID)")
    }

    static func text(_ scope: XCUIElement, containing value: String) -> XCUIElement {
        scope.descendants(matching: .any).matching(NSPredicate(format: "label CONTAINS %@", value)).firstMatch
    }

    static func openProposal(_ card: XCUIElement) {
        card.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 1)).withOffset(CGVector(dx: 0, dy: -22)).tap()
    }

    static func waitForText(_ scope: XCUIElement, _ value: String, timeout: TimeInterval) -> Bool {
        let deadline = Date().addingTimeInterval(timeout)
        repeat {
            if scope.exists && scope.label.contains(value) { return true }
            if text(scope, containing: value).exists { return true }
            RunLoop.current.run(until: Date().addingTimeInterval(0.2))
        } while Date() < deadline
        return false
    }

    static func expectText(_ scope: XCUIElement, _ value: String, step: String) {
        XCTAssertTrue(
            waitForText(scope, value, timeout: checkTimeout),
            "\(step): \"\(value)\" did not show within \(Int(checkTimeout)) s")
    }

    static func expectVoteBar(_ app: XCUIApplication, _ tally: String, step: String) {
        XCTAssertTrue(
            waitForText(app, "\(tally). ", timeout: checkTimeout),
            "\(step): the vote bar does not read \"\(tally)\" within \(Int(checkTimeout)) s")
    }

    static func waitForGone(_ element: XCUIElement, timeout: TimeInterval) -> Bool {
        let deadline = Date().addingTimeInterval(timeout)
        while element.exists && Date() < deadline {
            RunLoop.current.run(until: Date().addingTimeInterval(0.2))
        }
        return !element.exists
    }

    static func openHomeCard(_ app: XCUIApplication, _ proposalID: String, step: String) {
        XCTAssertTrue(
            app.tab("Home").waitForExistence(timeout: screenTimeout),
            "\(step): the tab bar did not show within \(Int(screenTimeout)) s")
        app.tab("Home").tap()
        XCTAssertTrue(
            waitForText(app, "Needs your vote", timeout: screenTimeout),
            "\(step): Home shows no \"Needs your vote\" within \(Int(screenTimeout)) s")
        let target = card(app, proposalID)
        app.scrollIntoReach(target)
        XCTAssertTrue(
            target.waitForExistence(timeout: screenTimeout),
            "\(step): proposal-card-\(proposalID) is not under \"Needs your vote\" within \(Int(screenTimeout)) s")
    }

    static func waitForProposalScreen(_ app: XCUIApplication, step: String) {
        XCTAssertTrue(
            waitForText(app, "Votes", timeout: screenTimeout),
            "\(step): the Proposal screen shows no \"Votes\" within \(Int(screenTimeout)) s")
        XCTAssertTrue(
            waitForText(app, "Thesis", timeout: screenTimeout),
            "\(step): the Proposal screen shows no \"Thesis\" within \(Int(screenTimeout)) s")
    }

    static func vote(
        _ app: XCUIApplication, _ proposalID: String, _ choice: String, change: Bool, step: String
    ) {
        if change {
            let changeButton = app.buttons["Change vote"].firstMatch
            XCTAssertTrue(
                changeButton.waitForExistence(timeout: checkTimeout), "\(step): the dock shows no \"Change vote\"")
            changeButton.tap()
        }
        let button = app.buttons["Vote \(choice)"].firstMatch
        XCTAssertTrue(
            button.waitForExistence(timeout: checkTimeout), "\(step): the dock shows no \"Vote \(choice)\"")
        button.tap()
        JoinJourney.waitForToast(app, "Vote recorded.", step: step)
    }

    static func voteFromHome(_ app: XCUIApplication, proposalID: String, run: String, recorder: JourneyRecorder) {
        let target = card(app, proposalID)

        recorder.step("S1.1", "Home lists the proposal under Needs your vote") {
            openHomeCard(app, proposalID, step: "S1.1")
            expectText(target, "Needs your vote", step: "S1.1")
            expectText(target, "2 yes votes needed", step: "S1.1")
            expectText(target, "Closes in", step: "S1.1")
            XCTAssertFalse(app.buttons["Yes"].exists, "S1.1: the card shows an inline \"Yes\"")
        }

        recorder.step("S1.2", "the card opens the Proposal screen") {
            openProposal(target)
            waitForProposalScreen(app, step: "S1.2")
            XCTAssertTrue(
                waitForText(app, "QA vote \(run)", timeout: checkTimeout),
                "S1.2: the reason does not read QA vote \(run)")
        }

        recorder.step("S1.3", "vote yes") {
            vote(app, proposalID, "Yes", change: false, step: "S1.3")
            XCTAssertTrue(
                app.buttons["proposal-change-vote"].waitForExistence(timeout: checkTimeout),
                "S1.3: the dock shows no \"Change vote\"")
            expectVoteBar(app, "1 yes, 0 no, 2 not voted", step: "S1.3")
        }

        recorder.step("S1.4", "change the ballot to no") {
            vote(app, proposalID, "No", change: true, step: "S1.4")
            expectVoteBar(app, "0 yes, 1 no, 2 not voted", step: "S1.4")
        }

        recorder.step("S1.5", "change the ballot back to yes") {
            vote(app, proposalID, "Yes", change: true, step: "S1.5")
            expectVoteBar(app, "1 yes, 0 no, 2 not voted", step: "S1.5")
        }
    }

    static func secondVoterPasses(
        _ app: XCUIApplication, proposalID: String, voter: String, recorder: JourneyRecorder
    ) {
        let target = card(app, proposalID)

        recorder.step("S1.6", "B opens the proposal from Home and sees A's ballot") {
            openHomeCard(app, proposalID, step: "S1.6")
            openProposal(target)
            expectVoteBar(app, "1 yes, 0 no, 2 not voted", step: "S1.6")
            let seeAll = app.buttons["proposal-votes-see-all"].firstMatch
            XCTAssertTrue(
                seeAll.waitForExistence(timeout: screenTimeout), "S1.6: the Proposal screen shows no \"See all\"")
            seeAll.tap()
            XCTAssertTrue(
                waitForText(app, "\(voter) voted yes", timeout: screenTimeout),
                "S1.6: the Votes sheet does not show \"\(voter) voted yes\"")
            app.buttons["Close"].firstMatch.tap()
        }

        recorder.step("S1.7", "B's yes makes the majority and closes the vote") {
            vote(app, proposalID, "Yes", change: false, step: "S1.7")
            for label in ["Vote Yes", "Vote No", "Change vote"] {
                XCTAssertTrue(
                    waitForGone(app.buttons[label].firstMatch, timeout: screenTimeout),
                    "S1.7: the dock still shows \"\(label)\" \(Int(screenTimeout)) s after the majority")
            }
        }
    }

    static func firstVoterSeesItLeave(_ app: XCUIApplication, proposalID: String, recorder: JourneyRecorder) {
        recorder.step("S1.8", "the closed proposal leaves A's Needs your vote") {
            app.terminate()
            app.launch()
            XCTAssertTrue(
                app.tab("Home").waitForExistence(timeout: screenTimeout),
                "S1.8: the tab bar did not show within \(Int(screenTimeout)) s")
            app.tab("Home").tap()
            XCTAssertTrue(
                waitForGone(card(app, proposalID), timeout: screenTimeout),
                "S1.8: proposal-card-\(proposalID) is still on Home \(Int(screenTimeout)) s after the vote closed")
        }
    }

    static func openCabal(_ app: XCUIApplication, cabalName: String, proposalID: String, step: String) {
        JoinJourney.openBySearch(app, cabalName, step: step)
        XCTAssertTrue(
            waitForText(app, "Needs your vote", timeout: screenTimeout),
            "\(step): the cabal shows no \"Needs your vote\" within \(Int(screenTimeout)) s")
        let target = card(app, proposalID)
        app.scrollIntoReach(target)
        XCTAssertTrue(
            target.waitForExistence(timeout: screenTimeout),
            "\(step): proposal-card-\(proposalID) is not on the cabal within \(Int(screenTimeout)) s")
        XCTAssertTrue(app.buttons["See all"].exists, "\(step): the cabal's proposals show no \"See all\"")
    }

    static func voteOnCabalCard(
        _ app: XCUIApplication, cabalName: String, proposalID: String, recorder: JourneyRecorder
    ) {
        let target = card(app, proposalID)

        recorder.step("S2.1", "the cabal lists the proposal under Needs your vote") {
            openCabal(app, cabalName: cabalName, proposalID: proposalID, step: "S2.1")
        }

        recorder.step("S2.2", "the cabal's card opens the Proposal screen") {
            openProposal(target)
            waitForProposalScreen(app, step: "S2.2")
        }

        recorder.step("S2.3", "vote yes from the dock") {
            vote(app, proposalID, "Yes", change: false, step: "S2.3")
            XCTAssertTrue(
                app.buttons["proposal-change-vote"].waitForExistence(timeout: checkTimeout),
                "S2.3: the dock shows no \"Change vote\"")
        }
    }

    static func seeAllHistory(
        _ app: XCUIApplication, cabalName: String, proposalID: String, closedProposalID: String,
        recorder: JourneyRecorder
    ) {
        recorder.step("S3.1", "the cabal lists the proposal with See all") {
            openCabal(app, cabalName: cabalName, proposalID: proposalID, step: "S3.1")
        }

        recorder.step("S3.2", "See all lists the open proposal and, under Failed, the closed one") {
            app.buttons["See all"].tap()
            XCTAssertTrue(
                app.navigationBars["Proposals"].waitForExistence(timeout: screenTimeout),
                "S3.2: See all did not open the Proposals list within \(Int(screenTimeout)) s")
            XCTAssertTrue(
                card(app, proposalID).waitForExistence(timeout: screenTimeout),
                "S3.2: the open proposal-card-\(proposalID) is not in the Open list")
            app.buttons["Failed"].tap()
            let closed = card(app, closedProposalID)
            XCTAssertTrue(
                closed.waitForExistence(timeout: screenTimeout),
                "S3.2: the closed proposal-card-\(closedProposalID) did not show under Failed")
            expectText(closed, "Expired", step: "S3.2")
            expectText(closed, "Voting ended without enough yes votes", step: "S3.2")
        }
    }
}
