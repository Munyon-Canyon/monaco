import XCTest

enum GovernanceVoteJourney {
    static let id = "governance/vote"
    static let version = 5

    static let screenTimeout: TimeInterval = 15
    static let checkTimeout: TimeInterval = 10

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func labelMatches(
        _ app: XCUIApplication, _ identifier: String, _ value: String, timeout: TimeInterval = checkTimeout
    ) -> Bool {
        let element = app.element(identifier)
        return JourneyWait.until(timeout: timeout) { element.exists && element.label.contains(value) }
    }

    static func expectLabel(
        _ app: XCUIApplication, _ identifier: String, _ value: String, step: String,
        timeout: TimeInterval = checkTimeout
    ) {
        XCTAssertTrue(
            labelMatches(app, identifier, value, timeout: timeout),
            "\(step): \(identifier) does not read \"\(value)\" within \(Int(timeout)) s, it reads \"\(app.element(identifier).label)\""
        )
    }

    static func text(_ scope: XCUIElement, containing value: String) -> XCUIElement {
        scope.descendants(matching: .any).matching(NSPredicate(format: "label CONTAINS %@", value)).firstMatch
    }

    static func waitForText(_ scope: XCUIElement, _ value: String, timeout: TimeInterval) -> Bool {
        JourneyWait.until(timeout: timeout) { text(scope, containing: value).exists }
    }

    static func waitForGone(_ element: XCUIElement, timeout: TimeInterval) -> Bool {
        JourneyWait.until(timeout: timeout) { !element.exists }
    }

    static func toastLabel(_ app: XCUIApplication) -> String {
        let banner = app.element("monaco-toast-banner")
        guard banner.exists else { return "" }
        return banner.label.isEmpty ? banner.staticTexts.firstMatch.label : banner.label
    }

    static func expectToast(_ app: XCUIApplication, _ message: String, step: String) {
        XCTAssertTrue(
            JourneyWait.until(timeout: checkTimeout) { toastLabel(app).contains(message) },
            "\(step): the toast \"\(message)\" did not show within \(Int(checkTimeout)) s, it reads \"\(toastLabel(app))\""
        )
    }

    static func expectNoUndo(_ app: XCUIApplication, step: String) {
        XCTAssertFalse(app.element("monaco-toast-action").exists, "\(step): the toast offers Undo")
    }

    static func expectVoted(_ app: XCUIApplication, _ choice: String, count: Int, step: String) {
        let card = "You voted \(choice)"
        let committed = "Voted \(choice), \(count) vote\(count == 1 ? "" : "s")"
        let side = "proposal-vote-\(choice.lowercased())"
        XCTAssertTrue(
            JourneyWait.until(timeout: checkTimeout) {
                let voted = app.element("proposal-voted")
                return (voted.exists && voted.label.contains(card))
                    || (app.element(side).exists && app.element(side).label.contains(committed))
            },
            "\(step): the card shows neither \"\(card)\" nor \"\(committed)\" within \(Int(checkTimeout)) s")
    }

    static func expectTally(_ app: XCUIApplication, yes: Int, no: Int, step: String) {
        expectLabel(app, "proposal-vote-yes", "Yes, \(yes) vote", step: step)
        expectLabel(app, "proposal-vote-no", "No, \(no) vote", step: step)
    }

    static func openHomeCard(_ app: XCUIApplication, _ proposalID: String, step: String) {
        XCTAssertTrue(
            app.tab("Home").waitForExistence(timeout: screenTimeout),
            "\(step): the tab bar did not show within \(Int(screenTimeout)) s")
        app.tab("Home").tap()
        let card = app.element("home-vote-card-\(proposalID)")
        XCTAssertTrue(
            JourneyDiagnostics.wait(for: card, in: app, timeout: screenTimeout),
            "\(step): home-vote-card-\(proposalID) is not under \"Your vote\" within \(Int(screenTimeout)) s")
        XCTAssertTrue(waitForText(app, "Your vote", timeout: screenTimeout), "\(step): Home shows no \"Your vote\"")
    }

