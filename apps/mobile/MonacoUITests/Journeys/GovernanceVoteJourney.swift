import XCTest

private typealias S = GovernanceVoteJourneySteps

enum GovernanceVoteJourney {
    static let screenTimeout: TimeInterval = 15
    static let checkTimeout: TimeInterval = 10

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: S.id, version: S.version)
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
        let banner = app.element("monaco-toast-banner")
        let text = app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", message)).firstMatch
        let deadline = Date().addingTimeInterval(checkTimeout)
        var shown = false
        repeat {
            shown = (banner.exists && banner.label.contains(message)) || text.exists
            if !shown { RunLoop.current.run(until: Date().addingTimeInterval(0.2)) }
        } while !shown && Date() < deadline
        XCTAssertTrue(shown, "\(step): the toast does not read \"\(message)\" within \(Int(checkTimeout)) s")
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
        recorder.step(S.s1p1) {
            openHomeVote(app, proposalID, step: S.s1p1.id)
            XCTAssertTrue(
                app.element("home-vote-no-\(proposalID)").exists,
                "\(S.s1p1.id): home-vote-no-\(proposalID) does not show")
            expectLabel(app, "home-vote-yes-\(proposalID)", S.s1p1.copy[1], step: S.s1p1.id)
            expectLabel(app, "home-vote-no-\(proposalID)", S.s1p1.copy[1], step: S.s1p1.id)
            XCTAssertFalse(
                app.element("home-vote-yes-\(proposalID)").isSelected, "\(S.s1p1.id): Yes is highlighted before a pick")
        }

        recorder.step(S.s1p2) {
            openProposalFromHome(app, proposalID, step: S.s1p2.id)
            XCTAssertTrue(
                waitForText(app, S.s1p2.filled(4, ["QA.run": run]), timeout: checkTimeout),
                "\(S.s1p2.id): the thesis does not read QA vote \(run)")
            for cell in S.s1p2.copy.prefix(4) {
                XCTAssertTrue(
                    waitForText(app.element("proposal-stat-row"), cell, timeout: checkTimeout),
                    "\(S.s1p2.id): the stat row shows no \(cell)")
            }
            expectTally(app, yes: S.s1p2.copy[5], no: S.s1p2.copy[6], step: S.s1p2.id)
            XCTAssertTrue(
                JourneyDiagnostics.wait(for: app.buttons["proposal-remind"], in: app, timeout: checkTimeout),
                "\(S.s1p2.id): no \"Remind me at close\"")
        }

        recorder.step(S.s1p3) {
            app.element("proposal-vote-yes").tap()
            XCTAssertFalse(
                app.element("monaco-toast-banner").waitForExistence(timeout: 2),
                "\(S.s1p3.id): a short tap toasted a vote")
            expectTally(app, yes: S.s1p3.copy[0], no: S.s1p3.copy[1], step: S.s1p3.id)
        }

        recorder.step(S.s1p4) {
            app.pickAndHold("yes")
            expectLabel(app, "proposal-vote-yes", S.s1p4.copy[0], step: S.s1p4.id)
            expectLabel(app, "proposal-vote-status", S.s1p4.copy[1], step: S.s1p4.id)
            expectToast(app, S.s1p4.copy[2], undo: true, step: S.s1p4.id)
            XCTAssertFalse(app.buttons["proposal-remind"].exists, "\(S.s1p4.id): \"Remind me at close\" still shows")
            XCTAssertFalse(
                app.element("push-pre-prompt").exists, "\(S.s1p4.id): the permission sheet shows while the toast does")
            XCTAssertTrue(
                app.element("monaco-toast-banner").waitForNonExistence(timeout: 12),
                "\(S.s1p4.id): the toast did not close")
            XCTAssertTrue(
                waitForText(app, S.s1p4.copy[4], timeout: S.s1p4.seconds),
                "\(S.s1p4.id): no \"Hear how votes end?\" within \(Int(S.s1p4.seconds)) s of the toast closing")
            XCTAssertTrue(
                app.answerVotePermissionSheet(turnOn: false), "\(S.s1p4.id): Not now did not dismiss the sheet")
        }

        recorder.step(S.s1p5) {
            XCTAssertTrue(
                JourneyDiagnostics.wait(for: app.buttons["proposal-change-vote"], in: app, timeout: S.s1p5.seconds),
                "\(S.s1p5.id): no \"Change\" on the open ballot")
            app.changeVote(to: "no")
            expectTally(app, yes: S.s1p5.copy[1], no: S.s1p5.copy[2], step: S.s1p5.id)
            expectToast(app, S.s1p5.copy[3], undo: false, step: S.s1p5.id)
            XCTAssertTrue(
                JourneyDiagnostics.wait(for: app.buttons["proposal-change-vote"], in: app, timeout: S.s1p5.seconds),
                "\(S.s1p5.id): no \"Change\" after the change")
            app.waitForToastGone()
            app.changeVote(to: "yes")
            expectTally(app, yes: S.s1p5.copy[5], no: S.s1p5.copy[6], step: S.s1p5.id)
            expectToast(app, S.s1p5.copy[7], undo: false, step: S.s1p5.id)
            XCTAssertFalse(app.element("push-pre-prompt").exists, "\(S.s1p5.id): the permission sheet showed again")
        }
    }

    static func secondVoterPasses(_ app: XCUIApplication, proposalID: String, recorder: JourneyRecorder) {
        recorder.step(S.s1p6) {
            openHomeVote(app, proposalID, step: S.s1p6.id)
            openProposalFromHome(app, proposalID, step: S.s1p6.id)
            expectTally(app, yes: S.s1p6.copy[0], no: S.s1p6.copy[1], step: S.s1p6.id)
            XCTAssertTrue(
                waitForText(app, S.s1p6.copy[2], timeout: S.s1p6.seconds),
                "\(S.s1p6.id): the status line shows no 1 of 3 voted")
            XCTAssertFalse(
                app.element("proposal-votes-see-all").exists, "\(S.s1p6.id): a votes list shows on an open proposal")
        }

        recorder.step(S.s1p7) {
            app.pickAndHold("no")
            expectLabel(app, "proposal-vote-no", S.s1p7.copy[0], step: S.s1p7.id)
            expectToast(app, S.s1p7.copy[1], undo: true, step: S.s1p7.id)
            XCTAssertTrue(app.tapToastAction(timeout: S.s1p7.seconds), "\(S.s1p7.id): the toast has no Undo")
            expectTally(app, yes: S.s1p7.copy[3], no: S.s1p7.copy[4], step: S.s1p7.id)
            XCTAssertTrue(
                waitForLabel(app.element("monaco-toast-banner"), S.s1p7.copy[5], timeout: S.s1p7.seconds),
                "\(S.s1p7.id): the toast does not read \"Vote undone.\"")
            XCTAssertTrue(
                JourneyDiagnostics.wait(for: app.element("proposal-vote-yes"), in: app, timeout: S.s1p7.seconds),
                "\(S.s1p7.id): the ballot is not idle again")
            XCTAssertFalse(app.buttons["proposal-change-vote"].exists, "\(S.s1p7.id): \"Change\" shows after Undo")
            XCTAssertFalse(app.element("push-pre-prompt").exists, "\(S.s1p7.id): the permission sheet showed again")
        }

        recorder.step(S.s1p8) {
            app.waitForToastGone()
            app.pickAndHold("yes")
            expectToast(app, S.s1p8.copy[0], undo: false, step: S.s1p8.id)
            expectLabel(app, "proposal-vote-yes", S.s1p8.copy[1], step: S.s1p8.id)
            XCTAssertTrue(
                waitForText(app.element("proposal-vote-status"), S.s1p8.copy[2], timeout: S.s1p8.seconds),
                "\(S.s1p8.id): the status line does not start with Passed 2–0")
            XCTAssertFalse(
                app.buttons["proposal-change-vote"].exists, "\(S.s1p8.id): \"Change\" shows on a settled card")
            XCTAssertTrue(
                waitForText(app.element("proposal-stat-row"), S.s1p8.copy[3], timeout: S.s1p8.seconds),
                "\(S.s1p8.id): the stat row's last cell does not read Passed")
        }
    }

    static func firstVoterSeesItSettled(_ app: XCUIApplication, proposalID: String, recorder: JourneyRecorder) {
        recorder.step(S.s1p9) {
            app.terminate()
            app.launch()
            openHome(app, step: S.s1p9.id)
            XCTAssertTrue(
                waitForGone(app.element("home-vote-yes-\(proposalID)"), timeout: S.s1p9.seconds),
                "\(S.s1p9.id): home-vote-yes-\(proposalID) still offers a vote \(Int(S.s1p9.seconds)) s after settlement"
            )
            let open = app.element("home-vote-open-\(proposalID)")
            if open.waitForExistence(timeout: 3) {
                open.tap()
                waitForProposalScreen(app, step: S.s1p9.id)
                XCTAssertFalse(
                    app.buttons["proposal-change-vote"].exists, "\(S.s1p9.id): \"Change\" shows after settlement")
            }
        }
    }
}

