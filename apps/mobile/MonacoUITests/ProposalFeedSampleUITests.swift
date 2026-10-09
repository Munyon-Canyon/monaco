import XCTest

nonisolated final class ProposalFeedSampleUITests: XCTestCase {
    @MainActor private var app: XCUIApplication!

    @MainActor
    private func launch(_ scenario: String) {
        continueAfterFailure = false
        app = XCUIApplication()
        app.launchArguments = ["-MonacoProposalSample", scenario]
        app.launch()
    }

    @MainActor
    private func capture(_ name: String) {
        let shot = XCUIScreen.main.screenshot()
        let attachment = XCTAttachment(screenshot: shot)
        attachment.name = name
        attachment.lifetime = .keepAlways
        add(attachment)
        if let dir = ProcessInfo.processInfo.environment["MONACO_QA_SCREENSHOT_DIR"], !dir.isEmpty {
            try? shot.pngRepresentation.write(to: URL(fileURLWithPath: dir).appendingPathComponent("\(name).png"))
        }
    }

    @MainActor
    private func element(_ id: String) -> XCUIElement {
        app.descendants(matching: .any).matching(identifier: id).firstMatch
    }

    @MainActor
    private func waitUntilHittable(_ element: XCUIElement, timeout: TimeInterval = 5) -> Bool {
        let hittable = expectation(for: NSPredicate(format: "isHittable == true"), evaluatedWith: element)
        return XCTWaiter().wait(for: [hittable], timeout: timeout) == .completed
    }

    @MainActor
    private func scrollIntoReach(_ element: XCUIElement, maxSwipes: Int = 8) {
        app.scrollIntoReach(element, maxSwipes: maxSwipes)
    }

    @MainActor
    func testNeedsVote_voteFromCard_thenCommentAndReplyInThread() throws {
        launch("needsVote")
        let first = element("proposal-card-proposal-1")
        XCTAssertTrue(first.waitForExistence(timeout: 10), "the first vote card never drew")
        let yes = first.buttons["Yes"]
        XCTAssertTrue(yes.exists, "the first card should offer Yes")
        XCTAssertTrue(first.buttons["No"].exists, "the first card should offer No")
        XCTAssertTrue(first.staticTexts["Buy"].exists, "the card should say which way the trade goes")
        capture("01-votes")

        yes.tap()
        XCTAssertTrue(app.staticTexts["Vote recorded."].waitForExistence(timeout: 5), "no toast after voting")
        XCTAssertTrue(first.buttons["Change"].waitForExistence(timeout: 5), "the card should offer Change after a vote")
        XCTAssertFalse(first.buttons["Yes"].exists, "Yes should be gone once voted")
        capture("02-after-card-vote")

        let last = element("proposal-card-proposal-2")
        var swipes = 0
        while !last.exists && swipes < 6 {
            app.swipeUp()
            swipes += 1
        }
        XCTAssertTrue(last.exists, "the second open card never rendered")
        capture("03-votes-scrolled")
        while !first.isHittable && swipes > 0 {
            app.swipeDown()
            swipes -= 1
        }

        first.coordinate(withNormalizedOffset: .zero).withOffset(CGVector(dx: 120, dy: 40)).tap()
        XCTAssertTrue(element("proposal-reason").waitForExistence(timeout: 10), "the thesis block is missing")
        XCTAssertTrue(element("comment-thread").waitForExistence(timeout: 10), "the thread never drew")
        let quotes = app.staticTexts.matching(NSPredicate(format: "label == %@", "Earnings next week."))
        XCTAssertEqual(quotes.count, 1, "the thesis should be quoted once")
        scrollIntoReach(element("comment-row-c2"))
        XCTAssertTrue(element("comment-row-c2").exists, "the nested reply is missing")
        capture("04-detail-thread")

        let field = element("comment-composer-field")
        field.tap()
        field.typeText("Count me in if we cap it at $25.")
        let keyboardWasUp = app.keyboards.firstMatch.exists
        element("comment-composer-send").tap()
        XCTAssertTrue(
            app.staticTexts["Count me in if we cap it at $25."].waitForExistence(timeout: 5),
            "the posted comment never reached the thread")
        if keyboardWasUp {
            XCTAssertTrue(
                app.keyboards.firstMatch.waitForNonExistence(timeout: 5), "the keyboard stayed up over the thread")
        }
        XCTAssertTrue(
            waitUntilHittable(app.staticTexts["Count me in if we cap it at $25."]),
            "the posted comment is off screen or behind the composer"
        )
        capture("05-comment-posted")

        let reply = element("comment-reply-c1")
        XCTAssertTrue(waitUntilHittable(reply), "Reply is not reachable after posting")
        reply.tap()
        XCTAssertTrue(app.staticTexts["Replying to Maya"].waitForExistence(timeout: 5), "no reply target shown")
        field.tap()
        field.typeText("Agreed, Nvidia next.")
        element("comment-composer-send").tap()
        XCTAssertTrue(
            app.staticTexts["Agreed, Nvidia next."].waitForExistence(timeout: 5),
            "the posted reply never reached the thread")
        XCTAssertTrue(
            app.staticTexts["Replying to Maya"].waitForNonExistence(timeout: 5),
            "the reply target should clear after posting")
        XCTAssertTrue(
            app.staticTexts["6"].waitForExistence(timeout: 5), "the thread count should read 6 after two posts")
        XCTAssertTrue(
            waitUntilHittable(app.staticTexts["Agreed, Nvidia next."]),
            "the posted reply is off screen or behind the composer"
        )
        capture("06-reply-posted")
    }

    @MainActor
    func testComposer_whitespaceOnly_keepsPostDisabled() throws {
        launch("open")
        let field = element("comment-composer-field")
        XCTAssertTrue(field.waitForExistence(timeout: 10), "the composer never drew")
        XCTAssertTrue(element("comment-thread").waitForExistence(timeout: 10), "the thread never drew")
        scrollIntoReach(element("comment-row-c1"))
        field.tap()
        field.typeText("   ")

        XCTAssertFalse(element("comment-composer-send").isEnabled, "Post should stay disabled for whitespace")
        XCTAssertTrue(element("comment-row-c1").exists, "the sample thread should be on screen")
        XCTAssertFalse(element("comment-thread-empty").exists, "a thread with comments is not the empty state")
    }

    @MainActor
    func testReadOnlyProposal_showsTallyWithoutButtons() throws {
        launch("readOnly")
        let card = element("proposal-card-proposal-1")
        XCTAssertTrue(card.waitForExistence(timeout: 10), "the proposal card never drew")
        XCTAssertTrue(element("proposal-votes").waitForExistence(timeout: 10), "the votes block never drew")
        XCTAssertTrue(
            app.staticTexts["1 yes · 0 no · 2 not voted"].exists, "the tally line is missing or wrong")
        XCTAssertFalse(card.buttons["Yes"].exists, "a read-only proposal must not offer Yes")
        XCTAssertFalse(card.buttons["No"].exists, "a read-only proposal must not offer No")
    }
}