    static func openProposalFromHome(_ app: XCUIApplication, _ proposalID: String, step: String) {
        openHomeCard(app, proposalID, step: step)
        let header = app.element("home-vote-open-\(proposalID)")
        app.scrollIntoReach(header)
        header.tap()
        XCTAssertTrue(
            app.element("proposal-vote-yes").waitForExistence(timeout: screenTimeout),
            "\(step): the Proposal screen shows no proposal-vote-yes within \(Int(screenTimeout)) s")
    }

    static func voteFromHome(_ app: XCUIApplication, proposalID: String, run: String, recorder: JourneyRecorder) {
        recorder.step("S1.1", "Home lists the proposal under Your vote with live neutral counts") {
            openHomeCard(app, proposalID, step: "S1.1")
            expectLabel(app, "home-vote-yes-\(proposalID)", "Yes, 0 votes", step: "S1.1")
            expectLabel(app, "home-vote-no-\(proposalID)", "No, 0 votes", step: "S1.1")
            XCTAssertFalse(
                app.element("home-vote-yes-\(proposalID)").isSelected, "S1.1: Yes is highlighted before a pick")
            XCTAssertFalse(
                app.element("home-vote-no-\(proposalID)").isSelected, "S1.1: No is highlighted before a pick")
        }

        recorder.step("S1.2", "the card opens the Proposal screen") {
            let header = app.element("home-vote-open-\(proposalID)")
            app.scrollIntoReach(header)
            header.tap()
            let stats = app.element("proposal-stat-row")
            XCTAssertTrue(
                stats.waitForExistence(timeout: screenTimeout),
                "S1.2: proposal-stat-row did not show within \(Int(screenTimeout)) s")
            for label in ["Price", "Shares", "Of treasury", "Closes"] {
                XCTAssertTrue(
                    waitForText(stats, label, timeout: screenTimeout), "S1.2: the stat row shows no \"\(label)\"")
            }
            XCTAssertTrue(
                app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH 'Buy '")).firstMatch.exists,
                "S1.2: the Proposal screen shows no title \"Buy …\"")
            expectLabel(app, "proposal-reason", "QA vote \(run)", step: "S1.2", timeout: screenTimeout)
            expectTally(app, yes: 0, no: 0, step: "S1.2")
            XCTAssertTrue(
                app.element("proposal-remind").waitForExistence(timeout: checkTimeout),
                "S1.2: the Proposal screen shows no \"Remind me at close\"")
        }

        recorder.step("S1.3", "a tap picks and casts nothing") {
            app.element("proposal-vote-yes").tap()
            let cast = JourneyWait.until(timeout: 2) {
                app.element("monaco-toast-banner").exists || app.element("proposal-voted").exists
            }
            XCTAssertFalse(cast, "S1.3: a tap cast a vote")
            expectTally(app, yes: 0, no: 0, step: "S1.3")
        }
    }

    static func castAndChange(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S1.4", "pick and hold casts the first vote") {
            app.holdToCommit(app.element("proposal-vote-yes"))
            expectToast(app, "Voted Yes. 1st of 3.", step: "S1.4")
            XCTAssertTrue(app.element("monaco-toast-action").exists, "S1.4: the first ballot's toast has no Undo")
            XCTAssertFalse(app.element("push-pre-prompt").exists, "S1.4: the permission sheet shows over the toast")
            expectVoted(app, "Yes", count: 1, step: "S1.4")
            expectLabel(app, "proposal-vote-status", "1 of 3 voted", step: "S1.4")
            XCTAssertTrue(
                waitForGone(app.element("proposal-remind"), timeout: checkTimeout),
                "S1.4: \"Remind me at close\" still shows after the vote")
        }