extension GovernanceVoteJourney {
    static func voteFromCabalPage(
        _ app: XCUIApplication, cabalName: String, proposalID: String, recorder: JourneyRecorder
    ) {
        recorder.step(S.s2p1) {
            app.openCabal(named: cabalName)
            let row = app.element("cabal-open-proposal-\(proposalID)")
            XCTAssertTrue(
                JourneyDiagnostics.wait(for: row, in: app, timeout: S.s2p1.seconds),
                "\(S.s2p1.id): cabal-open-proposal-\(proposalID) did not show within \(Int(S.s2p1.seconds)) s")
        }

        recorder.step(S.s2p2) {
            let open = app.element("cabal-open-proposal-open-\(proposalID)")
            XCTAssertTrue(
                JourneyDiagnostics.wait(for: open, in: app, timeout: S.s2p2.seconds),
                "\(S.s2p2.id): cabal-open-proposal-open-\(proposalID) did not show within \(Int(S.s2p2.seconds)) s")
            open.tap()
            waitForProposalScreen(app, step: S.s2p2.id)
        }

        recorder.step(S.s2p3) {
            app.pickAndHold("yes")
            expectToast(app, S.s2p3.copy[0], undo: true, step: S.s2p3.id)
            XCTAssertTrue(
                JourneyDiagnostics.wait(for: app.buttons["proposal-change-vote"], in: app, timeout: S.s2p3.seconds),
                "\(S.s2p3.id): no \"Change\" on the card")
        }
    }