nonisolated final class ProposeFlowSampleUITests: XCTestCase {
    @MainActor private var app: XCUIApplication!

    @MainActor
    private func launch(_ scenario: String) {
        continueAfterFailure = false
        app = XCUIApplication()
        app.launchArguments = ["-MonacoProposeSample", scenario]
        app.launch()
    }

    @MainActor
    private func element(_ id: String) -> XCUIElement {
        app.descendants(matching: .any).matching(identifier: id).firstMatch
    }

    @MainActor
    private func capture(_ name: String) {
        let shot = XCUIScreen.main.screenshot()
        let attachment = XCTAttachment(screenshot: shot)
        attachment.name = name
        attachment.lifetime = .keepAlways
        add(attachment)
        if let dir = ProcessInfo.processInfo.environment["MONACO_QA_SCREENSHOT_DIR"], !dir.isEmpty {
            try? shot.pngRepresentation.write(to: URL(fileURLWithPath: dir).appendingPathComponent("\(name).png"))
        }
    }

    @MainActor
    private func waitUntilEnabled(_ element: XCUIElement, timeout: TimeInterval = 10) -> Bool {
        let enabled = expectation(for: NSPredicate(format: "isEnabled == true"), evaluatedWith: element)
        return XCTWaiter().wait(for: [enabled], timeout: timeout) == .completed
    }

    @MainActor
    private func assertAboveReview(_ field: XCUIElement, file: StaticString = #filePath, line: UInt = #line) {
        let review = element("propose-amount-review")
        XCTAssertTrue(field.isHittable, "reason field is covered", file: file, line: line)
        XCTAssertLessThanOrEqual(
            field.frame.maxY, review.frame.minY, "reason field runs under Review", file: file, line: line)
    }

    @MainActor
    func testBuy_threeSteps_sendsToCabal() throws {
        launch("chooser")
        let buy = element("propose-kind-buy")
        XCTAssertTrue(buy.waitForExistence(timeout: 10), "the chooser never drew")
        sleep(1)
        capture("10-chooser")
        buy.tap()

        let alphabet = element("propose-buy-stock-GOOGLx")
        XCTAssertTrue(alphabet.waitForExistence(timeout: 5), "the stock list never drew Alphabet")
        sleep(1)
        capture("11-pick-stock")
        alphabet.tap()

        let preset = app.buttons["$50"]
        XCTAssertTrue(preset.waitForExistence(timeout: 5), "the amount step never drew its presets")
        XCTAssertEqual(element("amount-entry-field").label, "Amount", "the figure should be named Amount")
        XCTAssertEqual(element("amount-entry-field").value as? String, "$0", "the empty figure should read $0")
        sleep(1)
        capture("12a-amount-empty")
        preset.tap()
        sleep(1)
        capture("12-amount")

        element("amount-keypad-0").tap()
        element("amount-keypad-0").tap()
        XCTAssertTrue(
            app.staticTexts["More than the pot has"].waitForExistence(timeout: 5),
            "$5,000 should read as more than the pot has")
        capture("13-amount-over")
        preset.tap()

        element("propose-amount-add-reason").tap()
        let thesis = element("propose-amount-reason")
        XCTAssertTrue(thesis.waitForExistence(timeout: 3), "the reason field never drew")
        thesis.typeText("Earnings Thursday.")
        sleep(1)
        assertAboveReview(thesis)
        capture("13b-amount-reason")

        let review = element("propose-amount-review")
        XCTAssertTrue(waitUntilEnabled(review), "Review stayed disabled for a valid amount")
        review.tap()
        let send = element("propose-review-send")
        XCTAssertTrue(send.waitForExistence(timeout: 5), "the review screen never drew")
        XCTAssertEqual(send.label, "Send to cabal", "the send button should say where it goes")
        XCTAssertTrue(app.staticTexts["Earnings Thursday."].exists, "the thesis should repeat on the receipt")
        sleep(1)
        capture("14-review")
        send.tap()

        let sent = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "Proposal sent to")).firstMatch
        XCTAssertTrue(sent.waitForExistence(timeout: 5), "no toast after sending")
        capture("15-sent")
    }

    @MainActor
    func testFlow_keepsSheetFullHeightWhenDragged() throws {
        launch("amount")
        let addReason = element("propose-amount-add-reason")
        XCTAssertTrue(addReason.waitForExistence(timeout: 10), "the amount step never drew")
        sleep(1)

        let title = app.navigationBars["Amount"]
        XCTAssertTrue(title.exists, "the Amount bar never drew")
        let before = title.frame.minY
        title.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5))
            .press(
                forDuration: 0.1,
                thenDragTo: app.windows.firstMatch.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)))
        sleep(1)
        capture("16-flow-after-drag")
        XCTAssertTrue(title.exists, "the screen closed")
        XCTAssertEqual(title.frame.minY, before, accuracy: 2, "the screen dropped from full height")
        XCTAssertTrue(addReason.isHittable, "Add a reason slid out of reach")
        XCTAssertTrue(element("propose-amount-review").isHittable, "Review slid out of reach")
    }

    @MainActor
    func testSell_dollarsToShares_reviewShowsEstimate() throws {
        launch("chooser")
        let sell = element("propose-kind-sell")
        XCTAssertTrue(sell.waitForExistence(timeout: 10), "the chooser never drew a sell row")
        sell.tap()

        let alphabet = element("propose-sell-GOOGLx")
        XCTAssertTrue(alphabet.waitForExistence(timeout: 5), "the sell list never drew Alphabet")
        sleep(1)
        capture("20-sell-pick")
        alphabet.tap()

        let half = app.buttons["50%"]
        XCTAssertTrue(half.waitForExistence(timeout: 5), "the sell amount step never drew its presets")
        half.tap()
        element("propose-amount-add-reason").tap()
        let thesis = element("propose-amount-reason")
        XCTAssertTrue(thesis.waitForExistence(timeout: 3), "the reason field never drew")
        thesis.typeText("Take some profit before earnings.")
        sleep(1)
        assertAboveReview(thesis)
        capture("21-sell-amount")

        let review = element("propose-amount-review")
        XCTAssertTrue(waitUntilEnabled(review), "Review stayed disabled for a valid sell")
        review.tap()
        let send = element("propose-review-send")
        XCTAssertTrue(send.waitForExistence(timeout: 5), "the review screen never drew")
        XCTAssertTrue(
            app.staticTexts["Take some profit before earnings."].exists, "the thesis should repeat on the receipt")
        XCTAssertTrue(app.staticTexts["Raises"].exists, "a sell review should estimate what it raises")
        sleep(1)
        capture("22-sell-review")
        send.tap()
        let sent = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "Proposal sent to")).firstMatch
        XCTAssertTrue(sent.waitForExistence(timeout: 5), "no toast after sending")
    }
}