        recorder.step("S1.5", "the permission sheet comes after the toast") {
            let sheet = app.element("push-pre-prompt")
            XCTAssertTrue(
                sheet.waitForExistence(timeout: 20),
                "S1.5: push-pre-prompt did not show within 10 s of the toast closing")
            XCTAssertFalse(app.element("monaco-toast-banner").exists, "S1.5: the sheet shows while the toast is up")
            XCTAssertTrue(
                app.staticTexts["Hear how votes end?"].firstMatch.exists,
                "S1.5: the sheet is not \"Hear how votes end?\"")
            XCTAssertTrue(app.answerVotePermissionSheet(turnOn: false), "S1.5: \"Not now\" did not close the sheet")
        }

        recorder.step("S1.6", "Change flips the open ballot with no Undo") {
            let voted = app.element("proposal-voted")
            XCTAssertTrue(
                voted.waitForExistence(timeout: checkTimeout) && voted.label.contains("You voted Yes"),
                "S1.6: the card does not read \"You voted Yes\"")
            app.changeVote(to: "no")
            expectToast(app, "Changed to No.", step: "S1.6")
            expectNoUndo(app, step: "S1.6")
            expectVoted(app, "No", count: 1, step: "S1.6")
            app.changeVote(to: "yes")
            expectToast(app, "Changed to Yes.", step: "S1.6")
            expectNoUndo(app, step: "S1.6")
            expectVoted(app, "Yes", count: 1, step: "S1.6")
            XCTAssertFalse(app.element("push-pre-prompt").exists, "S1.6: the permission sheet showed again")
        }
    }

    static func secondVoterPasses(_ app: XCUIApplication, proposalID: String, recorder: JourneyRecorder) {
        recorder.step("S1.7", "B sees the live counts and nobody's side") {
            openProposalFromHome(app, proposalID, step: "S1.7")
            expectTally(app, yes: 1, no: 0, step: "S1.7")
            expectLabel(app, "proposal-vote-status", "1 of 3 voted", step: "S1.7", timeout: screenTimeout)
            XCTAssertFalse(app.element("proposal-votes-see-all").exists, "S1.7: the Proposal screen lists who voted")
        }

        recorder.step("S1.8", "B takes a first ballot back with Undo") {
            app.pickAndHold("no")
            expectToast(app, "Voted No. 2nd of 3.", step: "S1.8")
            XCTAssertTrue(app.tapToastAction(timeout: checkTimeout), "S1.8: the toast has no Undo")
            expectToast(app, "Vote undone.", step: "S1.8")
            expectTally(app, yes: 1, no: 0, step: "S1.8")
            XCTAssertFalse(app.element("proposal-change-vote").exists, "S1.8: Change shows after Undo")
            XCTAssertFalse(app.element("proposal-voted").exists, "S1.8: the card still reads \"You voted\"")
            XCTAssertFalse(app.element("push-pre-prompt").exists, "S1.8: the permission sheet showed again")
        }

        recorder.step("S1.9", "B's yes settles the vote at quorum") {
            app.pickAndHold("yes")
            expectToast(app, "Voted Yes. Passed 2–0.", step: "S1.9")
            expectNoUndo(app, step: "S1.9")
            expectLabel(app, "proposal-vote-yes", "Voted Yes, 2 votes", step: "S1.9")
            XCTAssertTrue(
                JourneyWait.until(timeout: checkTimeout) {
                    app.descendants(matching: .any).matching(NSPredicate(format: "label BEGINSWITH 'Passed 2–0'"))
                        .firstMatch.exists
                },
                "S1.9: no status line starts \"Passed 2–0\"")
            XCTAssertFalse(app.element("proposal-change-vote").exists, "S1.9: Change shows on the settled card")
            XCTAssertTrue(
                waitForText(app.element("proposal-stat-row"), "Passed", timeout: checkTimeout),
                "S1.9: the stat row's last cell does not read \"Passed\"")
        }
    }

    static func firstVoterSeesItLeave(_ app: XCUIApplication, proposalID: String, recorder: JourneyRecorder) {
        recorder.step("S1.10", "the settled proposal leaves A's Your vote") {
            app.terminate()
            app.launch()
            XCTAssertTrue(
                app.tab("Home").waitForExistence(timeout: screenTimeout),
                "S1.10: the tab bar did not show within \(Int(screenTimeout)) s")
            app.tab("Home").tap()
            XCTAssertTrue(
                waitForGone(app.element("home-vote-card-\(proposalID)"), timeout: screenTimeout),
                "S1.10: home-vote-card-\(proposalID) is still on Home \(Int(screenTimeout)) s after the vote settled")
        }

        recorder.step("S1.11", "the settled proposal has no Change") {
            openAllProposals(app, step: "S1.11")
            openClosed(app, proposalID, step: "S1.11")
            XCTAssertTrue(
                app.element("proposal-result").waitForExistence(timeout: screenTimeout),
                "S1.11: proposal-result did not show within \(Int(screenTimeout)) s")
            XCTAssertTrue(
                waitForText(app, "Passed 2–0", timeout: checkTimeout),
                "S1.11: the closed proposal does not read \"Passed 2–0\"")
            XCTAssertFalse(app.element("proposal-change-vote").exists, "S1.11: Change shows on a settled proposal")
        }
    }
}

