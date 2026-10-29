import XCTest

enum GovernanceVoteJourney {
    static let id = "governance/vote"
    static let version = 5

    static let screenTimeout: TimeInterval = 15
    static let checkTimeout: TimeInterval = 10

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func text(_ scope: XCUIElement, containing value: String) -> XCUIElement {
        scope.descendants(matching: .any).matching(NSPredicate(format: "label CONTAINS %@", value)).firstMatch
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

    static func waitForLabel(_ element: XCUIElement, _ value: String, timeout: TimeInterval) -> Bool {
        let deadline = Date().addingTimeInterval(timeout)
        repeat {
            if element.exists && element.label.contains(value) { return true }
            RunLoop.current.run(until: Date().addingTimeInterval(0.2))
        } while Date() < deadline
        return false
    }

    static func waitForGone(_ element: XCUIElement, timeout: TimeInterval) -> Bool {
        element.waitForNonExistence(timeout: timeout)
    }

    static func expectLabel(_ app: XCUIApplication, _ identifier: String, _ value: String, step: String) {
        XCTAssertTrue(
            waitForLabel(app.element(identifier), value, timeout: checkTimeout),
            "\(step): \(identifier) does not read \"\(value)\" within \(Int(checkTimeout)) s")
    }

    static func expectToast(_ app: XCUIApplication, _ message: String, undo: Bool, step: String) {
        XCTAssertTrue(
            waitForLabel(app.element("monaco-toast-banner"), message, timeout: checkTimeout),
            "\(step): the toast does not read \"\(message)\" within \(Int(checkTimeout)) s")
        XCTAssertEqual(
            app.element("monaco-toast-action").exists, undo,
            "\(step): the toast \(undo ? "has no" : "offers") Undo")
    }

    static func expectTally(_ app: XCUIApplication, yes: String, no: String, step: String) {
        expectLabel(app, "proposal-vote-yes", yes, step: step)
        expectLabel(app, "proposal-vote-no", no, step: step)
    }

    static func openHome(_ app: XCUIApplication, step: String) {
        XCTAssertTrue(
            app.tab("Home").waitForExistence(timeout: screenTimeout),
            "\(step): the tab bar did not show within \(Int(screenTimeout)) s")
        app.tab("Home").tap()
    }

    static func openHomeVote(_ app: XCUIApplication, _ proposalID: String, step: String) {
        openHome(app, step: step)
        XCTAssertTrue(
            waitForText(app, "Your vote", timeout: screenTimeout),
            "\(step): Home shows no \"Your vote\" within \(Int(screenTimeout)) s")
        let yes = app.element("home-vote-yes-\(proposalID)")
        app.scrollIntoReach(yes)
        XCTAssertTrue(
            JourneyDiagnostics.wait(for: yes, in: app, timeout: screenTimeout),
            "\(step): home-vote-yes-\(proposalID) is not under \"Your vote\" within \(Int(screenTimeout)) s")
    }

    static func openProposalFromHome(_ app: XCUIApplication, _ proposalID: String, step: String) {
        let open = app.element("home-vote-open-\(proposalID)")
        app.scrollIntoReach(open)
        XCTAssertTrue(
            JourneyDiagnostics.wait(for: open, in: app, timeout: screenTimeout),
            "\(step): home-vote-open-\(proposalID) did not show within \(Int(screenTimeout)) s")
        open.tap()
        waitForProposalScreen(app, step: step)
    }

    static func waitForProposalScreen(_ app: XCUIApplication, step: String) {
        XCTAssertTrue(
            JourneyDiagnostics.wait(for: app.element("proposal-stat-row"), in: app, timeout: screenTimeout),
            "\(step): the Proposal screen shows no proposal-stat-row within \(Int(screenTimeout)) s")
    }

    static func voteFromHome(_ app: XCUIApplication, proposalID: String, run: String, recorder: JourneyRecorder) {
        recorder.step("S1.1", "Home lists the proposal under Your vote with neither side highlighted") {
            openHomeVote(app, proposalID, step: "S1.1")
            XCTAssertTrue(
                app.element("home-vote-no-\(proposalID)").exists, "S1.1: home-vote-no-\(proposalID) does not show")
            expectLabel(app, "home-vote-yes-\(proposalID)", "0", step: "S1.1")
            expectLabel(app, "home-vote-no-\(proposalID)", "0", step: "S1.1")
            XCTAssertFalse(
                app.element("home-vote-yes-\(proposalID)").isSelected, "S1.1: Yes is highlighted before a pick")
        }

        recorder.step("S1.2", "the card's row opens the Proposal screen") {
            openProposalFromHome(app, proposalID, step: "S1.2")
            XCTAssertTrue(
                waitForText(app, "QA vote \(run)", timeout: checkTimeout),
                "S1.2: the thesis does not read QA vote \(run)")
            for cell in ["Price", "Shares", "Of treasury", "Closes"] {
                XCTAssertTrue(
                    waitForText(app.element("proposal-stat-row"), cell, timeout: checkTimeout),
                    "S1.2: the stat row shows no \(cell)")
            }
            expectTally(app, yes: "Yes 0", no: "No 0", step: "S1.2")
            XCTAssertTrue(
                JourneyDiagnostics.wait(for: app.buttons["proposal-remind"], in: app, timeout: checkTimeout),
                "S1.2: no \"Remind me at close\"")
        }

        recorder.step("S1.3", "a short tap picks Yes and casts nothing") {
            app.element("proposal-vote-yes").tap()
            XCTAssertFalse(
                app.element("monaco-toast-banner").waitForExistence(timeout: 2),
                "S1.3: a short tap toasted a vote")
            expectTally(app, yes: "Yes 0", no: "No 0", step: "S1.3")
        }

        recorder.step("S1.4", "pick and hold casts the first vote, then the permission sheet shows") {
            app.pickAndHold("yes")
            expectLabel(app, "proposal-vote-yes", "Voted Yes 1", step: "S1.4")
            expectLabel(app, "proposal-vote-status", "1 of 3 voted", step: "S1.4")
            expectToast(app, "Voted Yes. 1st of 3.", undo: true, step: "S1.4")
            XCTAssertFalse(app.buttons["proposal-remind"].exists, "S1.4: \"Remind me at close\" still shows")
            XCTAssertFalse(
                app.element("push-pre-prompt").exists, "S1.4: the permission sheet shows while the toast does")
            XCTAssertTrue(
                app.element("monaco-toast-banner").waitForNonExistence(timeout: 12),
                "S1.4: the toast did not close")
            XCTAssertTrue(
                waitForText(app, "Hear how votes end?", timeout: checkTimeout),
                "S1.4: no \"Hear how votes end?\" within \(Int(checkTimeout)) s of the toast closing")
            XCTAssertTrue(app.answerVotePermissionSheet(turnOn: false), "S1.4: Not now did not dismiss the sheet")
        }

        recorder.step("S1.5", "Change moves the open ballot, with no Undo") {
            XCTAssertTrue(
                JourneyDiagnostics.wait(for: app.buttons["proposal-change-vote"], in: app, timeout: checkTimeout),
                "S1.5: no \"Change\" on the open ballot")
            app.changeVote(to: "no")
            expectTally(app, yes: "Yes 0", no: "No 1", step: "S1.5")
            expectToast(app, "Changed to No.", undo: false, step: "S1.5")
            XCTAssertTrue(
                JourneyDiagnostics.wait(for: app.buttons["proposal-change-vote"], in: app, timeout: checkTimeout),
                "S1.5: no \"Change\" after the change")
            app.waitForToastGone()
            app.changeVote(to: "yes")
            expectTally(app, yes: "Yes 1", no: "No 0", step: "S1.5")
            expectToast(app, "Changed to Yes.", undo: false, step: "S1.5")
            XCTAssertFalse(app.element("push-pre-prompt").exists, "S1.5: the permission sheet showed again")
        }
    }

    static func secondVoterPasses(_ app: XCUIApplication, proposalID: String, recorder: JourneyRecorder) {
        recorder.step("S1.6", "B opens the proposal and sees A's ballot only as a count") {
            openHomeVote(app, proposalID, step: "S1.6")
            openProposalFromHome(app, proposalID, step: "S1.6")
            expectTally(app, yes: "Yes 1", no: "No 0", step: "S1.6")
            XCTAssertTrue(
                waitForText(app, "1 of 3 voted", timeout: screenTimeout), "S1.6: the status line shows no 1 of 3 voted")
            XCTAssertFalse(app.element("proposal-votes-see-all").exists, "S1.6: a votes list shows on an open proposal")
        }

        recorder.step("S1.7", "B votes No, then Undo takes the ballot back") {
            app.pickAndHold("no")
            expectLabel(app, "proposal-vote-no", "Voted No 1", step: "S1.7")
            XCTAssertEqual(
                app.toastText(timeout: checkTimeout)?.contains("Voted No. 2nd of 3."), true,
                "S1.7: wrong toast")
            XCTAssertTrue(app.tapToastAction(timeout: checkTimeout), "S1.7: the toast has no Undo")
            expectTally(app, yes: "Yes 1", no: "No 0", step: "S1.7")
            XCTAssertTrue(
                waitForLabel(app.element("monaco-toast-banner"), "Vote undone.", timeout: checkTimeout),
                "S1.7: the toast does not read \"Vote undone.\"")
            XCTAssertTrue(
                JourneyDiagnostics.wait(for: app.element("proposal-vote-yes"), in: app, timeout: checkTimeout),
                "S1.7: the ballot is not idle again")
            XCTAssertFalse(app.buttons["proposal-change-vote"].exists, "S1.7: \"Change\" shows after Undo")
            XCTAssertFalse(app.element("push-pre-prompt").exists, "S1.7: the permission sheet showed again")
        }

        recorder.step("S1.8", "B's yes settles the proposal with no Undo and no Change") {
            app.waitForToastGone()
            app.pickAndHold("yes")
            expectToast(app, "Voted Yes. Passed 2–0.", undo: false, step: "S1.8")
            expectLabel(app, "proposal-vote-yes", "Voted Yes 2", step: "S1.8")
            XCTAssertTrue(
                waitForText(app.element("proposal-vote-status"), "Passed 2–0", timeout: checkTimeout),
                "S1.8: the status line does not start with Passed 2–0")
            XCTAssertFalse(app.buttons["proposal-change-vote"].exists, "S1.8: \"Change\" shows on a settled card")
            XCTAssertTrue(
                waitForText(app.element("proposal-stat-row"), "Passed", timeout: checkTimeout),
                "S1.8: the stat row's last cell does not read Passed")
        }
    }

    static func firstVoterSeesItSettled(_ app: XCUIApplication, proposalID: String, recorder: JourneyRecorder) {
        recorder.step("S1.9", "the settled proposal no longer asks A for a vote") {
            app.terminate()
            app.launch()
            openHome(app, step: "S1.9")
            XCTAssertTrue(
                waitForGone(app.element("home-vote-yes-\(proposalID)"), timeout: screenTimeout),
                "S1.9: home-vote-yes-\(proposalID) still offers a vote \(Int(screenTimeout)) s after settlement")
            let open = app.element("home-vote-open-\(proposalID)")
            if open.waitForExistence(timeout: 3) {
                open.tap()
                waitForProposalScreen(app, step: "S1.9")
                XCTAssertFalse(app.buttons["proposal-change-vote"].exists, "S1.9: \"Change\" shows after settlement")
            }
        }
    }
}

extension GovernanceVoteJourney {
    static func voteFromCabalPage(
        _ app: XCUIApplication, cabalName: String, proposalID: String, recorder: JourneyRecorder
    ) {
        recorder.step("S2.1", "the cabal page shows the open proposal above its tabs") {
            app.openCabal(named: cabalName)
            let row = app.element("cabal-open-proposal-\(proposalID)")
            XCTAssertTrue(
                JourneyDiagnostics.wait(for: row, in: app, timeout: screenTimeout),
                "S2.1: cabal-open-proposal-\(proposalID) did not show within \(Int(screenTimeout)) s")
        }

        recorder.step("S2.2", "the row opens the Proposal screen") {
            app.element("cabal-open-proposal-\(proposalID)").tap()
            waitForProposalScreen(app, step: "S2.2")
        }

        recorder.step("S2.3", "pick and hold votes Yes with Undo and Change") {
            app.pickAndHold("yes")
            expectToast(app, "Voted Yes.", undo: true, step: "S2.3")
            XCTAssertTrue(
                JourneyDiagnostics.wait(for: app.buttons["proposal-change-vote"], in: app, timeout: checkTimeout),
                "S2.3: no \"Change\" on the card")
        }
    }

