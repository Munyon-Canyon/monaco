import XCTest

enum MoneyBestPriceTradeJourney {
    static let id = "money/best-price-trade"
    static let version = 1

    static let screenTimeout: TimeInterval = 15
    static let checkTimeout: TimeInterval = 10
    static let comparisonTimeout: TimeInterval = 30
    static let routedTimeout: TimeInterval = 900
    static let potTimeout: TimeInterval = 60
    static let symbol = "AAPL"
    static let issuers = ["xStocks", "xStock", "Ondo", "Coinbase"]

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func venueCount(_ label: String) -> Int? {
        guard let range = label.range(of: #"Best price across [0-9]+ venue"#, options: .regularExpression) else {
            return nil
        }
        return Int(label[range].filter(\.isNumber))
    }

    static func expectComparison(_ app: XCUIApplication, timeout: TimeInterval, step: String) {
        let row = app.element("best-price-row")
        app.scrollIntoReach(row)
        XCTAssertTrue(
            JoinJourney.waitForLabel(row, containing: "Best price across", timeout: timeout),
            "\(step): no best price within \(Int(timeout)) s"
        )
        let count = venueCount(row.label) ?? 0
        XCTAssertGreaterThanOrEqual(count, 2, "\(step): the best price compares fewer than 2 venues: \(row.label)")
    }

    static func fillThePot(_ app: XCUIApplication, cabal: String, recorder: JourneyRecorder) {
        recorder.step("S1.1", "A adds $2 to the pot") {
            GovernanceProposeBuyJourney.fund(app, cabal: cabal, dollars: 2, step: "S1.1")
            GovernanceProposeBuyJourney.expectPot(app, "$2.00", timeout: 30, step: "S1.1")
        }
    }

    static func proposeAndCompare(_ app: XCUIApplication, cabal: String, recorder: JourneyRecorder) {
        recorder.step("S2.1", "pick Apple") {
            GovernanceProposeBuyJourney.openCabalAsVoter(app, cabal, step: "S2.1")
            GovernanceProposeBuyJourney.openChooser(app, step: "S2.1")
            GovernanceProposeBuyJourney.tap(app, "propose-kind-buy", step: "S2.1")
            let field = app.element("monaco-search-field")
            XCTAssertTrue(field.waitForExistence(timeout: checkTimeout), "S2.1: no stock search")
            field.tap()
            field.typeText(symbol)
            let row = app.descendants(matching: .any)
                .matching(NSPredicate(format: "identifier BEGINSWITH %@", "propose-buy-stock-\(symbol)")).firstMatch
            XCTAssertTrue(row.waitForExistence(timeout: checkTimeout), "S2.1: no \(symbol) row")
            row.tap()
            XCTAssertTrue(
                app.element("propose-amount-screen").waitForExistence(timeout: checkTimeout),
                "S2.1: no Amount screen within \(Int(checkTimeout)) s"
            )
        }

        recorder.step("S2.2", "Review shows the best price") {
            MoneyFundCabalJourney.typeKeypadAmount(app, "1")
            GovernanceProposeBuyJourney.tap(app, "propose-amount-review", step: "S2.2")
            XCTAssertTrue(
                app.element("propose-review-screen").waitForExistence(timeout: checkTimeout),
                "S2.2: no Review screen within \(Int(checkTimeout)) s"
            )
            GovernanceProposeBuyJourney.expectLabel(app, "Buy $1.00 of \(symbol)", step: "S2.2")
            expectComparison(app, timeout: comparisonTimeout, step: "S2.2")
            let row = app.element("best-price-row")
            XCTAssertTrue(row.label.contains("a share"), "S2.2: the best price has no price a share: \(row.label)")
            JoinJourney.snap(app, "S2-A-review-best-price")
        }

        recorder.step("S2.3", "the sheet lists every venue") {
            app.element("best-price-row").tap()
            let winner = app.element("best-price-winner")
            XCTAssertTrue(winner.waitForExistence(timeout: checkTimeout), "S2.3: no winning venue within 10 s")
            XCTAssertTrue(winner.label.contains("Best price"), "S2.3: the winner is not marked: \(winner.label)")
            let others = app.descendants(matching: .any).matching(identifier: "best-price-venue")
            XCTAssertGreaterThanOrEqual(others.count, 1, "S2.3: the sheet lists no other venue")
            var rows = [winner]
            for index in 0..<others.count { rows.append(others.element(boundBy: index)) }
            for row in rows {
                XCTAssertTrue(
                    row.label.contains(" on "), "S2.3: a venue is not named <exchange> on <network>: \(row.label)")
                for issuer in issuers {
                    XCTAssertFalse(row.label.contains(issuer), "S2.3: a venue names the issuer \(issuer): \(row.label)")
                }
                XCTAssertLessThanOrEqual(winner.frame.minY, row.frame.minY, "S2.3: the winner is not first")
            }
            JoinJourney.snap(app, "S2-A-best-price-sheet")
        }

        recorder.step("S2.4", "close the sheet and send") {
            GovernanceProposeBuyJourney.tap(app, "best-price-done", step: "S2.4")
            XCTAssertTrue(
                app.element("best-price-winner").waitForNonExistence(timeout: checkTimeout),
                "S2.4: the sheet did not close")
            GovernanceProposeBuyJourney.tap(app, "propose-review-send", step: "S2.4")
            JoinJourney.waitForToast(app, "Proposal sent to \(cabal)", step: "S2.4")
            XCTAssertTrue(
                JoinJourney.waitForLabel(app.element("cabal-header-name"), containing: cabal, timeout: checkTimeout),
                "S2.4: the flow did not close back to the cabal screen"
            )
        }
    }