extension GovernanceVoteJourney {
    static func openAllProposals(_ app: XCUIApplication, step: String) {
        app.tab("Home").tap()
        let all = app.buttons["All proposals"].firstMatch
        XCTAssertTrue(
            all.waitForExistence(timeout: screenTimeout),
            "\(step): Home shows no \"All proposals\" within \(Int(screenTimeout)) s")
        app.scrollIntoReach(all)
        all.tap()
        XCTAssertTrue(
            app.element("proposals-segment-open").waitForExistence(timeout: screenTimeout),
            "\(step): the \"All proposals\" screen did not open within \(Int(screenTimeout)) s")
    }

    static func openClosed(_ app: XCUIApplication, _ proposalID: String, step: String) {
        app.element("proposals-segment-closed").tap()
        let row = app.element("proposal-row-\(proposalID)")
        XCTAssertTrue(
            row.waitForExistence(timeout: screenTimeout),
            "\(step): proposal-row-\(proposalID) did not show under Closed within \(Int(screenTimeout)) s")
        app.scrollIntoReach(row)
        row.tap()
    }

    static func voteFromCabalPage(
        _ app: XCUIApplication, cabalName: String, proposalID: String, run: String, recorder: JourneyRecorder
    ) {
        recorder.step("S2.1", "the cabal page lists the open proposal above its tabs") {
            app.openCabal(named: cabalName)
            XCTAssertTrue(
                app.element("cabal-open-proposal-\(proposalID)").waitForExistence(timeout: screenTimeout),
                "S2.1: cabal-open-proposal-\(proposalID) is not on the cabal page within \(Int(screenTimeout)) s")
        }

        recorder.step("S2.2", "the row opens the Proposal screen") {
            let row = app.element("cabal-open-proposal-\(proposalID)")
            app.scrollIntoReach(row)
            row.tap()
            XCTAssertTrue(
                app.element("proposal-vote-yes").waitForExistence(timeout: screenTimeout),
                "S2.2: the Proposal screen shows no proposal-vote-yes within \(Int(screenTimeout)) s")
            expectLabel(app, "proposal-reason", "QA vote \(run)", step: "S2.2", timeout: screenTimeout)
        }

        recorder.step("S2.3", "pick and hold votes from the cabal's proposal") {
            let yes = app.element("proposal-vote-yes")
            yes.tap()
            app.holdToCommit(yes)
            expectToast(app, "Voted Yes. 1st of 3.", step: "S2.3")
            XCTAssertTrue(app.element("monaco-toast-action").exists, "S2.3: the toast has no Undo")
            let voted = app.element("proposal-voted")
            XCTAssertTrue(
                JourneyWait.until(timeout: checkTimeout) { voted.exists && voted.label.contains("You voted Yes") },
                "S2.3: the card does not read \"You voted Yes\"")
            XCTAssertTrue(
                app.element("proposal-change-vote").waitForExistence(timeout: checkTimeout),
                "S2.3: the card shows no \"Change\"")
        }
    }