    static func allProposals(
        _ app: XCUIApplication, proposalID: String, closedProposalID: String, recorder: JourneyRecorder
    ) {
        recorder.step("S3.1", "Home's Your vote header opens All proposals") {
            openHomeVote(app, proposalID, step: "S3.1")
            let all = app.buttons["All proposals"].firstMatch
            XCTAssertTrue(
                JourneyDiagnostics.wait(for: all, in: app, timeout: screenTimeout), "S3.1: no \"All proposals\"")
            all.tap()
            XCTAssertTrue(
                app.navigationBars["All proposals"].waitForExistence(timeout: screenTimeout),
                "S3.1: All proposals did not open within \(Int(screenTimeout)) s")
        }

        recorder.step("S3.2", "All proposals lists the open proposal and the closed one") {
            let open = app.descendants(matching: .any).matching(
                NSPredicate(format: "identifier IN %@", ["proposal-card-\(proposalID)", "proposal-row-\(proposalID)"])
            ).firstMatch
            XCTAssertTrue(
                JourneyDiagnostics.wait(for: open, in: app, timeout: screenTimeout),
                "S3.2: the open proposal \(proposalID) is not under Open")
            app.element("proposals-segment-closed").tap()
            let closed = app.element("proposal-row-\(closedProposalID)")
            XCTAssertTrue(
                JourneyDiagnostics.wait(for: closed, in: app, timeout: screenTimeout),
                "S3.2: proposal-row-\(closedProposalID) did not show under Closed")
            XCTAssertTrue(
                waitForText(closed, "Expired", timeout: checkTimeout) || waitForText(closed, "Failed", timeout: 1),
                "S3.2: the closed row shows neither Expired nor Failed")
        }
    }

