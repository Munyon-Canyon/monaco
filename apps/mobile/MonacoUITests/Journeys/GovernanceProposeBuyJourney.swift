import XCTest

enum GovernanceProposeBuyJourney {
    static let id = "governance/propose-buy"
    static let version = 9

    static let screenTimeout: TimeInterval = 15
    static let checkTimeout: TimeInterval = 10

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func reason(run: String) -> String { "Journey buy \(run)" }

    static func labelled(_ app: XCUIApplication, containing text: String) -> XCUIElement {
        app.descendants(matching: .any).matching(NSPredicate(format: "label CONTAINS %@", text)).firstMatch
    }

    static func expectLabel(_ app: XCUIApplication, _ text: String, timeout: TimeInterval = checkTimeout, step: String)
    {
        XCTAssertTrue(
            labelled(app, containing: text).waitForExistence(timeout: timeout),
            "\(step): no \"\(text)\" within \(Int(timeout)) s"
        )
    }

    static func expectTitle(sheet app: XCUIApplication, step: String) {
        XCTAssertTrue(
            app.element("propose-review-sheet").waitForExistence(timeout: checkTimeout),
            "\(step): no Review sheet within \(Int(checkTimeout)) s"
        )
    }

    static func expectLabels(_ app: XCUIApplication, _ texts: [String], step: String) {
        for text in texts {
            expectLabel(app, text, step: step)
        }
    }

    static func expectTitle(_ app: XCUIApplication, _ title: String, step: String) {
        XCTAssertTrue(
            app.navigationBars[title].waitForExistence(timeout: checkTimeout),
            "\(step): no screen titled \(title) within \(Int(checkTimeout)) s"
        )
    }

    static func expectHelper(_ app: XCUIApplication, _ text: String, timeout: TimeInterval, step: String) {
        XCTAssertTrue(
            JoinJourney.waitForLabel(app.element("amount-entry-helper"), containing: text, timeout: timeout),
            "\(step): the helper does not read \"\(text)\": \(app.element("amount-entry-helper").label)"
        )
    }

    static func tap(_ app: XCUIApplication, _ identifier: String, step: String) {
        let target = app.element(identifier)
        XCTAssertTrue(
            target.waitForExistence(timeout: checkTimeout), "\(step): no \(identifier) within \(Int(checkTimeout)) s")
        target.tap()
    }

    static func buyCabal(run: String) -> String { "QA buy \(run)" }

    static let tradeTimeout: TimeInterval = 120
    static let potTimeout: TimeInterval = 60

    static func expectText(_ app: XCUIApplication, _ text: String, step: String) {
        XCTAssertTrue(
            app.staticTexts[text].firstMatch.waitForExistence(timeout: 10),
            "\(step): no \"\(text)\" within 10 s"
        )
    }

    static func expectTexts(_ app: XCUIApplication, _ texts: [String], step: String) {
        for text in texts {
            expectText(app, text, step: step)
        }
    }

    static func tapLabel(_ app: XCUIApplication, _ label: String, step: String) {
        let target = app.buttons[label].firstMatch
        XCTAssertTrue(target.waitForExistence(timeout: 10), "\(step): no \(label) to tap within 10 s")
        target.tap()
    }

    static func openChooser(_ app: XCUIApplication, step: String) {
        app.buttons["cabal-action-propose"].tap()
        XCTAssertTrue(
            app.element("propose-chooser-sheet").waitForExistence(timeout: checkTimeout),
            "\(step): no Propose action sheet within \(Int(checkTimeout)) s"
        )
    }

    static func review(_ app: XCUIApplication, chip: String, reads summary: String, step: String) {
        tapLabel(app, chip, step: step)
        tapLabel(app, "Review proposal", step: step)
        expectTitle(sheet: app, step: step)
        expectText(app, summary, step: step)
    }

    static func send(_ app: XCUIApplication, to cabal: String, step: String) {
        tapLabel(app, "Send to vote", step: step)
        expectLabel(app, "Proposal sent", step: step)
        expectLabel(app, cabal, step: step)
    }