    static func allProposals(
        _ app: XCUIApplication, proposalID: String, closedProposalID: String, recorder: JourneyRecorder
    ) {
        recorder.step(S.s3p1) {
            openHomeVote(app, proposalID, step: S.s3p1.id)
            let all = app.buttons["All proposals"].firstMatch
            XCTAssertTrue(
                JourneyDiagnostics.wait(for: all, in: app, timeout: S.s3p1.seconds),
                "\(S.s3p1.id): no \"All proposals\"")
            all.tap()
            XCTAssertTrue(
                app.navigationBars[S.s3p1.copy[0]].waitForExistence(timeout: S.s3p1.seconds),
                "\(S.s3p1.id): All proposals did not open within \(Int(S.s3p1.seconds)) s")
        }

        recorder.step(S.s3p2) {
            let open = app.descendants(matching: .any).matching(
                NSPredicate(format: "identifier IN %@", ["proposal-card-\(proposalID)", "proposal-row-\(proposalID)"])
            ).firstMatch
            XCTAssertTrue(
                JourneyDiagnostics.wait(for: open, in: app, timeout: S.s3p2.seconds),
                "\(S.s3p2.id): the open proposal \(proposalID) is not under Open")
            app.element("proposals-segment-closed").tap()
            let closed = app.element("proposal-row-\(closedProposalID)")
            XCTAssertTrue(
                JourneyDiagnostics.wait(for: closed, in: app, timeout: S.s3p2.seconds),
                "\(S.s3p2.id): proposal-row-\(closedProposalID) did not show under Closed")
            XCTAssertTrue(
                waitForText(closed, S.s3p2.copy[0], timeout: checkTimeout)
                    || waitForText(closed, S.s3p2.copy[1], timeout: 1),
                "\(S.s3p2.id): the closed row shows neither Expired nor Failed")
        }
    }

    static func sidesAfterClose(
        _ app: XCUIApplication, closedProposalID: String, offProposalID: String, hostName: String, otherName: String,
        recorder: JourneyRecorder
    ) {
        recorder.step(S.s4p1) {
            openClosed(app, closedProposalID, step: S.s4p1.id)
            let sides = app.element("proposal-sides")
            XCTAssertTrue(
                JourneyDiagnostics.wait(for: sides, in: app, timeout: S.s4p1.seconds),
                "\(S.s4p1.id): the closed proposal shows no proposal-sides")
            XCTAssertTrue(
                waitForText(app, S.s4p1.copy[0], timeout: checkTimeout)
                    && waitForText(app, S.s4p1.copy[1], timeout: checkTimeout),
                "\(S.s4p1.id): the closed tally does not read ✓ 2 and 1 ✕")
            for name in [S.s4p1.copy[2], hostName, otherName] {
                XCTAssertTrue(
                    waitForText(sides, name, timeout: checkTimeout),
                    "\(S.s4p1.id): proposal-sides does not name \(name)")
            }
        }

        recorder.step(S.s4p2) {
            app.popToRoot()
            openClosed(app, offProposalID, step: S.s4p2.id)
            XCTAssertTrue(
                JourneyDiagnostics.wait(for: app.element("proposal-result"), in: app, timeout: S.s4p2.seconds),
                "\(S.s4p2.id): the closed proposal shows no result")
            XCTAssertFalse(app.element("proposal-sides").exists, "\(S.s4p2.id): proposal-sides shows with the rule off")
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
