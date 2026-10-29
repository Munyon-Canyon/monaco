import XCTest

enum FeedReactAndReplyJourney {
    static let id = "feed/react-and-reply"
    static let version = 2

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static let host = "QA host"
    static let rowPrefix = "comment-row-"

    static func root(run: String) -> String { "QA root \(run)" }
    static func reply(run: String) -> String { "QA feed reply \(run)" }
    static func publicCabal(run: String) -> String { "QA public \(run)" }
    static func publicReply(run: String) -> String { "QA public reply \(run)" }

    static func row(_ app: XCUIApplication, reading body: String) -> XCUIElement {
        app.descendants(matching: .any)
            .matching(NSPredicate(format: "identifier BEGINSWITH %@ AND label CONTAINS %@", rowPrefix, body))
            .firstMatch
    }

    static func rowID(_ row: XCUIElement) -> String { String(row.identifier.dropFirst(rowPrefix.count)) }

    static func count(_ app: XCUIApplication, prefix: String) -> Int {
        app.descendants(matching: .any).matching(NSPredicate(format: "identifier BEGINSWITH %@", prefix)).count
    }

    static func waitForLabel(_ element: XCUIElement, containing text: String, timeout: TimeInterval) -> Bool {
        let predicate = NSPredicate(format: "label CONTAINS %@", text)
        let expectation = XCTNSPredicateExpectation(predicate: predicate, object: element)
        return XCTWaiter().wait(for: [expectation], timeout: timeout) == .completed
    }

    static func waitGone(_ element: XCUIElement, timeout: TimeInterval) -> Bool {
        element.waitForNonExistence(timeout: timeout)
    }

    static func waitForRepliesHeader(_ app: XCUIApplication, count: Int, timeout: TimeInterval) -> Bool {
        let title = app.staticTexts["Replies"]
        let number = app.staticTexts.matching(NSPredicate(format: "label == %@", "\(count)")).firstMatch
        return title.waitForExistence(timeout: timeout) && number.waitForExistence(timeout: timeout)
    }

    static func chooseEveryone(_ app: XCUIApplication) {
        app.element("feed-filter").tap()
        let everyone = app.buttons
            .matching(NSPredicate(format: "identifier == 'feed-scope-Everyone' OR label == 'Everyone'"))
            .firstMatch
        XCTAssertTrue(everyone.waitForExistence(timeout: 5), "no Everyone scope in the filter menu")
        everyone.tap()
        if everyone.exists {
            app.coordinate(withNormalizedOffset: CGVector(dx: 0.08, dy: 0.6)).tap()
        }
    }

    static func addReaction(_ app: XCUIApplication, to key: String, search: String, hex: String, step: String) {
        let add = app.element("reaction-add-\(key)")
        app.scrollIntoReach(add)
        XCTAssertTrue(add.waitForExistence(timeout: 10), "\(step): no + on \(key)")
        add.tap()
        let field = app.element("emoji-search")
        if !field.waitForExistence(timeout: 3) {
            let more = app.element("reaction-tray-more-\(key)")
            XCTAssertTrue(more.waitForExistence(timeout: 5), "\(step): the + opened neither the picker nor the tray")
            more.tap()
        }
        XCTAssertTrue(field.waitForExistence(timeout: 5), "\(step): no emoji-search in the picker")
        field.tap()
        field.typeText(search)
        let cell = app.element("emoji-cell-\(hex)")
        XCTAssertTrue(cell.waitForExistence(timeout: 5), "\(step): no emoji-cell-\(hex) for '\(search)'")
        cell.tap()
    }

    static func openActions(_ app: XCUIApplication, on row: XCUIElement, step: String) -> String {
        let id = rowID(row)
        row.press(forDuration: 1.0)
        XCTAssertTrue(
            app.element("comment-actions-\(id)").waitForExistence(timeout: 5), "\(step): no actions menu on the reply")
        return id
    }

