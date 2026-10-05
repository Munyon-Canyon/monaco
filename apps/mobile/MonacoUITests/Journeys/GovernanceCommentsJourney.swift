import XCTest

enum GovernanceCommentsJourney {
    static let id = "governance/comments"
    static let version = 1

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func comment(run: String) -> String { "QA comment \(run)" }
    static func ask(run: String) -> String { "QA ask \(run)" }
    static func answer(run: String) -> String { "QA answer \(run)" }

    static func row(_ app: XCUIApplication, reading body: String) -> XCUIElement {
        app.descendants(matching: .any)
            .matching(NSPredicate(format: "identifier BEGINSWITH 'comment-row-' AND label CONTAINS %@", body))
            .firstMatch
    }

    static func openProposal(_ app: XCUIApplication, step: String) {
        FeedBrowseJourney.openFeed(app, step: step)
        app.element("feed-chip-Proposals").tap()
        let cell = app.descendants(matching: .any)
            .matching(NSPredicate(format: "identifier BEGINSWITH 'feed-cell-'")).firstMatch
        XCTAssertTrue(cell.waitForExistence(timeout: 15), "\(step): no proposal cell within 15 s (known failure, #612)")
        cell.tap()
        XCTAssertTrue(
            app.element("proposal-detail").waitForExistence(timeout: 15),
            "\(step): the proposal did not open within 15 s (known failure, #612)"
        )
    }

    static func post(_ app: XCUIApplication, _ body: String, step: String) {
        let field = app.element("comment-composer-field")
        app.scrollIntoReach(field)
        XCTAssertTrue(field.waitForExistence(timeout: 10), "\(step): no comment composer (known failure, #711)")
        field.tap()
        field.typeText(body)
        app.element("comment-composer-send").tap()
        XCTAssertTrue(
            row(app, reading: body).waitForExistence(timeout: 10),
            "\(step): no comment reading \(body) within 10 s (known failure, #711)"
        )
    }

    static func readAndPost(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S1.1", "open the proposal") {
            openProposal(app, step: "S1.1")
        }

        recorder.step("S1.2", "read the empty thread") {
            let thread = app.element("comment-thread")
            app.scrollIntoReach(thread)
            XCTAssertTrue(thread.waitForExistence(timeout: 10), "S1.2: no comment thread (known failure, #711)")
            XCTAssertTrue(app.staticTexts["Comments"].exists, "S1.2: the thread is not headed 'Comments' (#711)")
            XCTAssertTrue(
                JoinJourney.waitForLabel(
                    app.element("comment-thread-empty"), containing: "No comments yet", timeout: 10),
                "S1.2: no 'No comments yet' within 10 s (known failure, #711)"
            )
        }

        recorder.step("S1.3", "post a comment") {
            let field = app.element("comment-composer-field")
            XCTAssertTrue(field.waitForExistence(timeout: 10), "S1.3: no comment composer (known failure, #711)")
            XCTAssertEqual(field.placeholderValue, "Add a comment", "S1.3: the composer placeholder is not the spec's")
            post(app, comment(run: run), step: "S1.3")
        }
    }

    static func reply(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S2.1", "open the proposal") {
            openProposal(app, step: "S2.1")
        }

        recorder.step("S2.2", "post the comment to answer") {
            post(app, ask(run: run), step: "S2.2")
        }

        recorder.step("S2.3", "tap Reply on it") {
            let asked = row(app, reading: ask(run: run))
            let reply = asked.descendants(matching: .any)
                .matching(NSPredicate(format: "identifier BEGINSWITH 'comment-reply-'")).firstMatch
            XCTAssertTrue(reply.waitForExistence(timeout: 5), "S2.3: no Reply on the comment (known failure, #711)")
            reply.tap()
            XCTAssertTrue(
                app.element("comment-composer-cancel-reply").waitForExistence(timeout: 5),
                "S2.3: the composer is not replying (known failure, #711)"
            )
        }

        recorder.step("S2.4", "post the reply") {
            post(app, answer(run: run), step: "S2.4")
            XCTAssertLessThan(
                row(app, reading: ask(run: run)).frame.minY, row(app, reading: answer(run: run)).frame.minY,
                "S2.4: the reply is not below the comment it answers"
            )
        }
    }
}
