import XCTest

enum GovernanceVoteJourney {
    static let id = "governance/vote"
    static let version = 1

    static let screenTimeout: TimeInterval = 15
    static let checkTimeout: TimeInterval = 10

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func card(_ app: XCUIApplication, _ proposalID: String) -> XCUIElement {
        app.element("proposal-card-\(proposalID)")
    }

    static func text(_ scope: XCUIElement, containing value: String) -> XCUIElement {
        scope.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", value)).firstMatch
    }

    static func tapHeaderToOpenProposal(_ card: XCUIElement) {
        card.coordinate(withNormalizedOffset: CGVector(dx: 0.3, dy: 0)).withOffset(CGVector(dx: 0, dy: 36)).tap()
    }

    static func waitForText(_ scope: XCUIElement, _ value: String, timeout: TimeInterval) -> Bool {
        text(scope, containing: value).waitForExistence(timeout: timeout)
    }

    static func expectText(_ scope: XCUIElement, _ value: String, step: String) {
        XCTAssertTrue(
            waitForText(scope, value, timeout: checkTimeout),
            "\(step): \"\(value)\" did not show within \(Int(checkTimeout)) s")
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
            waitForText(app, "Why buy", timeout: screenTimeout),
            "\(step): the Proposal screen shows no \"Why buy\" within \(Int(screenTimeout)) s")
    }

    static func vote(
        _ app: XCUIApplication, _ proposalID: String, _ choice: String, change: Bool, step: String
    ) {
        let target = card(app, proposalID)
        if change {
            let changeButton = target.buttons["Change"]
            XCTAssertTrue(
                changeButton.waitForExistence(timeout: checkTimeout), "\(step): the card shows no \"Change\"")
            changeButton.tap()
        }
        let button = target.buttons[choice]
        XCTAssertTrue(button.waitForExistence(timeout: checkTimeout), "\(step): the card shows no \"\(choice)\"")
        button.tap()
        JoinJourney.waitForToast(app, "Vote in", step: step)
    }

    static func voteFromHome(_ app: XCUIApplication, proposalID: String, run: String, recorder: JourneyRecorder) {
        let target = card(app, proposalID)

        recorder.step("S1.1", "Home lists the proposal under Needs your vote") {
            openHomeCard(app, proposalID, step: "S1.1")
            expectText(target, "Closes in", step: "S1.1")
            XCTAssertTrue(
                waitForText(target, "0 of 3 voted · 2 yes to pass", timeout: checkTimeout),
                "S1.1: the card's tracker does not read \"0 of 3 voted · 2 yes to pass\"")
            XCTAssertTrue(target.buttons["Yes"].exists, "S1.1: the card shows no \"Yes\"")
            XCTAssertTrue(target.buttons["No"].exists, "S1.1: the card shows no \"No\"")
        }

        recorder.step("S1.2", "the card opens the Proposal screen") {
            tapHeaderToOpenProposal(target)
            waitForProposalScreen(app, step: "S1.2")
            XCTAssertTrue(
                waitForText(app, "“QA vote \(run)”", timeout: checkTimeout),
                "S1.2: the reason does not read “QA vote \(run)”")
        }

        recorder.step("S1.3", "vote yes") {
            vote(app, proposalID, "Yes", change: false, step: "S1.3")
            expectText(target, "✓ You voted yes", step: "S1.3")
            XCTAssertTrue(target.buttons["Change"].exists, "S1.3: the card shows no \"Change\"")
            XCTAssertTrue(
                waitForText(target, "1 of 3 voted · 2 yes to pass", timeout: checkTimeout),
                "S1.3: the tracker does not read \"1 of 3 voted · 2 yes to pass\"")
        }

        recorder.step("S1.4", "change the ballot to no") {
            vote(app, proposalID, "No", change: true, step: "S1.4")
            expectText(target, "✓ You voted no", step: "S1.4")
        }

        recorder.step("S1.5", "change the ballot back to yes") {
            vote(app, proposalID, "Yes", change: true, step: "S1.5")
            expectText(target, "✓ You voted yes", step: "S1.5")
        }
    }

    static func secondVoterPasses(
        _ app: XCUIApplication, proposalID: String, voter: String, recorder: JourneyRecorder
    ) {
        let target = card(app, proposalID)

        recorder.step("S1.6", "B opens the proposal from Home and sees A's ballot") {
            openHomeCard(app, proposalID, step: "S1.6")
            tapHeaderToOpenProposal(target)
            XCTAssertTrue(
                waitForText(app, "\(voter) voted yes", timeout: screenTimeout),
                "S1.6: the Proposal screen does not read \"\(voter) voted yes\" within \(Int(screenTimeout)) s")
        }

        recorder.step("S1.7", "B's yes makes the majority and closes the vote") {
            vote(app, proposalID, "Yes", change: false, step: "S1.7")
            XCTAssertTrue(
                waitForGone(text(target, containing: "Closes in"), timeout: screenTimeout),
                "S1.7: the card still reads \"Closes in\" \(Int(screenTimeout)) s after the majority")
            for label in ["Yes", "No", "Change"] {
                XCTAssertFalse(target.buttons[label].exists, "S1.7: the closed card still shows \"\(label)\"")
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

        recorder.step("S2.2", "vote yes on the cabal's card") {
            vote(app, proposalID, "Yes", change: false, step: "S2.2")
            expectText(target, "✓ You voted yes", step: "S2.2")
        }

        recorder.step("S2.3", "the cabal's card opens the Proposal screen") {
            tapHeaderToOpenProposal(target)
            waitForProposalScreen(app, step: "S2.3")
        }
    }

    static func seeAllHistory(
        _ app: XCUIApplication, cabalName: String, proposalID: String, closedProposalID: String,
        recorder: JourneyRecorder
    ) {
        recorder.step("S3.1", "the cabal lists the proposal with See all") {
            openCabal(app, cabalName: cabalName, proposalID: proposalID, step: "S3.1")
        }

        recorder.step("S3.2", "See all lists the open and the closed proposal") {
            app.buttons["See all"].tap()
            XCTAssertTrue(
                app.navigationBars["Proposals"].waitForExistence(timeout: screenTimeout),
                "S3.2: See all did not open the Proposals list within \(Int(screenTimeout)) s")
            let closed = card(app, closedProposalID)
            XCTAssertTrue(
                closed.waitForExistence(timeout: screenTimeout),
                "S3.2: the closed proposal-card-\(closedProposalID) did not show within \(Int(screenTimeout)) s")
            XCTAssertTrue(card(app, proposalID).exists, "S3.2: the open proposal-card-\(proposalID) is not in See all")
            expectText(closed, "Expired", step: "S3.2")
            XCTAssertFalse(closed.buttons["Yes"].exists, "S3.2: the closed card shows \"Yes\"")
            XCTAssertFalse(closed.buttons["No"].exists, "S3.2: the closed card shows \"No\"")
        }
    }
}