    static func openCabalAsVoter(_ app: XCUIApplication, _ name: String, step: String) {
        app.openCabal(named: name)
        let propose = app.buttons["cabal-action-propose"]
        app.scrollIntoReach(propose)
        XCTAssertTrue(propose.waitForExistence(timeout: screenTimeout), "\(step): no Propose action")
        XCTAssertTrue(propose.isEnabled, "\(step): Propose is disabled for a voter")
        XCTAssertFalse(
            app.element("cabal-action-propose-caption").exists,
            "\(step): a voter sees Only voters can propose"
        )
    }

    static func proposeBuy(_ app: XCUIApplication, cabal: String, run: String, recorder: JourneyRecorder) {
        recorder.step("S2.1", "open the cabal as a voter") {
            openCabalAsVoter(app, cabal, step: "S2.1")
        }

        recorder.step("S2.2", "open the Propose sheet") {
            app.buttons["cabal-action-propose"].tap()
            let buy = app.buttons["propose-kind-buy"]
            XCTAssertTrue(buy.waitForExistence(timeout: checkTimeout), "S2.2: no \"Buy stock\" in the Propose sheet")
            XCTAssertTrue(buy.label.contains("Buy stock"), "S2.2: the buy row reads \(buy.label)")
            let sell = app.element("propose-kind-sell")
            XCTAssertTrue(
                JoinJourney.waitForLabel(sell, containing: "Sell a holding", timeout: checkTimeout),
                "S2.2: no \"Sell a holding\" row")
            XCTAssertTrue(
                JoinJourney.waitForLabel(sell, containing: "No holdings to sell", timeout: checkTimeout),
                "S2.2: the sell row does not say \"No holdings to sell\"")
            XCTAssertFalse(sell.isEnabled, "S2.2: \"Sell a holding\" can be tapped with nothing to sell")
            XCTAssertTrue(app.buttons["propose-chooser-cancel"].exists, "S2.2: no Cancel in the Propose sheet")
        }

        recorder.step("S2.3", "pick GOOGL") {
            pickStock(app, symbol: "GOOGL", step: "S2.3")
        }

        recorder.step("S2.4", "an amount over the pot is refused") {
            MoneyFundCabalJourney.typeAmount(app, "5")
            XCTAssertTrue(
                app.element("amount-keypad-limit-line").waitForExistence(timeout: 5),
                "S2.4: no amount-keypad-limit-line over the pot within 5 s")
            XCTAssertFalse(app.element("amount-keypad-action").exists, "S2.4: the keypad offers an action over the pot")
        }

        recorder.step("S2.5", "review $1 of GOOGL") {
            tap(app, "amount-keypad-delete", step: "S2.5")
            MoneyFundCabalJourney.typeAmount(app, "1")
            tap(app, "amount-keypad-action", step: "S2.5")
            expectReview(app, cabal: cabal, step: "S2.5")
        }

        recorder.step("S2.6", "send to the cabal") {
            sendAndClose(app, cabal: cabal, step: "S2.6")
        }
    }

    static func pickStock(_ app: XCUIApplication, symbol: String, step: String) {
        tap(app, "propose-kind-buy", step: step)
        expectTitle(app, "Buy stock", step: step)
        let field = app.element("monaco-search-field")
        XCTAssertTrue(field.waitForExistence(timeout: checkTimeout), "\(step): no stock search")
        field.tap()
        field.typeText(symbol)
        let row = app.descendants(matching: .any)
            .matching(NSPredicate(format: "identifier BEGINSWITH %@", "propose-buy-stock-\(symbol)")).firstMatch
        XCTAssertTrue(
            row.waitForExistence(timeout: checkTimeout), "\(step): no \(symbol) row within \(Int(checkTimeout)) s")
        row.tap()
        XCTAssertTrue(
            app.element("amount-keypad-screen").waitForExistence(timeout: checkTimeout),
            "\(step): no keypad within \(Int(checkTimeout)) s"
        )
    }