    static func allProposals(
        _ app: XCUIApplication, proposalID: String, closedProposalID: String, recorder: JourneyRecorder
    ) {
        recorder.step("S3.1", "All proposals lists the open proposal") {
            openAllProposals(app, step: "S3.1")
            XCTAssertTrue(
                app.element("proposal-card-\(proposalID)").waitForExistence(timeout: screenTimeout),
                "S3.1: proposal-card-\(proposalID) is not under Open within \(Int(screenTimeout)) s")
        }

        recorder.step("S3.2", "Closed lists the expired proposal with its status") {
            app.element("proposals-segment-closed").tap()
            let row = app.element("proposal-row-\(closedProposalID)")
            XCTAssertTrue(
                row.waitForExistence(timeout: screenTimeout),
                "S3.2: proposal-row-\(closedProposalID) did not show under Closed within \(Int(screenTimeout)) s")
            XCTAssertTrue(
                waitForText(row, "Expired", timeout: checkTimeout) || row.label.contains("Expired")
                    || row.label.contains("Failed"),
                "S3.2: the closed row reads \"\(row.label)\", not Expired or Failed")
        }
    }

    static func sidesAfterClose(
        _ app: XCUIApplication, sidesClosedID: String, plainClosedID: String, hostName: String, otherName: String,
        recorder: JourneyRecorder
    ) {
        recorder.step("S4.1", "the closed proposal shows the counts at both ends") {
            openAllProposals(app, step: "S4.1")
            openClosed(app, sidesClosedID, step: "S4.1")
            let tally = app.element("proposal-result-tally")
            XCTAssertTrue(
                tally.waitForExistence(timeout: screenTimeout),
                "S4.1: proposal-result-tally did not show within \(Int(screenTimeout)) s")
            XCTAssertTrue(
                tally.label.hasPrefix("✓ 2") && tally.label.hasSuffix("1 ✕"),
                "S4.1: the tally reads \"\(tally.label)\", not \"✓ 2 … 1 ✕\"")
        }

        recorder.step("S4.2", "the sides row names who voted which way") {
            let sides = app.element("proposal-sides")
            XCTAssertTrue(
                sides.waitForExistence(timeout: screenTimeout),
                "S4.2: proposal-sides did not show within \(Int(screenTimeout)) s")
            let yes = sides.descendants(matching: .any).matching(NSPredicate(format: "label BEGINSWITH 'Yes:'"))
                .firstMatch
            let no = sides.descendants(matching: .any).matching(NSPredicate(format: "label BEGINSWITH 'No:'"))
                .firstMatch
            XCTAssertTrue(yes.exists, "S4.2: the sides row has no \"Yes:\" line")
            XCTAssertTrue(
                yes.label.contains("you") && yes.label.contains(hostName),
                "S4.2: the Yes line reads \"\(yes.label)\", not you and \(hostName)")
            XCTAssertTrue(
                no.exists && no.label.contains(otherName), "S4.2: the No line reads \"\(no.label)\", not \(otherName)")
        }

        recorder.step("S4.3", "the cabal with the rule off shows no sides") {
            app.tapBack()
            openClosed(app, plainClosedID, step: "S4.3")
            XCTAssertTrue(
                app.element("proposal-result").waitForExistence(timeout: screenTimeout),
                "S4.3: proposal-result did not show within \(Int(screenTimeout)) s")
            XCTAssertFalse(app.element("proposal-sides").exists, "S4.3: the sides row shows with the rule off")
        }
    }
}

extension GovernanceVoteJourney {
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
}