    static func reactFromTray(_ app: XCUIApplication, id: String, hex: String, text: String, step: String) {
        let tray = app.element("reaction-tray-\(hex)")
        XCTAssertTrue(tray.waitForExistence(timeout: 5), "\(step): no reaction-tray-\(hex)")
        tray.tap()
        let chip = app.element("reaction-chip-comment-\(id)-\(hex)")
        XCTAssertTrue(chip.waitForExistence(timeout: 10), "\(step): no chip after the tray pick within 10 s")
        XCTAssertTrue(waitForLabel(chip, containing: text, timeout: 10), "\(step): the chip does not read \(text)")
    }

    static func reactAndReplyFromCard(
        _ app: XCUIApplication, item: String, run: String, recorder: JourneyRecorder
    ) {
        let key = "feed-\(item)"
        let chip = app.element("reaction-chip-\(key)-1F525")

        recorder.step("S1.1", "open the Feed and read the sale's card") {
            app.popToRoot()
            FeedBrowseJourney.openFeed(app, step: "S1.1")
            let cell = app.element("feed-cell-\(item)")
            app.scrollIntoReach(cell)
            XCTAssertTrue(cell.waitForExistence(timeout: 15), "S1.1: no sale card within 15 s")
            XCTAssertTrue(app.element("reaction-add-\(key)").exists, "S1.1: no + on the card")
            let reply = app.element("feed-reply-\(item)")
            XCTAssertTrue(reply.exists, "S1.1: no reply control on the card")
            XCTAssertTrue(waitForLabel(reply, containing: "1", timeout: 10), "S1.1: the reply control does not read 1")
        }

        recorder.step("S1.2", "add, remove and add a fire reaction") {
            addReaction(app, to: key, search: "fire", hex: "1F525", step: "S1.2")
            XCTAssertTrue(chip.waitForExistence(timeout: 10), "S1.2: no 🔥 chip within 10 s")
            XCTAssertTrue(waitForLabel(chip, containing: "1", timeout: 10), "S1.2: the chip does not read 1")
            chip.tap()
            XCTAssertTrue(waitGone(chip, timeout: 10), "S1.2: the chip stayed after tapping it")
            addReaction(app, to: key, search: "fire", hex: "1F525", step: "S1.2")
            XCTAssertTrue(chip.waitForExistence(timeout: 10), "S1.2: no 🔥 chip after adding it again")
        }

        recorder.step("S1.3", "reply from the half sheet") {
            app.element("feed-reply-\(item)").tap()
            XCTAssertTrue(app.element("reply-sheet").waitForExistence(timeout: 5), "S1.3: no reply sheet")
            let quote = app.element("reply-sheet-quote")
            XCTAssertTrue(quote.waitForExistence(timeout: 5), "S1.3: no quoted post in the sheet")
            XCTAssertTrue(
                quote.label.contains(host), "S1.3: the quote does not name \(host); it reads '\(quote.label)'")
            let title = app.staticTexts
                .matching(NSPredicate(format: "label CONTAINS %@", "Reply to \(host)"))
                .firstMatch
            XCTAssertTrue(title.exists, "S1.3: the sheet does not read 'Reply to \(host)'")
            let field = app.element("comment-composer-field")
            XCTAssertTrue(field.waitForExistence(timeout: 5), "S1.3: no composer field")
            XCTAssertEqual(field.value(forKey: "hasKeyboardFocus") as? Bool, true, "S1.3: the field has no focus")
            XCTAssertTrue(app.element("comment-composer-gif").exists, "S1.3: no GIF control in the field")
            field.typeText(reply(run: run))
            app.element("comment-composer-send").tap()
            XCTAssertEqual(app.toastText(timeout: 10), "Replied", "S1.3: no 'Replied' toast within 10 s")
            XCTAssertTrue(waitGone(app.element("reply-sheet"), timeout: 10), "S1.3: the sheet stayed open")
            XCTAssertTrue(
                waitForLabel(app.element("feed-reply-\(item)"), containing: "2", timeout: 10),
                "S1.3: the reply control does not read 2")
        }
    }

