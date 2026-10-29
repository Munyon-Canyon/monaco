import XCTest

nonisolated final class ProposalFeedSampleUITests: XCTestCase {
    @MainActor private var app: XCUIApplication!

    @MainActor
    private func launch(_ scenario: String, arguments: [String] = []) {
        continueAfterFailure = false
        app = XCUIApplication()
        app.launchArguments = ["-MonacoProposalSample", scenario] + arguments
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
    private func scrollAboveComposer(_ element: XCUIElement) {
        app.scroll(element, above: self.element("comment-composer-field"))
    }

    @MainActor
    private func openProposal(from card: XCUIElement) {
        card.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 1)).withOffset(CGVector(dx: 0, dy: -22)).tap()
    }

    @MainActor
    func testNeedsVote_reviewFromTheCard_voteInTheContent_thenCommentAndReplyInThread() throws {
        launch("needsVote")
        XCTAssertTrue(app.navigationBars["All proposals"].waitForExistence(timeout: 10), "the list never drew")
        let first = element("proposal-card-proposal-1")
        XCTAssertTrue(first.waitForExistence(timeout: 10), "the first vote card never drew")
        XCTAssertTrue(first.label.contains("Needs your vote"), "the card should say it needs a vote: \(first.label)")
        XCTAssertTrue(first.label.contains("Buy Alphabet"), "the card should say which way the trade goes")
        XCTAssertFalse(app.buttons["Yes"].exists, "a card offers no inline Yes")
        XCTAssertFalse(app.buttons["No"].exists, "a card offers no inline No")
        capture("01-all-proposals")

        openProposal(from: first)
        let yes = app.buttons["Vote Yes"]
        XCTAssertTrue(yes.waitForExistence(timeout: 10), "the proposal never offered Vote Yes")
        XCTAssertTrue(app.buttons["Vote No"].exists, "the proposal should offer Vote No")
        XCTAssertTrue(element("proposal-reason").exists, "the thesis block is missing")
        capture("02-proposal-vote")

        yes.tap()
        XCTAssertTrue(app.staticTexts["Vote recorded."].waitForExistence(timeout: 5), "no toast after voting")
        XCTAssertTrue(
            element("proposal-change-vote").waitForExistence(timeout: 5), "the vote section should offer Change vote")
        XCTAssertFalse(app.buttons["Vote Yes"].exists, "Vote Yes should be gone once voted")
        capture("03-after-vote")

        element("proposal-change-vote").tap()
        let no = app.buttons["Vote No"]
        XCTAssertTrue(no.waitForExistence(timeout: 5), "Change vote never offered Vote No")
        no.tap()
        XCTAssertTrue(app.staticTexts["Vote recorded."].waitForExistence(timeout: 5), "no toast after changing")

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
        XCTAssertTrue(app.staticTexts["Reply to Maya"].waitForExistence(timeout: 5), "no reply sheet shown")
        let sheetField = element("comment-composer-field")
        sheetField.tap()
        sheetField.typeText("Agreed, Nvidia next.")
        element("comment-composer-send").tap()
        XCTAssertTrue(
            app.staticTexts["Agreed, Nvidia next."].waitForExistence(timeout: 5),
            "the posted reply never reached the thread")
        XCTAssertTrue(
            app.staticTexts["Reply to Maya"].waitForNonExistence(timeout: 5),
            "the reply sheet should close after posting")
        XCTAssertTrue(
            waitUntilHittable(app.staticTexts["Agreed, Nvidia next."]),
            "the posted reply is off screen or behind the composer"
        )
        capture("06-reply-posted")
    }

    @MainActor
    func testOpen_voteSitsInTheContent_andTheBottomHoldsOnlyTheComposer() throws {
        launch("open")
        let yes = app.buttons["Vote Yes"]
        let no = app.buttons["Vote No"]
        XCTAssertTrue(yes.waitForExistence(timeout: 10), "Vote Yes never drew")
        XCTAssertTrue(no.exists, "Vote No never drew")
        let before = yes.frame.minY
        XCTAssertEqual(yes.frame.width, no.frame.width, accuracy: 1, "Yes and No should share the row equally")
        XCTAssertEqual(yes.frame.minY, no.frame.minY, accuracy: 1, "Yes and No should sit side by side")
        XCTAssertTrue(element("proposal-vote-progress").exists, "votes so far and time left are missing")
        let field = element("comment-composer-field")
        XCTAssertTrue(field.waitForExistence(timeout: 10), "the composer never drew")
        XCTAssertLessThan(yes.frame.maxY, field.frame.minY, "the vote should sit above the composer")
        scrollAboveComposer(yes)
        capture("30-vote-open")

        scrollIntoReach(element("comment-row-c1"))
        XCTAssertLessThan(yes.frame.minY, before, "the vote should scroll with the content, not float")
        XCTAssertTrue(field.isHittable, "the composer should stay reachable at the bottom")
    }

    @MainActor
    func testOpen_atTheLargestTextSize_stacksYesOverNo() throws {
        launch("open", arguments: ["-UIPreferredContentSizeCategoryName", "UICTContentSizeCategoryAccessibilityXXXL"])
        let yes = app.buttons["Vote Yes"]
        let no = app.buttons["Vote No"]
        XCTAssertTrue(yes.waitForExistence(timeout: 10), "Vote Yes never drew")
        scrollAboveComposer(no)
        XCTAssertLessThan(
            no.frame.maxY, element("comment-composer-field").frame.minY, "Vote No is under the composer")
        XCTAssertLessThan(yes.frame.maxY, no.frame.minY, "Yes and No should stack at accessibility sizes")
        XCTAssertEqual(yes.frame.width, no.frame.width, accuracy: 1, "stacked buttons should match widths")
        capture("34-vote-largest-text")
    }

    @MainActor
    func testVoted_showsYouVotedAndChangeVote() throws {
        launch("voted")
        let voted = element("proposal-voted")
        XCTAssertTrue(voted.waitForExistence(timeout: 10), "the voted line never drew")
        XCTAssertEqual(voted.label, "You voted Yes", "the voted line should name the ballot")
        XCTAssertTrue(app.buttons["Change vote"].exists, "Change vote is missing")
        XCTAssertFalse(app.buttons["Vote Yes"].exists, "Vote Yes should be gone once voted")
        XCTAssertFalse(app.buttons["Vote No"].exists, "Vote No should be gone once voted")
        scrollAboveComposer(element("proposal-change-vote"))
        capture("31-vote-voted")
        element("proposal-change-vote").tap()
        sleep(1)
        capture("32-vote-change")
        XCTAssertTrue(app.buttons["Vote No"].waitForExistence(timeout: 5), "Change vote never offered Vote No")
    }

    @MainActor
    func testClosed_showsTheResultInPlaceOfButtons() throws {
        launch("rejected")
        let result = element("proposal-vote-result")
        XCTAssertTrue(result.waitForExistence(timeout: 10), "the result never drew")
        XCTAssertTrue(result.label.contains("did not pass"), "the result should say the proposal did not pass")
        XCTAssertFalse(app.buttons["Vote Yes"].exists, "a closed proposal must not offer Vote Yes")
        XCTAssertFalse(app.buttons["Vote No"].exists, "a closed proposal must not offer Vote No")
        XCTAssertFalse(element("proposal-change-vote").exists, "a closed proposal must not offer Change vote")
        XCTAssertTrue(element("comment-composer-field").exists, "the composer should stay")
        capture("33-vote-closed")
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
    func testReadOnlyProposal_showsTallyAndTheViewOnlyLineWithoutVoteButtons() throws {
        launch("readOnly")
        XCTAssertTrue(element("proposal-votes").waitForExistence(timeout: 10), "the votes block never drew")
        let bar = app.descendants(matching: .any)
            .matching(NSPredicate(format: "label CONTAINS %@", "1 yes, 0 no, 2 not voted")).firstMatch
        XCTAssertTrue(bar.exists, "the vote bar is missing or reads the wrong tally")
        XCTAssertTrue(element("proposal-view-only").exists, "the view-only caption is missing")
        XCTAssertFalse(app.buttons["Vote Yes"].exists, "a read-only proposal must not offer Vote Yes")
        XCTAssertFalse(app.buttons["Vote No"].exists, "a read-only proposal must not offer Vote No")
        XCTAssertFalse(element("proposal-change-vote").exists, "a read-only proposal must not offer Change vote")
    }

    @MainActor
    func testSeeAll_opensTheVotesSheetWithEveryVoter() throws {
        launch("open")
        let seeAll = element("proposal-votes-see-all")
        XCTAssertTrue(seeAll.waitForExistence(timeout: 10), "See all never drew")
        seeAll.tap()
        XCTAssertTrue(element("proposal-voters").waitForExistence(timeout: 5), "the votes sheet never opened")
        let voters = app.descendants(matching: .any).matching(
            NSPredicate(format: "identifier BEGINSWITH 'proposal-voter-'"))
        XCTAssertEqual(voters.count, 3, "the sheet should list the three voters")
        XCTAssertTrue(
            app.descendants(matching: .any).matching(NSPredicate(format: "label == %@", "Kai voted yes")).firstMatch
                .exists,
            "Kai's yes is missing")
        capture("07-votes-sheet")
        app.buttons["Close"].tap()
        XCTAssertTrue(app.buttons["Vote Yes"].waitForExistence(timeout: 5), "Close should return to the proposal")
    }

    @MainActor
    func testAllProposals_switchesBetweenOpenAndClosed() throws {
        launch("allOpen")
        XCTAssertTrue(element("proposal-card-proposal-1").waitForExistence(timeout: 10), "the open card never drew")
        element("proposals-segment-closed").tap()
        XCTAssertTrue(
            app.staticTexts["No closed proposals yet."].waitForExistence(timeout: 5),
            "Closed should say there are none yet")
        XCTAssertFalse(element("proposal-card-proposal-1").exists, "Closed should not list the open card")
        element("proposals-segment-open").tap()
        XCTAssertTrue(element("proposal-card-proposal-1").waitForExistence(timeout: 5), "Open lost its card")
    }

    @MainActor
    func testAllProposals_closedRowsCarryTheirStatusBadge() throws {
        launch("allClosed")
        let pending = element("proposal-row-closed-1")
        XCTAssertTrue(pending.waitForExistence(timeout: 10), "the first closed row never drew")
        XCTAssertTrue(pending.label.contains("Pending"), "the approved row should read Pending: \(pending.label)")
        XCTAssertTrue(element("proposal-row-closed-2").label.contains("Expired"), "the expired row is missing")
        XCTAssertTrue(element("proposal-row-closed-3").label.contains("Blocked"), "the blocked row is missing")
    }

    @MainActor
    func testAllProposals_emptyOffersTheCabalsTab() throws {
        launch("allEmpty")
        XCTAssertTrue(app.staticTexts["No open proposals"].waitForExistence(timeout: 10), "the empty state never drew")
        app.buttons["Choose a proposal"].tap()
        XCTAssertTrue(
            app.navigationBars["All proposals"].waitForNonExistence(timeout: 5),
            "Choose a proposal should leave the list")
    }
}