nonisolated final class ProposeFromStockSampleUITests: XCTestCase {
    @MainActor private var app: XCUIApplication!

    @MainActor
    private func launch(_ scenario: String) {
        continueAfterFailure = false
        app = XCUIApplication()
        app.launchArguments = ["-MonacoProposeSample", scenario]
        app.launch()
    }

    @MainActor
    private func element(_ id: String) -> XCUIElement {
        app.descendants(matching: .any).matching(identifier: id).firstMatch
    }

    @MainActor
    func testStockEntry_reachesTheAmountStepWithThePot() throws {
        launch("amount")
        XCTAssertTrue(element("amount-entry-field").waitForExistence(timeout: 10), "the amount step never drew")
        let helper = element("amount-entry-helper")
        XCTAssertTrue(helper.waitForExistence(timeout: 10), "the pot helper never drew")
        XCTAssertTrue(helper.label.hasPrefix("The pot has"), "the helper should say what the pot has: \(helper.label)")
        XCTAssertTrue(element("propose-amount-pot-total").exists, "the pot total row is missing")
    }

    @MainActor
    func testStockEntry_potFails_amountStepStillReachesReview() throws {
        launch("potFailed")

        let amount = element("amount-entry-field")
        XCTAssertTrue(amount.waitForExistence(timeout: 10), "the amount step did not open when the pot failed")
        let total = element("propose-amount-pot-total")
        XCTAssertTrue(total.waitForExistence(timeout: 10), "the pot total row is missing")
        XCTAssertTrue(total.label.contains("Unavailable"), "the pot total should say Unavailable: \(total.label)")
        XCTAssertFalse(
            app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "The pot has")).firstMatch.exists,
            "no pot figure should be claimed when the read failed")

        let preset = app.buttons["$50"]
        XCTAssertTrue(preset.waitForExistence(timeout: 5), "the presets never drew")
        preset.tap()
        let review = element("propose-amount-review")
        let enabled = expectation(for: NSPredicate(format: "isEnabled == true"), evaluatedWith: review)
        XCTAssertEqual(XCTWaiter().wait(for: [enabled], timeout: 10), .completed, "Review stayed disabled")
    }
}