    static func postPage(_ app: XCUIApplication, item: String, run: String, recorder: JourneyRecorder) {
        recorder.step("S2.1", "open the post page") {
            let cell = app.element("feed-cell-\(item)")
            XCTAssertTrue(cell.waitForExistence(timeout: 10), "S2.1: no sale card")
            cell.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.1)).tap()
            XCTAssertTrue(app.element("feed-item-detail").waitForExistence(timeout: 15), "S2.1: no post page")
            XCTAssertTrue(app.navigationBars["Post"].waitForExistence(timeout: 5), "S2.1: the title is not 'Post'")
            XCTAssertTrue(app.element("reaction-add-feed-\(item)").exists, "S2.1: no reaction row on the post")
            XCTAssertTrue(waitForRepliesHeader(app, count: 2, timeout: 10), "S2.1: no 'Replies 2'")
            XCTAssertTrue(row(app, reading: root(run: run)).waitForExistence(timeout: 10), "S2.1: no root reply")
            XCTAssertTrue(row(app, reading: reply(run: run)).waitForExistence(timeout: 10), "S2.1: no reply by A")
        }

        recorder.step("S2.2", "scroll to the docked composer") {
            let field = app.element("comment-composer-field")
            app.scrollIntoReach(field)
            XCTAssertTrue(field.waitForExistence(timeout: 10), "S2.2: no docked composer")
            XCTAssertEqual(field.placeholderValue, "Say something", "S2.2: the composer placeholder is not the spec's")
            XCTAssertTrue(app.element("comment-composer-gif").exists, "S2.2: no GIF control in the composer")
            let last = row(app, reading: reply(run: run))
            XCTAssertLessThanOrEqual(
                last.frame.maxY, field.frame.minY + 1, "S2.2: the last reply is not above the composer"
            )
        }

        recorder.step("S2.3", "reply to the root with a GIF") {
            let rootRow = row(app, reading: root(run: run))
            app.scrollIntoReach(rootRow)
            let rootID = rowID(rootRow)
            app.element("comment-reply-\(rootID)").tap()
            XCTAssertTrue(app.element("reply-sheet").waitForExistence(timeout: 5), "S2.3: no reply sheet")
            let quote = app.element("reply-sheet-quote")
            XCTAssertTrue(quote.waitForExistence(timeout: 5), "S2.3: no quote in the sheet")
            XCTAssertTrue(quote.label.contains(root(run: run)), "S2.3: the quote does not show the root")
            app.element("comment-composer-gif").tap()
            XCTAssertTrue(
                app.element("gif-sheet").waitForExistence(timeout: 10), "S2.3: no GIF sheet (is the build keyed?)"
            )
            XCTAssertTrue(app.element("gif-attribution").exists, "S2.3: no GIPHY attribution")
            let search = app.element("gif-search")
            search.tap()
            search.typeText("rocket")
            let cell = app.descendants(matching: .any)
                .matching(NSPredicate(format: "identifier BEGINSWITH 'gif-cell-'"))
                .firstMatch
            XCTAssertTrue(cell.waitForExistence(timeout: 15), "S2.3: no GIF result for 'rocket' within 15 s")
            cell.tap()
            let bubble = app.descendants(matching: .any)
                .matching(NSPredicate(format: "identifier BEGINSWITH 'gif-bubble-'")).firstMatch
            XCTAssertTrue(bubble.waitForExistence(timeout: 15), "S2.3: no GIF bubble within 15 s")
            XCTAssertTrue(waitGone(app.element("gif-sheet"), timeout: 5), "S2.3: the GIF sheet stayed open")
            XCTAssertTrue(waitGone(app.element("reply-sheet"), timeout: 5), "S2.3: the reply sheet stayed open")
            let commentID = String(bubble.identifier.dropFirst("gif-bubble-".count))
            let quoted = app.element("comment-quote-\(commentID)")
            XCTAssertTrue(quoted.waitForExistence(timeout: 5), "S2.3: the GIF reply has no quote")
            XCTAssertTrue(quoted.label.contains(host), "S2.3: the quote does not name \(host)")
        }

        commentActions(app, run: run, recorder: recorder)
    }
}

extension FeedReactAndReplyJourney {
    static func commentActions(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S2.4", "long-press the root and react from the tray") {
            let rootRow = row(app, reading: root(run: run))
            app.scrollIntoReach(rootRow)
            let id = openActions(app, on: rootRow, step: "S2.4")
            for action in ["reply", "copy", "report"] {
                XCTAssertTrue(app.element("comment-action-\(action)").exists, "S2.4: no \(action) action")
            }
            XCTAssertFalse(app.element("comment-action-delete").exists, "S2.4: a Delete on someone else's reply")
            reactFromTray(app, id: id, hex: "1F389", text: "1", step: "S2.4")
        }