    static func expectReview(_ app: XCUIApplication, cabal: String, step: String) {
        XCTAssertTrue(
            app.element("propose-review-screen").waitForExistence(timeout: checkTimeout),
            "\(step): no Review screen within \(Int(checkTimeout)) s"
        )
        let order = app.element("propose-review-market-order")
        XCTAssertTrue(
            JoinJourney.waitForLabel(order, containing: "Buy · market order", timeout: checkTimeout),
            "\(step): the stock row does not read Buy · market order: \(order.label)")
        expectLabel(app, cabal, step: step)
        XCTAssertTrue(
            app.element("propose-review-thesis").waitForExistence(timeout: checkTimeout), "\(step): no thesis row")
        let stats = app.element("propose-review-stats")
        XCTAssertTrue(stats.waitForExistence(timeout: checkTimeout), "\(step): no stat row")
        expectLabels(app, ["Shares", "Price", "Of treasury", "To pass"], step: step)
    }

    static func sendAndClose(_ app: XCUIApplication, cabal: String, step: String) {
        tap(app, "propose-review-send", step: step)
        XCTAssertTrue(
            app.element("propose-review-sent").waitForExistence(timeout: checkTimeout),
            "\(step): no sent state within \(Int(checkTimeout)) s")
        tapLabel(app, "Close", step: step)
        XCTAssertTrue(
            JoinJourney.waitForLabel(app.element("cabal-header-name"), containing: cabal, timeout: checkTimeout),
            "\(step): the flow did not close back to the cabal screen"
        )
    }

    static func expectSettlingToast(
        _ app: XCUIApplication, _ text: String, withUndo: Bool, step: String
    ) {
        let toast = app.toastText()
        XCTAssertTrue(toast?.contains(text) == true, "\(step): no toast \"\(text)\": \(toast ?? "none")")
        XCTAssertEqual(
            app.element("monaco-toast-action").exists, withUndo,
            "\(step): the toast \(withUndo ? "has no" : "has an") Undo")
    }

    static func expectStatusLine(_ app: XCUIApplication, passed: String, timeout: TimeInterval, step: String) {
        let line = app.element("proposal-vote-status")
        app.scrollIntoReach(line)
        XCTAssertTrue(
            JoinJourney.waitForLabel(line, containing: "filled at $", timeout: timeout),
            "\(step): the status line does not read filled at $… within \(Int(timeout)) s: \(line.label)"
        )
        XCTAssertTrue(line.label.contains(passed), "\(step): the status line does not read \(passed): \(line.label)")
    }

    static func pendingCard(_ app: XCUIApplication) -> XCUIElement {
        app.descendants(matching: .any).matching(NSPredicate(format: "identifier BEGINSWITH 'proposal-card-'"))
            .firstMatch
    }

    static func openHomeCard(_ app: XCUIApplication, step: String) -> XCUIElement {
        XCTAssertTrue(
            app.tab("Home").waitForExistence(timeout: screenTimeout),
            "\(step): the tab bar did not show within \(Int(screenTimeout)) s"
        )
        app.tab("Home").tap()
        XCTAssertTrue(
            GovernanceVoteJourney.waitForText(app, "Needs your vote", timeout: checkTimeout),
            "\(step): Home shows no proposal needing a vote within \(Int(checkTimeout)) s"
        )
        let card = pendingCard(app)
        app.scrollIntoReach(card)
        XCTAssertTrue(
            card.waitForExistence(timeout: checkTimeout), "\(step): no proposal card within \(Int(checkTimeout)) s")
        return card
    }

