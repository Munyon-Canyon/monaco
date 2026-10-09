import XCTest

enum GovernanceProposeBuyJourney {
    static let id = "governance/propose-buy"
    static let version = 6

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
        expectTitle(app, "Propose", step: step)
    }

    static func review(_ app: XCUIApplication, chip: String, reads summary: String, step: String) {
        tapLabel(app, chip, step: step)
        tapLabel(app, "Review", step: step)
        expectTitle(app, "Review", step: step)
        expectText(app, summary, step: step)
    }

    static func send(_ app: XCUIApplication, to cabal: String, step: String) {
        tapLabel(app, "Send to cabal", step: step)
        JoinJourney.waitForToast(app, "Proposal sent to \(cabal)", step: step)
    }

    static func openCabalAsVoter(_ app: XCUIApplication, _ name: String, step: String) {
        JoinJourney.openBySearch(app, name, step: step)
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

        recorder.step("S2.2", "open the Propose chooser") {
            openChooser(app, step: "S2.2")
            expectLabels(app, ["Buy a stock", "Your cabal votes on it first", "Nothing to sell yet"], step: "S2.2")
            let sell = app.element("propose-kind-sell")
            XCTAssertTrue(
                JoinJourney.waitForLabel(sell, containing: "Sell something the cabal owns", timeout: checkTimeout),
                "S2.2: no \"Sell something the cabal owns\" row")
            XCTAssertFalse(
                app.buttons["propose-kind-sell"].exists,
                "S2.2: \"Sell something the cabal owns\" can be tapped with nothing to sell")
        }

        recorder.step("S2.3", "pick GOOGL") {
            tap(app, "propose-kind-buy", step: "S2.3")
            expectTitle(app, "Buy", step: "S2.3")
            expectLabel(app, "Popular", step: "S2.3")
            let field = app.element("monaco-search-field")
            XCTAssertTrue(field.waitForExistence(timeout: checkTimeout), "S2.3: no stock search")
            field.tap()
            field.typeText("GOOGL")
            let row = app.descendants(matching: .any)
                .matching(NSPredicate(format: "identifier BEGINSWITH 'propose-buy-stock-GOOGL'")).firstMatch
            XCTAssertTrue(
                row.waitForExistence(timeout: checkTimeout), "S2.3: no GOOGL row within \(Int(checkTimeout)) s")
            row.tap()
            XCTAssertTrue(
                app.element("propose-amount-screen").waitForExistence(timeout: checkTimeout),
                "S2.3: no Amount screen within \(Int(checkTimeout)) s"
            )
        }

        recorder.step("S2.4", "an amount over the pot is refused") {
            MoneyFundCabalJourney.typeKeypadAmount(app, "5")
            expectLabel(app, "More than the pot has", timeout: 5, step: "S2.4")
            XCTAssertFalse(app.buttons["propose-amount-review"].isEnabled, "S2.4: Review is enabled over the pot")
        }

        recorder.step("S2.5", "review $1 of GOOGL with a reason") {
            MoneyFundCabalJourney.typeKeypadAmount(app, "1")
            expectHelper(app, "The pot has $3.00", timeout: checkTimeout, step: "S2.5")
            tap(app, "propose-amount-add-reason", step: "S2.5")
            let why = app.element("propose-amount-reason")
            XCTAssertTrue(why.waitForExistence(timeout: checkTimeout), "S2.5: no reason field")
            why.tap()
            why.typeText(reason(run: run))
            tap(app, "propose-amount-review", step: "S2.5")
            XCTAssertTrue(
                app.element("propose-review-screen").waitForExistence(timeout: checkTimeout),
                "S2.5: no Review screen within \(Int(checkTimeout)) s"
            )
            expectLabels(
                app, ["Buy $1.00 of GOOGL", "Cabal gets", "Price", "Pot", "Who votes", "Why buy", reason(run: run)],
                step: "S2.5")
        }

        recorder.step("S2.6", "send to the cabal") {
            tap(app, "propose-review-send", step: "S2.6")
            JoinJourney.waitForToast(app, "Proposal sent to \(cabal)", step: "S2.6")
            XCTAssertTrue(
                JoinJourney.waitForLabel(app.element("cabal-header-name"), containing: cabal, timeout: checkTimeout),
                "S2.6: the flow did not close back to the cabal screen"
            )
        }
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
            "\(step): Home shows no \"Needs your vote\" within \(Int(checkTimeout)) s"
        )
        let card = pendingCard(app)
        app.scrollIntoReach(card)
        XCTAssertTrue(
            card.waitForExistence(timeout: checkTimeout), "\(step): no proposal card within \(Int(checkTimeout)) s")
        return card
    }

    static func secondVoterVotes(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S3.1", "Home lists the buy under Needs your vote") {
            let card = openHomeCard(app, step: "S3.1")
            for text in ["GOOGL", "Closes in", reason(run: run), "0 of 2 voted · 2 yes to pass"] {
                GovernanceVoteJourney.expectText(card, text, step: "S3.1")
            }
        }

        recorder.step("S3.2", "vote yes") {
            let card = pendingCard(app)
            let proposalID = String(card.identifier.dropFirst("proposal-card-".count))
            GovernanceVoteJourney.vote(app, proposalID, "Yes", change: false, step: "S3.2")
            GovernanceVoteJourney.expectText(card, "✓ You voted yes", step: "S3.2")
            GovernanceVoteJourney.expectText(card, "1 of 2 voted · 2 yes to pass", step: "S3.2")
        }
    }

    static func firstVoterPasses(
        _ app: XCUIApplication, proposer: String, voter: String, recorder: JourneyRecorder
    ) {
        recorder.step("S3.3", "A sees B's ballot and votes yes") {
            let card = openHomeCard(app, step: "S3.3")
            let proposalID = String(card.identifier.dropFirst("proposal-card-".count))
            let header = card.descendants(matching: .any).matching(identifier: "proposal-closes-in").firstMatch
            XCTAssertTrue(header.waitForExistence(timeout: checkTimeout), "S3.3: the card has no header to tap")
            header.tap()
            GovernanceVoteJourney.waitForProposalScreen(app, step: "S3.3")
            expectLabel(app, "Proposed by \(proposer)", timeout: screenTimeout, step: "S3.3")
            expectLabel(app, "\(voter) voted yes", timeout: screenTimeout, step: "S3.3")
            GovernanceVoteJourney.vote(app, proposalID, "Yes", change: false, step: "S3.3")
        }

        recorder.step("S3.4", "the passed vote buys") {
            let tracker = app.element("proposal-tracker")
            XCTAssertTrue(
                JoinJourney.waitForLabel(tracker, containing: "Bought, step 3 of 3, done", timeout: tradeTimeout),
                "S3.4: the Status tracker does not read Bought within \(Int(tradeTimeout)) s: \(tracker.label)"
            )
            let chip = app.element("proposal-status-chip")
            XCTAssertTrue(
                JoinJourney.waitForLabel(chip, containing: "Bought", timeout: tradeTimeout),
                "S3.4: the proposal chip does not read Bought within \(Int(tradeTimeout)) s: \(chip.label)"
            )
        }
    }

}