    static func sidesAfterClose(
        _ app: XCUIApplication, closedProposalID: String, offProposalID: String, hostName: String, otherName: String,
        recorder: JourneyRecorder
    ) {
        recorder.step("S4.1", "a closed proposal in a cabal with the rule on shows who voted what") {
            openClosed(app, closedProposalID, step: "S4.1")
            let sides = app.element("proposal-sides")
            XCTAssertTrue(
                JourneyDiagnostics.wait(for: sides, in: app, timeout: screenTimeout),
                "S4.1: the closed proposal shows no proposal-sides")
            XCTAssertTrue(
                waitForText(app, "✓ 2", timeout: checkTimeout) && waitForText(app, "1 ✕", timeout: checkTimeout),
                "S4.1: the closed tally does not read ✓ 2 and 1 ✕")
            for name in ["you", hostName, otherName] {
                XCTAssertTrue(
                    waitForText(sides, name, timeout: checkTimeout), "S4.1: proposal-sides does not name \(name)")
            }
        }

        recorder.step("S4.2", "a closed proposal in a cabal with the rule off shows counts and no sides") {
            app.popToRoot()
            openClosed(app, offProposalID, step: "S4.2")
            XCTAssertTrue(
                JourneyDiagnostics.wait(for: app.element("proposal-result"), in: app, timeout: screenTimeout),
                "S4.2: the closed proposal shows no result")
            XCTAssertFalse(app.element("proposal-sides").exists, "S4.2: proposal-sides shows with the rule off")
        }
    }

    static func openClosed(_ app: XCUIApplication, _ proposalID: String, step: String) {
        openHome(app, step: step)
        let all = app.buttons["All proposals"].firstMatch
        XCTAssertTrue(
            JourneyDiagnostics.wait(for: all, in: app, timeout: screenTimeout), "\(step): no \"All proposals\"")
        all.tap()
        let segment = app.element("proposals-segment-closed")
        XCTAssertTrue(
            JourneyDiagnostics.wait(for: segment, in: app, timeout: screenTimeout),
            "\(step): All proposals shows no Closed segment")
        segment.tap()
        let row = app.element("proposal-row-\(proposalID)")
        XCTAssertTrue(
            JourneyDiagnostics.wait(for: row, in: app, timeout: screenTimeout),
            "\(step): proposal-row-\(proposalID) did not show under Closed")
        row.tap()
    }

    static func card(_ app: XCUIApplication, _ proposalID: String) -> XCUIElement {
        app.element("proposal-card-\(proposalID)")
    }

    static func openProposal(_ card: XCUIElement) {
        card.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 1)).withOffset(CGVector(dx: 0, dy: -22)).tap()
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
}
