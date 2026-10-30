import XCTest

enum MoneyBestPriceTradeJourney {
    static let id = "money/best-price-trade"
    static let version = 3

    static let screenTimeout: TimeInterval = 15
    static let checkTimeout: TimeInterval = 10
    static let routedTimeout: TimeInterval = 900
    static let potTimeout: TimeInterval = 60
    static let symbol = "AAPL"

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func fillThePot(_ app: XCUIApplication, cabal: String, recorder: JourneyRecorder) {
        recorder.step("S1.1", "A adds $2 to the pot") {
            GovernanceProposeBuyJourney.fund(app, cabal: cabal, dollars: 2, step: "S1.1")
            GovernanceProposeBuyJourney.expectPot(app, "$2.00", timeout: 30, step: "S1.1")
        }
    }

    static func propose(_ app: XCUIApplication, cabal: String, recorder: JourneyRecorder) {
        recorder.step("S2.1", "pick Apple") {
            GovernanceProposeBuyJourney.openCabalAsVoter(app, cabal, step: "S2.1")
            GovernanceProposeBuyJourney.openChooser(app, step: "S2.1")
            GovernanceProposeBuyJourney.pickStock(app, symbol: symbol, step: "S2.1")
        }

        recorder.step("S2.2", "review $1 of Apple") {
            MoneyFundCabalJourney.typeAmount(app, "1")
            GovernanceProposeBuyJourney.tap(app, "amount-keypad-action", step: "S2.2")
            GovernanceProposeBuyJourney.expectReview(app, cabal: cabal, step: "S2.2")
        }

        recorder.step("S2.3", "send to the cabal") {
            GovernanceProposeBuyJourney.sendAndClose(app, cabal: cabal, step: "S2.3")
        }
    }

    static func voteThrough(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S3.1", "Home lists the buy under Your vote") {
            let card = GovernanceProposeBuyJourney.openHomeCard(app, step: "S3.1")
            GovernanceVoteJourney.expectText(card, symbol, step: "S3.1")
            GovernanceVoteJourney.openProposal(card)
            GovernanceVoteJourney.waitForProposalScreen(app, step: "S3.1")
        }

        recorder.step("S3.2", "pick yes and hold") {
            app.pickAndHold("yes")
            GovernanceProposeBuyJourney.expectSettlingToast(
                app, "Voted Yes. Passed 1–0.", withUndo: false, step: "S3.2")
        }

        recorder.step("S3.3", "the status line reads filled") {
            GovernanceProposeBuyJourney.expectStatusLine(
                app, passed: "Passed 1–0", timeout: routedTimeout, step: "S3.3")
        }
    }

    static func seeTheTrade(_ app: XCUIApplication, cabal: String, recorder: JourneyRecorder) {
        recorder.step("S4.1", "Treasury shows Apple and cash") {
            app.openCabal(named: cabal)
            app.openCabalTab("treasury")
            let holding = app.element("cabal-holding-\(symbol)")
            app.scrollIntoReach(holding)
            XCTAssertTrue(holding.waitForExistence(timeout: 30), "S4.1: no cabal-holding-\(symbol) within 30 s")
            XCTAssertTrue(
                app.element("cabal-holdings-cash").waitForExistence(timeout: 30), "S4.1: no cabal-holdings-cash")
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
            JoinJourney.snap(app, "S4-A-trade-history")
        }
    }

    static func refund(_ app: XCUIApplication, cabal: String, to address: String, recorder: JourneyRecorder) {
        recorder.step("S5.1", "A cashes out all") {
            app.openCabal(named: cabal)
            let action = app.buttons["cabal-treasury-cash-out"]
            app.scrollIntoReach(action)
            XCTAssertTrue(action.waitForExistence(timeout: screenTimeout), "S5.1: no Cash out action")
            action.tap()
            XCTAssertTrue(app.navigationBars["Cash out"].waitForExistence(timeout: checkTimeout), "S5.1: no Cash out")
            GovernanceProposeBuyJourney.tapLabel(app, "All", step: "S5.1")
            let submit = app.buttons["cash-out-submit"]
            XCTAssertTrue(
                JoinJourney.waitForLabel(submit, containing: "Cash out $", timeout: screenTimeout) && submit.isEnabled,
                "S5.1: the submit does not read Cash out $… and enable: \(submit.label)"
            )
            submit.tap()
            XCTAssertTrue(
                DemoStoryJourney.waitForToast(app, startingWith: "Cashing out $", timeout: checkTimeout),
                "S5.1: no Cashing out toast within \(Int(checkTimeout)) s"
            )
            XCTAssertTrue(
                DemoStoryJourney.waitForToast(app, startingWith: "Cashed out $", timeout: routedTimeout),
                "S5.1: no Cashed out toast within \(Int(routedTimeout)) s"
            )
            GovernanceProposeBuyJourney.expectPot(app, "$0.00", timeout: potTimeout, step: "S5.1")
        }

        recorder.step("S5.2", "A withdraws Max to the refund address") {
            app.popToRoot()
            MoneyWithdrawJourney.withdrawAll(app, to: address, recorder: MoneyWithdrawJourney.recorder())
        }

        recorder.step("S5.3", "A's withdrawal completes") {
            XCTAssertTrue(
                DemoStoryJourney.waitForToast(app, startingWith: "Withdrawal complete: $", timeout: 120),
                "S5.3: no Withdrawal complete toast within 120 s"
            )
            app.tab("Home").tap()
            let balance = app.element("platform-balance-value")
            XCTAssertTrue(
                MoneyDepositJourney.waitForBalanceReading(balance, containing: "$0.00", timeout: potTimeout),
                "S5.3: the account balance does not read $0.00 within \(Int(potTimeout)) s: \(MoneyDepositJourney.balanceReading(balance))"
            )
        }
    }
}
