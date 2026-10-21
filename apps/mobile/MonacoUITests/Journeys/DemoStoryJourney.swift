import XCTest

enum DemoStoryJourney {
    static let id = "demo/story"
    static let version = 6

    static let screenTimeout: TimeInterval = 15
    static let formTimeout: TimeInterval = 10
    static let reason = "Super bullish. This stock will only keep growing."

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func cabal(run: String) -> String { "QA story \(run)" }

    static func button(_ app: XCUIApplication, _ label: String) -> XCUIElement {
        app.buttons[label].firstMatch
    }

    static func tapAction(_ app: XCUIApplication, _ identifier: String, run: String, step: String) {
        JoinJourney.openBySearch(app, cabal(run: run), step: step)
        let action = app.buttons[identifier]
        app.scrollIntoReach(action)
        XCTAssertTrue(action.waitForExistence(timeout: formTimeout), "\(step): no \(identifier) on the cabal screen")
        action.tap()
    }

    static func waitForTitle(_ app: XCUIApplication, _ title: String, step: String, blockedBy tickets: String) {
        XCTAssertTrue(
            app.navigationBars[title].waitForExistence(timeout: formTimeout),
            "\(step): no \(title) screen within 10 s (known failure, \(tickets))"
        )
    }

    static func waitForToast(_ app: XCUIApplication, startingWith prefix: String, timeout: TimeInterval) -> Bool {
        app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", prefix)).firstMatch
            .waitForExistence(timeout: timeout)
    }

    static func sendMessage(_ app: XCUIApplication, _ text: String, step: String) {
        let composer = app.textFields["Message your cabal"].firstMatch
        XCTAssertTrue(
            composer.waitForExistence(timeout: formTimeout),
            "\(step): no Message your cabal composer within 10 s (known failure, #623)"
        )
        composer.tap()
        composer.typeText(text + "\n")
        XCTAssertTrue(
            app.staticTexts[text].waitForExistence(timeout: formTimeout), "\(step): '\(text)' did not show within 10 s")
    }

    static func startTheCabal(
        _ app: XCUIApplication, as account: JourneyAccount, run: String, recorder: JourneyRecorder
    ) throws {
        recorder.step("S1.1", "sign in and land on Home") {
            XCTAssertTrue(app.tab("Home").waitForExistence(timeout: 30), "S1.1: the tab bar did not show within 30 s")
            app.tab("Home").tap()
        }

        recorder.step("S1.2", "the profile carries the name") {
            ProfileOverviewJourney.openProfile(app, step: "S1.2")
            XCTAssertTrue(
                app.staticTexts[account.name].waitForExistence(timeout: formTimeout),
                "S1.2: the Profile header does not show \(account.name)"
            )
        }

        try recorder.step("S1.3", "start a cabal and set the rules") {
            try CreateCabalJourney.creatorStartsAnOpenCabal(
                app, run: run, recorder: CreateCabalJourney.recorder())
        }
    }