    static func secondVoterVotes(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        var proposalID = ""

        recorder.step("S3.1", "Home lists the buy under Your vote") {
            let card = openHomeCard(app, step: "S3.1")
            proposalID = String(card.identifier.dropFirst("proposal-card-".count))
            for text in ["Your vote", "✓ 0 · ✕ 0"] {
                GovernanceVoteJourney.expectText(card, text, step: "S3.1")
            }
        }

        recorder.step("S3.2", "pick yes and hold") {
            app.homeVoteCardPickAndHold(proposalID: proposalID, choice: "yes")
            expectSettlingToast(app, "Voted Yes. 1st of 2.", withUndo: true, step: "S3.2")
            XCTAssertTrue(
                app.element("monaco-toast-banner").waitForNonExistence(timeout: 15),
                "S3.2: the toast did not end within 15 s")
            app.answerVotePermissionSheet(turnOn: false)
        }
    }

    static func firstVoterPasses(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S3.3", "A opens the proposal and settles it") {
            let card = openHomeCard(app, step: "S3.3")
            GovernanceVoteJourney.openProposal(card)
            GovernanceVoteJourney.waitForProposalScreen(app, step: "S3.3")
            app.pickAndHold("yes")
            expectSettlingToast(app, "Voted Yes. Passed 2–0.", withUndo: false, step: "S3.3")
        }

        recorder.step("S3.4", "the status line reads filled") {
            expectStatusLine(app, passed: "Passed 2–0", timeout: tradeTimeout, step: "S3.4")
        }
    }

}

extension GovernanceProposeBuyJourney {
    static func fund(_ app: XCUIApplication, cabal: String, dollars: Int, step: String) {
        app.openCabal(named: cabal)
        MoneyFundCabalJourney.fundOnKeypad(
            app, cabalName: cabal, dollars: "\(dollars)", recorder: MoneyFundCabalJourney.recorder())
    }

    static func fillThePotAsA(_ app: XCUIApplication, cabal: String, recorder: JourneyRecorder) {
        recorder.step("S1.1", "A adds $2 to the pot") {
            fund(app, cabal: cabal, dollars: 2, step: "S1.1")
        }
    }

    static func fillThePotAsB(_ app: XCUIApplication, cabal: String, recorder: JourneyRecorder) {
        recorder.step("S1.2", "B adds $1 to the pot") {
            fund(app, cabal: cabal, dollars: 1, step: "S1.2")
            expectPot(app, "$3.00", timeout: 30, step: "S1.2")
        }
    }

    static func expectPot(_ app: XCUIApplication, _ value: String, timeout: TimeInterval, step: String) {
        let pot = app.element("cabal-pot-value")
        XCTAssertTrue(
            JoinJourney.waitForLabel(pot, containing: value, timeout: timeout),
            "\(step): the pot does not read \(value) within \(Int(timeout)) s: \(pot.label)"
        )
    }

    static func seeTheTrade(_ app: XCUIApplication, cabal: String, recorder: JourneyRecorder) {
        recorder.step("S4.1", "Treasury shows GOOGL and cash") {
            app.openCabal(named: cabal)
            app.openCabalTab("treasury")
            let holding = app.element("cabal-holding-GOOGL")
            app.scrollIntoReach(holding)
            XCTAssertTrue(holding.waitForExistence(timeout: 30), "S4.1: no cabal-holding-GOOGL within 30 s")
            let cash = app.element("cabal-holdings-cash")
            XCTAssertTrue(cash.waitForExistence(timeout: 30), "S4.1: no cabal-holdings-cash within 30 s")
        }

        recorder.step("S4.2", "Treasury history shows the buy") {
            let history = app.element("cabal-treasury-history")
            app.scrollIntoReach(history)
            XCTAssertTrue(history.waitForExistence(timeout: 30), "S4.2: no Treasury history row within 30 s")
            history.tap()
            XCTAssertTrue(
                app.element("cabal-treasury-history-screen").waitForExistence(timeout: 30),
                "S4.2: no Treasury history screen within 30 s")
            let rows = app.descendants(matching: .any)
                .matching(NSPredicate(format: "identifier BEGINSWITH 'cabal-activity-row-'"))
            let bought = rows.matching(NSPredicate(format: "label CONTAINS 'Bought'")).firstMatch
            XCTAssertTrue(bought.waitForExistence(timeout: 30), "S4.2: no history row reads Bought within 30 s")
            for word in ["Pending", "Failed"] {
                XCTAssertFalse(
                    rows.matching(NSPredicate(format: "label CONTAINS %@", word)).firstMatch.exists,
                    "S4.2: a history row reads \(word)"
                )
            }
        }
    }

