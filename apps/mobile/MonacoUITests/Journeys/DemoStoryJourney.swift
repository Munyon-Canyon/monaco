import XCTest

enum DemoStoryJourney {
    static let id = "demo/story"
    static let version = 1

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

    static func startAndShareTheCode(
        _ app: XCUIApplication, as account: JourneyAccount, run: String, recorder: JourneyRecorder
    ) throws -> String {
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

        var code = ""
        try recorder.step("S1.3", "start a cabal and set the rules") {
            code = try CreateCabalJourney.creatorStartsAnOpenCabal(
                app, run: run, recorder: CreateCabalJourney.recorder())
        }

        recorder.step("S1.4", "the invite code is ready to copy") {
            XCTAssertTrue(app.element("cabal-invite-card").exists, "S1.4: no invite card")
            XCTAssertEqual(app.staticTexts["cabal-invite-code"].label.count, 10, "S1.4: the code is not 10 characters")
            XCTAssertTrue(app.buttons["cabal-invite-copy"].exists, "S1.4: no Copy code button")
        }
        return code
    }

    static func friendJoins(_ app: XCUIApplication, run: String, code: String, recorder: JourneyRecorder) {
        recorder.step("S1.5", "paste the code and join") {
            CreateCabalJourney.friendJoinsWithTheCode(
                app, run: run, code: code, recorder: CreateCabalJourney.recorder())
        }
    }

    static func fundThePot(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S2.1", "open the cabal") {
            JoinJourney.openBySearch(app, cabal(run: run), step: "S2.1")
        }

        recorder.step("S2.2", "open Fund this cabal") {
            let fund = app.buttons["cabal-action-fund"]
            app.scrollIntoReach(fund)
            XCTAssertTrue(fund.waitForExistence(timeout: formTimeout), "S2.2: no cabal-action-fund")
            fund.tap()
            waitForTitle(app, "Fund this cabal", step: "S2.2", blockedBy: "#608 #651")
            XCTAssertTrue(button(app, "Max").exists, "S2.2: no Max (known failure, #651)")
        }