extension GovernanceProposeBuyJourney {
    static func fund(_ app: XCUIApplication, cabal: String, dollars: Int, step: String) {
        JoinJourney.openBySearch(app, cabal, step: step)
        MoneyFundCabalJourney.openFund(app, step: step)
        MoneyFundCabalJourney.typeAmount(app, "\(dollars)")
        tapLabel(app, "Add $\(dollars) to the pot", step: step)
        XCTAssertTrue(
            app.staticTexts["Added $\(dollars) to \(cabal)."].firstMatch.waitForExistence(timeout: potTimeout),
            "\(step): the toast \"Added $\(dollars) to \(cabal).\" did not show within \(Int(potTimeout)) s"
        )
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
        recorder.step("S4.1", "Holdings shows GOOGL and cash") {
            JoinJourney.openBySearch(app, cabal, step: "S4.1")
            let holding = app.descendants(matching: .any)
                .matching(NSPredicate(format: "identifier == 'cabal-holding-GOOGL'")).firstMatch
            app.scrollIntoReach(holding)
            XCTAssertTrue(holding.waitForExistence(timeout: 30), "S4.1: no cabal-holding-GOOGL within 30 s")
            let cash = app.element("cabal-holdings-cash")
            XCTAssertTrue(cash.waitForExistence(timeout: 30), "S4.1: no cabal-holdings-cash within 30 s")
        }

        recorder.step("S4.2", "Activity shows the buy") {
            let activity = app.element("cabal-activity")
            app.scrollIntoReach(activity)
            let rows = app.descendants(matching: .any)
                .matching(NSPredicate(format: "identifier BEGINSWITH 'cabal-activity-row-'"))
            let bought = rows.matching(NSPredicate(format: "label CONTAINS 'Bought'")).firstMatch
            XCTAssertTrue(bought.waitForExistence(timeout: 30), "S4.2: no Activity row reads Bought within 30 s")
            for word in ["Pending", "Failed"] {
                XCTAssertFalse(
                    rows.matching(NSPredicate(format: "label CONTAINS %@", word)).firstMatch.exists,
                    "S4.2: an Activity row reads \(word)"
                )
            }
        }
    }

    static func cashOutAll(_ app: XCUIApplication, cabal: String, step: String) {
        JoinJourney.openBySearch(app, cabal, step: step)
        let action = app.buttons["cabal-action-cash-out"]
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