        recorder.step("S2.5", "copy the root") {
            let rootRow = row(app, reading: root(run: run))
            _ = openActions(app, on: rootRow, step: "S2.5")
            app.element("comment-action-copy").tap()
            XCTAssertEqual(app.toastText(timeout: 10), "Copied", "S2.5: no 'Copied' toast")
        }

        recorder.step("S2.6", "delete A's own text reply") {
            app.waitForToastGone()
            let own = row(app, reading: reply(run: run))
            app.scrollIntoReach(own)
            _ = openActions(app, on: own, step: "S2.6")
            XCTAssertTrue(app.element("comment-action-delete").exists, "S2.6: no Delete on A's own reply")
            XCTAssertFalse(app.element("comment-action-report").exists, "S2.6: a Report on A's own reply")
            app.element("comment-action-delete").tap()
            let confirm = app.confirmDialogButton("Delete")
            XCTAssertTrue(confirm.waitForExistence(timeout: 5), "S2.6: no delete confirmation")
            confirm.tap()
            XCTAssertTrue(
                waitGone(row(app, reading: reply(run: run)), timeout: 10), "S2.6: the reply stayed within 10 s"
            )
            XCTAssertTrue(
                waitForRepliesHeader(app, count: 2, timeout: 10), "S2.6: the header is not 'Replies 2'"
            )
        }
    }

    static func publicPost(_ app: XCUIApplication, item: String, run: String, recorder: JourneyRecorder) {
        recorder.step("S3.1", "open the public proposal") {
            app.popToRoot()
            FeedBrowseJourney.openFeed(app, step: "S3.1")
            chooseEveryone(app)
            let cell = app.descendants(matching: .any)
                .matching(NSPredicate(format: "identifier == %@", "feed-cell-\(item)")).firstMatch
            app.scrollIntoReach(cell)
            XCTAssertTrue(cell.waitForExistence(timeout: 15), "S3.1: no public proposal card within 15 s")
            cell.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.1)).tap()
            let card = app.element("proposal-vote-readonly")
            XCTAssertTrue(card.waitForExistence(timeout: 15), "S3.1: no read-only vote card")
            XCTAssertTrue(card.label.contains("members vote"), "S3.1: no '<cabal> members vote' in the card label")
            for gone in ["proposal-vote-yes", "proposal-vote-no", "proposal-remind"] {
                XCTAssertFalse(app.element(gone).exists, "S3.1: \(gone) shows for a non-member")
            }
        }

        recorder.step("S3.2", "read the members-only line") {
            let line = app.element("comment-members-only")
            app.scrollIntoReach(line)
            XCTAssertTrue(line.waitForExistence(timeout: 10), "S3.2: no comment-members-only")
            XCTAssertTrue(
                waitForLabel(line, containing: "Members of \(publicCabal(run: run)) reply here", timeout: 5),
                "S3.2: the line does not name the cabal")
            XCTAssertFalse(app.element("comment-composer-field").exists, "S3.2: a composer shows for a non-member")
            XCTAssertEqual(count(app, prefix: "comment-reply-"), 0, "S3.2: a Reply shows for a non-member")
        }

        recorder.step("S3.3", "long-press the host's reply and react") {
            let theirs = row(app, reading: publicReply(run: run))
            app.scrollIntoReach(theirs)
            XCTAssertTrue(theirs.waitForExistence(timeout: 10), "S3.3: no host reply")
            let id = openActions(app, on: theirs, step: "S3.3")
            XCTAssertTrue(app.element("comment-action-copy").exists, "S3.3: no Copy")
            XCTAssertTrue(app.element("comment-action-report").exists, "S3.3: no Report")
            XCTAssertFalse(app.element("comment-action-reply").exists, "S3.3: a Reply for a non-member")
            reactFromTray(app, id: id, hex: "1F44D", text: "1", step: "S3.3")
        }
    }
}