        recorder.step("S2.3", "add $1.00 to the pot") {
            button(app, "Max").tap()
            button(app, "Add $1.00 to the pot").tap()
            XCTAssertTrue(
                waitForToast(app, startingWith: "Added $1.00 to \(cabal(run: run))", timeout: 30),
                "S2.3: no Added $1.00 toast within 30 s (known failure, #608 #651)"
            )
        }
    }

    static func browseStocks(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S3.1", "open Journey Alpha") {
            StocksAssetDetailJourney.openAsset(app, StocksAssetDetailJourney.alpha, scrolls: false, step: "S3.1")
            XCTAssertEqual(
                app.staticTexts["asset-detail-name"].label, StocksAssetDetailJourney.alpha.name,
                "S3.1: the asset screen is not Journey Alpha"
            )
        }

        recorder.step("S3.2", "switch to 1M then 1Y") {
            for range in ["1M", "1Y"] {
                app.element("asset-chart-ranges").buttons[range].tap()
                XCTAssertTrue(
                    app.element("asset-detail-range-change").waitForExistence(timeout: formTimeout),
                    "S3.2: no range change for \(range)"
                )
            }
        }

        recorder.step("S3.3", "the Pre-IPO block") {
            app.navigationBars.buttons.firstMatch.tap()
            StocksAssetDetailJourney.openAsset(app, StocksAssetDetailJourney.preIpo, scrolls: true, step: "S3.3")
            StocksAssetDetailJourney.scrollToText(app, "Private-market reference", step: "S3.3")
            StocksAssetDetailJourney.scrollToText(app, "Also available from", step: "S3.3")
        }
    }

    static func proposeABuy(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S4.1", "open Propose") {
            tapAction(app, "cabal-action-propose", run: run, step: "S4.1")
            waitForTitle(app, "Propose", step: "S4.1", blockedBy: "#613")
            XCTAssertTrue(button(app, "Buy a stock").exists, "S4.1: no Buy a stock row (known failure, #613)")
        }

        recorder.step("S4.2", "propose $1 with a reason and send") {
            button(app, "Buy a stock").tap()
            let amount = app.textFields.firstMatch
            XCTAssertTrue(amount.waitForExistence(timeout: formTimeout), "S4.2: no amount field (known failure, #613)")
            amount.tap()
            amount.typeText("1")
            let why = app.textViews.firstMatch
            why.tap()
            why.typeText(reason)
            button(app, "Review").tap()
            button(app, "Send").tap()
            XCTAssertTrue(
                app.staticTexts.matching(NSPredicate(format: "label CONTAINS '1 of 2 voted'")).firstMatch
                    .waitForExistence(timeout: screenTimeout),
                "S4.2: no proposal card with 1 of 2 voted (known failure, #613)"
            )
        }
    }

    static func voteAndBuy(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S4.3", "vote yes from Home") {
            app.tab("Home").tap()
            let header = app.staticTexts["Needs your vote"]
            XCTAssertTrue(
                header.waitForExistence(timeout: screenTimeout),
                "S4.3: no Needs your vote on Home (known failure, #612)")
            button(app, "Yes").tap()
            JoinJourney.waitForToast(app, "Vote in", step: "S4.3")
        }

        recorder.step("S4.4", "majority wins and the cabal buys") {
            XCTAssertTrue(
                app.staticTexts["Bought"].waitForExistence(timeout: 60),
                "S4.4: the proposal did not reach Bought within 60 s (known failure, #612)"
            )
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

    static func addABot(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S6.1", "propose a trading bot") {
            tapAction(app, "cabal-action-propose", run: run, step: "S6.1")
            let bot = button(app, "Add a trading bot")
            XCTAssertTrue(
                bot.waitForExistence(timeout: formTimeout), "S6.1: no Add a trading bot (known failure, #691)")
            bot.tap()
            let amount = app.textFields.firstMatch
            amount.tap()
            amount.typeText("1")
            let name = app.textFields.element(boundBy: 1)
            name.tap()
            name.typeText("Scout")
            button(app, "Send").tap()
            XCTAssertTrue(
                app.staticTexts.matching(NSPredicate(format: "label CONTAINS 'Scout'")).firstMatch
                    .waitForExistence(timeout: screenTimeout),
                "S6.1: no proposal for Scout (known failure, #691)"
            )
        }
    }

    static func connectTheBot(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S6.2", "copy the connect instructions") {
            JoinJourney.openBySearch(app, cabal(run: run), step: "S6.2")
            let header = app.staticTexts["Trading bot"]
            app.scrollIntoReach(header)
            XCTAssertTrue(
                header.waitForExistence(timeout: formTimeout), "S6.2: no Trading bot row (known failure, #691)")
            app.buttons.matching(NSPredicate(format: "label CONTAINS 'Scout'")).firstMatch.tap()
            button(app, "Copy connect instructions").tap()
            JoinJourney.waitForToast(app, "Connect instructions copied", step: "S6.2")
        }

        recorder.step("S6.3", "watch it trade") {
            let trades = app.staticTexts["Trades"]
            app.scrollIntoReach(trades)
            XCTAssertTrue(
                trades.waitForExistence(timeout: 30), "S6.3: no Trades on the bot screen (known failure, #691)")
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
        recorder.step("S8.1", "find Top investors on Home") {
            app.tab("Home").tap()
            let header = app.staticTexts["Top investors"]
            app.scrollIntoReach(header)
            XCTAssertTrue(header.waitForExistence(timeout: screenTimeout), "S8.1: no Top investors on Home")
        }

        recorder.step("S8.2", "the friend is on the board") {
            let row = app.buttons.matching(NSPredicate(format: "label CONTAINS %@", friend)).firstMatch
            app.scrollIntoReach(row)
            XCTAssertTrue(
                row.waitForExistence(timeout: screenTimeout), "S8.2: no board row names \(friend) (known failure, #617)"
            )
        }
    }
}