    static func voteThrough(_ app: XCUIApplication, recorder: JourneyRecorder) {
        var proposalID = ""

        recorder.step("S3.1", "the proposal shows the best price") {
            let card = GovernanceProposeBuyJourney.openHomeCard(app, step: "S3.1")
            proposalID = String(card.identifier.dropFirst("proposal-card-".count))
            GovernanceVoteJourney.expectText(card, symbol, step: "S3.1")
            let header = card.descendants(matching: .any).matching(identifier: "proposal-closes-in").firstMatch
            XCTAssertTrue(header.waitForExistence(timeout: checkTimeout), "S3.1: the card has no header to tap")
            header.tap()
            GovernanceVoteJourney.waitForProposalScreen(app, step: "S3.1")
            expectComparison(app, timeout: screenTimeout, step: "S3.1")
        }

        recorder.step("S3.2", "vote yes") {
            GovernanceVoteJourney.vote(app, proposalID, "Yes", change: false, step: "S3.2")
        }

        recorder.step("S3.3", "the passed vote buys") {
            let tracker = app.element("proposal-tracker")
            app.scrollIntoReach(tracker)
            XCTAssertTrue(
                JoinJourney.waitForLabel(tracker, containing: "Bought, step 3 of 3, done", timeout: routedTimeout),
                "S3.3: the Status tracker does not read Bought within \(Int(routedTimeout)) s: \(tracker.label)"
            )
            let chip = app.element("proposal-status-chip")
            XCTAssertTrue(
                JoinJourney.waitForLabel(chip, containing: "Bought", timeout: checkTimeout),
                "S3.3: the proposal chip does not read Bought: \(chip.label)"
            )
        }
    }

    static func seeTheTrade(_ app: XCUIApplication, cabal: String, recorder: JourneyRecorder) {
        recorder.step("S4.1", "Holdings shows Apple and cash") {
            JoinJourney.openBySearch(app, cabal, step: "S4.1")
            let holding = app.element("cabal-holding-\(symbol)")
            app.scrollIntoReach(holding)
            XCTAssertTrue(holding.waitForExistence(timeout: 30), "S4.1: no cabal-holding-\(symbol) within 30 s")
            XCTAssertTrue(
                app.element("cabal-holdings-cash").waitForExistence(timeout: 30), "S4.1: no cabal-holdings-cash")
        }

        recorder.step("S4.2", "the trade shows its best price") {
            let activity = app.element("cabal-activity")
            app.scrollIntoReach(activity)
            let rows = app.descendants(matching: .any)
                .matching(NSPredicate(format: "identifier BEGINSWITH 'cabal-activity-row-'"))
            let bought = rows.matching(NSPredicate(format: "label CONTAINS 'Bought'")).firstMatch
            XCTAssertTrue(bought.waitForExistence(timeout: 30), "S4.2: no Activity row reads Bought within 30 s")
            app.scrollIntoReach(bought)
            bought.tap()
            XCTAssertTrue(
                app.navigationBars["Transaction"].waitForExistence(timeout: 30), "S4.2: no Transaction screen")
            let status = app.element("cabal-txn-status")
            XCTAssertTrue(status.waitForExistence(timeout: 30), "S4.2: no status on the trade")
            XCTAssertFalse(
                status.label.contains("In progress"), "S4.2: the trade is still in progress: \(status.label)")
            expectComparison(app, timeout: 30, step: "S4.2")
            JoinJourney.snap(app, "S4-A-trade-best-price")
        }
    }

    static func refund(_ app: XCUIApplication, cabal: String, to address: String, recorder: JourneyRecorder) {
        recorder.step("S5.1", "A cashes out all") {
            JoinJourney.openBySearch(app, cabal, step: "S5.1")
            let action = app.buttons["cabal-action-cash-out"]
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
                JoinJourney.waitForLabel(balance, containing: "$0.00", timeout: potTimeout),
                "S5.3: the account balance does not read $0.00 within \(Int(potTimeout)) s: \(balance.label)"
            )
        }
    }
}