    static func friendRequestsToJoin(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S1.4", "find the cabal and request to join") {
            CreateCabalJourney.friendRequestsToJoin(
                app, run: run, recorder: CreateCabalJourney.recorder())
        }
    }

    static func fundThePot(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S2.1", "open the cabal") {
            JoinJourney.openBySearch(app, cabal(run: run), step: "S2.1")
        }

        recorder.step("S2.2", "open Fund this cabal") {
            MoneyFundCabalJourney.openFund(app, step: "S2.2")
            XCTAssertTrue(button(app, "Max").exists, "S2.2: no Max")
        }

        recorder.step("S2.3", "fund $1 into the pot") {
            MoneyFundCabalJourney.typeAmount(app, "1")
            button(app, "Fund $1.00").tap()
            MoneyFundCabalJourney.confirmDeposit(app, dollars: 1, cabal: cabal(run: run), step: "S2.3")
        }
    }

    static func browseStocks(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S3.1", "open Journey Alpha") {
            StocksAssetDetailJourney.openAsset(app, StocksAssetDetailJourney.alpha, step: "S3.1")
            XCTAssertEqual(
                app.staticTexts["asset-detail-name"].label, StocksAssetDetailJourney.alpha.name,
                "S3.1: the asset screen is not Journey Alpha"
            )
        }

        recorder.step("S3.2", "switch to 1M then 1Y") {
            for range in ["1M", "1Y"] {
                StocksAssetDetailJourney.rangeChip(app, range).tap()
                XCTAssertTrue(
                    app.element("asset-detail-range-change").waitForExistence(timeout: formTimeout),
                    "S3.2: no range change for \(range)"
                )
            }
        }

        recorder.step("S3.3", "the Pre-IPO listings") {
            app.navigationBars.buttons.firstMatch.tap()
            StocksAssetDetailJourney.openAsset(app, StocksAssetDetailJourney.preIpo, step: "S3.3")
            StocksAssetDetailJourney.scrollToText(app, "Also available from", step: "S3.3")
        }
    }

    static func proposeABuy(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S4.1", "open Propose") {
            tapAction(app, "cabal-action-propose", run: run, step: "S4.1")
            XCTAssertTrue(
                app.buttons["propose-kind-buy"].waitForExistence(timeout: formTimeout),
                "S4.1: no Buy stock row in the Propose sheet")
        }

        recorder.step("S4.2", "propose $1 with a thesis and send") {
            app.buttons["propose-kind-buy"].tap()
            let search = app.element("monaco-search-field")
            XCTAssertTrue(search.waitForExistence(timeout: formTimeout), "S4.2: no stock search")
            search.tap()
            search.typeText("GOOGL")
            let row = app.descendants(matching: .any)
                .matching(NSPredicate(format: "identifier BEGINSWITH 'propose-buy-stock-GOOGL'")).firstMatch
            XCTAssertTrue(row.waitForExistence(timeout: formTimeout), "S4.2: no GOOGL row")
            row.tap()
            XCTAssertTrue(
                app.element("propose-amount-screen").waitForExistence(timeout: formTimeout), "S4.2: no amount screen")
            MoneyFundCabalJourney.typeAmount(app, "1")
            let why = app.element("propose-amount-reason")
            XCTAssertTrue(why.waitForExistence(timeout: formTimeout), "S4.2: no thesis field")
            why.tap()
            why.typeText(reason)
            app.buttons["propose-amount-review"].tap()
            XCTAssertTrue(
                app.element("propose-review-screen").waitForExistence(timeout: formTimeout), "S4.2: no review sheet")
            app.buttons["propose-review-send"].tap()
            XCTAssertTrue(
                app.staticTexts["Proposal sent"].firstMatch.waitForExistence(timeout: formTimeout),
                "S4.2: the sheet does not say Proposal sent")
            app.buttons["Close"].firstMatch.tap()
            let card = GovernanceProposeBuyJourney.pendingCard(app)
            app.scrollIntoReach(card)
            XCTAssertTrue(
                GovernanceVoteJourney.waitForText(card, "2 yes votes needed", timeout: screenTimeout),
                "S4.2: no proposal card needing 2 yes votes")
        }
    }

    static func voteAndBuy(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S4.3", "vote yes from Home") {
            let card = GovernanceProposeBuyJourney.openHomeCard(app, step: "S4.3")
            let proposalID = String(card.identifier.dropFirst("proposal-card-".count))
            GovernanceVoteJourney.openProposal(card)
            GovernanceVoteJourney.waitForProposalScreen(app, step: "S4.3")
            GovernanceVoteJourney.vote(app, proposalID, "Yes", change: false, step: "S4.3")
        }
    }

    static func secondVoteBuys(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S4.4", "the second yes makes the majority and the cabal buys") {
            let card = GovernanceProposeBuyJourney.openHomeCard(app, step: "S4.4")
            let proposalID = String(card.identifier.dropFirst("proposal-card-".count))
            GovernanceVoteJourney.openProposal(card)
            GovernanceVoteJourney.waitForProposalScreen(app, step: "S4.4")
            GovernanceVoteJourney.vote(app, proposalID, "Yes", change: false, step: "S4.4")
            let tracker = app.element("proposal-tracker")
            XCTAssertTrue(
                JoinJourney.waitForLabel(tracker, containing: "Bought, step 3 of 3, done", timeout: 120),
                "S4.4: the Status tracker does not read Bought within 120 s: \(tracker.label)")
        }
    }

    static func chatFirst(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S5.1", "say it in the chat") {
            tapAction(app, "cabal-action-chat", run: run, step: "S5.1")
            sendMessage(app, "In. Told you", step: "S5.1")
        }
    }

    static func chatReply(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S5.2", "read the friend and reply") {
            tapAction(app, "cabal-action-chat", run: run, step: "S5.2")
            XCTAssertTrue(
                app.staticTexts["In. Told you"].waitForExistence(timeout: formTimeout),
                "S5.2: B's message did not show within 10 s (known failure, #623)"
            )
            sendMessage(app, "We own Google now", step: "S5.2")
        }
    }

    static func cashOut(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S7.1", "open Cash out") {
            tapAction(app, "cabal-action-cash-out", run: run, step: "S7.1")
            waitForTitle(app, "Cash out", step: "S7.1", blockedBy: "#657")
            XCTAssertTrue(button(app, "25%").exists, "S7.1: no 25% (known failure, #657)")
        }

        recorder.step("S7.2", "cash out 25%") {
            button(app, "25%").tap()
            app.buttons.matching(NSPredicate(format: "label BEGINSWITH 'Cash out $'")).firstMatch.tap()
            XCTAssertTrue(
                waitForToast(app, startingWith: "Cashing out", timeout: screenTimeout),
                "S7.2: no Cashing out toast within 15 s (known failure, #657)"
            )
        }
    }

    static func seeWhosUp(_ app: XCUIApplication, friend: String, recorder: JourneyRecorder) {
        recorder.step("S8.1", "find the Leaderboard on Home") {
            app.tab("Home").tap()
            let header = app.staticTexts["Leaderboard"].firstMatch
            app.scrollIntoReach(header)
            XCTAssertTrue(header.waitForExistence(timeout: screenTimeout), "S8.1: no Leaderboard on Home")
        }

        recorder.step("S8.2", "the friend is on the People board") {
            let category = app.element("home-leaderboard-category")
            XCTAssertTrue(category.waitForExistence(timeout: screenTimeout), "S8.2: no leaderboard category select")
            category.tap()
            app.buttons["People"].firstMatch.tap()
            let row = app.buttons.matching(
                NSPredicate(format: "identifier BEGINSWITH 'home-leaderboard-row-' AND label CONTAINS %@", friend)
            ).firstMatch
            app.scrollIntoReach(row)
            XCTAssertTrue(
                row.waitForExistence(timeout: screenTimeout), "S8.2: no People board row names \(friend)")
        }
    }
}