    static func cashOutAll(_ app: XCUIApplication, cabal: String, step: String) {
        app.openCabal(named: cabal)
        let action = app.buttons["cabal-treasury-cash-out"]
        app.scrollIntoReach(action)
        XCTAssertTrue(action.waitForExistence(timeout: screenTimeout), "\(step): no Cash out action")
        action.tap()
        XCTAssertTrue(app.navigationBars["Cash out"].waitForExistence(timeout: checkTimeout), "\(step): no Cash out")
        tapLabel(app, "All", step: step)
        let submit = app.buttons["cash-out-submit"]
        XCTAssertTrue(
            JoinJourney.waitForLabel(submit, containing: "Cash out $", timeout: screenTimeout) && submit.isEnabled,
            "\(step): the submit does not read Cash out $… and enable within \(Int(screenTimeout)) s: \(submit.label)"
        )
        submit.tap()
        XCTAssertTrue(
            DemoStoryJourney.waitForToast(app, startingWith: "Cashing out $", timeout: checkTimeout),
            "\(step): no Cashing out toast within \(Int(checkTimeout)) s"
        )
        XCTAssertTrue(
            DemoStoryJourney.waitForToast(app, startingWith: "Cashed out $", timeout: tradeTimeout),
            "\(step): no Cashed out toast within \(Int(tradeTimeout)) s"
        )
    }

    static func cashOutAsA(_ app: XCUIApplication, cabal: String, recorder: JourneyRecorder) {
        recorder.step("S5.1", "A cashes out all") {
            cashOutAll(app, cabal: cabal, step: "S5.1")
        }
    }

    static func cashOutAsB(_ app: XCUIApplication, cabal: String, recorder: JourneyRecorder) {
        recorder.step("S5.2", "B cashes out all") {
            cashOutAll(app, cabal: cabal, step: "S5.2")
            expectPot(app, "$0.00", timeout: potTimeout, step: "S5.2")
        }
    }

    static func withdrawAsA(_ app: XCUIApplication, to address: String, recorder: JourneyRecorder) {
        recorder.step("S5.3", "A withdraws Max to the refund address") {
            MoneyWithdrawJourney.withdrawAll(app, to: address, recorder: MoneyWithdrawJourney.recorder())
        }
        recorder.step("S5.4", "A's withdrawal completes") { withdrawalCompletes(app, step: "S5.4") }
    }

    static func withdrawAsB(_ app: XCUIApplication, to address: String, recorder: JourneyRecorder) {
        recorder.step("S5.5", "B withdraws Max to the refund address") {
            MoneyWithdrawJourney.withdrawAll(app, to: address, recorder: MoneyWithdrawJourney.recorder())
        }
        recorder.step("S5.6", "B's withdrawal completes") { withdrawalCompletes(app, step: "S5.6") }
    }

    private static func withdrawalCompletes(_ app: XCUIApplication, step doneStep: String) {
        XCTAssertTrue(
            DemoStoryJourney.waitForToast(app, startingWith: "Withdrawal complete: $", timeout: tradeTimeout),
            "\(doneStep): no Withdrawal complete toast within \(Int(tradeTimeout)) s"
        )
        app.tab("Home").tap()
        let balance = app.element("platform-balance-value")
        XCTAssertTrue(
            JoinJourney.waitForLabel(balance, containing: "$0.00", timeout: potTimeout),
            "\(doneStep): the account balance does not read $0.00 within \(Int(potTimeout)) s: \(balance.label)"
        )
    }
}