extension XCUIApplication {
    private static let composerBarPadding: CGFloat = 24

    @MainActor
    fileprivate func scroll(_ element: XCUIElement, above composer: XCUIElement, maxDrags: Int = 6) {
        var drags = 0
        while element.frame.maxY > composer.frame.minY - Self.composerBarPadding && drags < maxDrags {
            let from = windows.firstMatch.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.6))
            let to = windows.firstMatch.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.45))
            from.press(forDuration: 0.1, thenDragTo: to)
            drags += 1
        }
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
        launch("chooserPushed")
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

        element("amount-entry-field").typeText("00")
        XCTAssertTrue(
            app.staticTexts["Amount exceeds the cash balance."].waitForExistence(timeout: 5),
            "$5,000 should read as more than the pot has")
        capture("13-amount-over")
        preset.tap()

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
        XCTAssertEqual(send.label, "Send to vote", "the send button should say what it does")
        XCTAssertTrue(app.staticTexts["Earnings Thursday."].exists, "the thesis should repeat on the receipt")
        sleep(1)
        capture("14-review")
        send.tap()

        let sent = app.staticTexts["Proposal sent"]
        XCTAssertTrue(sent.waitForExistence(timeout: 5), "no outcome after sending")
        capture("15-sent")
    }

    @MainActor
    func testFlow_keepsSheetFullHeightWhenDragged() throws {
        launch("amount")
        let addReason = element("propose-amount-reason")
        XCTAssertTrue(addReason.waitForExistence(timeout: 10), "the amount step never drew")
        sleep(1)

        let title = app.navigationBars["Buy GOOGL"]
        XCTAssertTrue(title.exists, "the Buy GOOGL bar never drew")
        let before = title.frame.minY
        title.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5))
            .press(
                forDuration: 0.1,
                thenDragTo: app.windows.firstMatch.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)))
        sleep(1)
        capture("16-flow-after-drag")
        XCTAssertTrue(title.exists, "the screen closed")
        XCTAssertEqual(title.frame.minY, before, accuracy: 2, "the screen dropped from full height")
        XCTAssertTrue(addReason.isHittable, "the thesis field slid out of reach")
        XCTAssertTrue(element("propose-amount-review").isHittable, "Review slid out of reach")
    }

    @MainActor
    func testSell_dollarsToShares_reviewShowsEstimate() throws {
        launch("chooserPushed")
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
        let sent = app.staticTexts["Proposal sent"]
        XCTAssertTrue(sent.waitForExistence(timeout: 5), "no outcome after sending")
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
    func testStockEntry_reachesTheAmountStepWithTheCash() throws {
        launch("amount")
        XCTAssertTrue(element("amount-entry-field").waitForExistence(timeout: 10), "the amount step never drew")
        let helper = element("amount-entry-helper")
        XCTAssertTrue(helper.waitForExistence(timeout: 10), "the cash helper never drew")
        XCTAssertTrue(
            helper.label.hasPrefix("The cabal can spend"),
            "the helper should say what the cabal can spend: \(helper.label)")
        XCTAssertTrue(element("propose-amount-cash").exists, "the cash row is missing")
    }

    @MainActor
    func testStockEntry_potFails_amountStepStillReachesReview() throws {
        launch("potFailed")

        let amount = element("amount-entry-field")
        XCTAssertTrue(amount.waitForExistence(timeout: 10), "the amount step did not open when the pot failed")
        let total = element("propose-amount-cash")
        XCTAssertTrue(total.waitForExistence(timeout: 10), "the cash row is missing")
        XCTAssertTrue(total.label.contains("Unavailable"), "the cash row should say Unavailable: \(total.label)")
        XCTAssertFalse(
            app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "The cabal can spend")).firstMatch
                .exists,
            "no cash figure should be claimed when the read failed")

        let preset = app.buttons["$50"]
        XCTAssertTrue(preset.waitForExistence(timeout: 5), "the presets never drew")
        preset.tap()
        let review = element("propose-amount-review")
        let enabled = expectation(for: NSPredicate(format: "isEnabled == true"), evaluatedWith: review)
        XCTAssertEqual(XCTWaiter().wait(for: [enabled], timeout: 10), .completed, "Review stayed disabled")
    }
}
