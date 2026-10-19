import UIKit
import XCTest

enum JoinJourney {
    static let id = "cabals/join"
    static let version = 8

    static let screenTimeout: TimeInterval = 15

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func requestCabal(run: String) -> String { "QA pot \(run)" }
    static func openCabal(run: String) -> String { "QA open \(run)" }
    static func gateCabal(run: String) -> String { "QA gate \(run)" }
    static let requestedToast = "Request sent. You'll be in once the creator says yes."

    static func waitForToast(_ app: XCUIApplication, _ message: String, step: String) {
        XCTAssertTrue(
            app.staticTexts[message].firstMatch.waitForExistence(timeout: 10),
            "\(step): the toast \"\(message)\" did not show within 10 s"
        )
    }

    static func waitForLabel(
        _ element: XCUIElement, containing text: String, timeout: TimeInterval
    ) -> Bool {
        let deadline = Date().addingTimeInterval(timeout)
        repeat {
            if element.exists, element.label.contains(text) { return true }
            RunLoop.current.run(until: Date().addingTimeInterval(0.1))
        } while Date() < deadline
        return element.exists && element.label.contains(text)
    }

    static func snap(_ app: XCUIApplication, _ name: String) {
        XCTContext.runActivity(named: name) { activity in
            let attachment = XCTAttachment(screenshot: app.screenshot())
            attachment.name = name
            attachment.lifetime = .keepAlways
            activity.add(attachment)
        }
    }

    static func searchField(_ app: XCUIApplication) -> XCUIElement {
        app.textFields.matching(
            NSPredicate(format: "identifier == 'cabals-search-field' OR label == 'Search cabals'")
        ).firstMatch
    }

    static func openDiscover(_ app: XCUIApplication, step: String) {
        if searchField(app).waitForExistence(timeout: 2) { return }
        let filter = app.textFields["cabals-filter-field"]
        XCTAssertTrue(filter.waitForExistence(timeout: 10), "\(step): no filter field or Discover on the Cabals tab")
        let held = filter.value as? String ?? ""
        if held.isEmpty || held == filter.placeholderValue {
            filter.tap()
            filter.typeText("zzqq nobody")
        }
        let link = app.buttons["cabals-search-public"]
        XCTAssertTrue(link.waitForExistence(timeout: 10), "\(step): no Search public cabals link within 10 s")
        link.tap()
        XCTAssertTrue(searchField(app).waitForExistence(timeout: 10), "\(step): Discover cabals did not open")
    }

    static func search(_ app: XCUIApplication, for query: String, step: String) {
        openDiscover(app, step: step)
        let field = searchField(app)
        XCTAssertTrue(field.waitForExistence(timeout: 10), "\(step): no search field on Discover cabals")
        app.waitForToastGone()
        if let text = field.value as? String, !text.isEmpty, text != field.placeholderValue {
            field.coordinate(withNormalizedOffset: CGVector(dx: 0.97, dy: 0.5)).tap()
            field.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: text.count))
        }
        let current = field.value as? String ?? ""
        XCTAssertTrue(
            current.isEmpty || current == field.placeholderValue, "\(step): search field still holds \(current)")
        if field.value(forKey: "hasKeyboardFocus") as? Bool != true {
            field.tap()
        }
        field.typeText(query)
    }

    static func result(_ app: XCUIApplication, named name: String) -> XCUIElementQuery {
        app.buttons.matching(
            NSPredicate(format: "identifier BEGINSWITH 'cabals-search-result-' AND label CONTAINS %@", name))
    }

    static func openBySearch(_ app: XCUIApplication, _ name: String, step: String) {
        app.tab("Cabals").tap()
        search(app, for: name, step: step)
        let row = result(app, named: name).firstMatch
        XCTAssertTrue(row.waitForExistence(timeout: 10), "\(step): no search result named \(name) within 10 s")
        row.tap()
        XCTAssertTrue(
            waitForLabel(app.element("cabal-header-name"), containing: name, timeout: screenTimeout),
            "\(step): the cabal screen for \(name) did not show within \(Int(screenTimeout)) s"
        )
    }

    static func creatorOpensCabal(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        let name = requestCabal(run: run)

        recorder.step("S1.1", "find the request cabal by name") {
            app.tab("Cabals").tap()
            search(app, for: name, step: "S1.1")
            XCTAssertTrue(
                result(app, named: name).firstMatch.waitForExistence(timeout: 10),
                "S1.1: no search result named \(name) within 10 s"
            )
            XCTAssertEqual(result(app, named: name).count, 1, "S1.1: \(name) is in the results more than once")
        }

        recorder.step("S1.2", "open it as its creator") {
            result(app, named: name).firstMatch.tap()
            XCTAssertTrue(
                waitForLabel(app.element("cabal-header-name"), containing: name, timeout: screenTimeout),
                "S1.2: the cabal screen for \(name) did not show within \(Int(screenTimeout)) s"
            )
            XCTAssertTrue(
                waitForLabel(app.element("cabal-member-count"), containing: "1 member", timeout: screenTimeout),
                "S1.2: the hero does not read 1 member: \(app.element("cabal-member-count").label)"
            )
        }

        recorder.step("S1.3", "see no pending requests") {
            XCTAssertFalse(
                app.element("cabal-join-requests-heading").exists,
                "S1.3: a new request cabal shows a pending-request block"
            )
        }
    }

    static func memberRequests(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        let name = requestCabal(run: run)

        recorder.step("S1.4", "search for a cabal no one has named") {
            app.tab("Cabals").tap()
            search(app, for: "zzqq", step: "S1.4")
            let empty = app.element("cabals-search-empty")
            XCTAssertTrue(empty.waitForExistence(timeout: 10), "S1.4: no empty search state within 10 s")
            XCTAssertTrue(
                app.staticTexts["No matching cabals"].exists,
                "S1.4: the empty state does not read No matching cabals"
            )
        }

        recorder.step("S1.5", "find the request cabal and open it") {
            openBySearch(app, name, step: "S1.5")
            XCTAssertTrue(
                app.buttons["cabal-join-button"].waitForExistence(timeout: screenTimeout),
                "S1.5: no Request to join on the cabal within \(Int(screenTimeout)) s"
            )
        }

        recorder.step("S1.6", "request to join") {
            app.buttons["cabal-join-button"].tap()
            waitForToast(app, requestedToast, step: "S1.6")
            XCTAssertTrue(
                waitForLabel(
                    app.element("cabal-join-requested"), containing: "Request pending", timeout: screenTimeout),
                "S1.6: no Request pending on the cabal screen"
            )
            XCTAssertTrue(app.buttons["cabal-join-cancel"].exists, "S1.6: no Cancel request")
            snap(app, "S1-B-request-sent")
        }
    }

    static func creatorApproves(_ app: XCUIApplication, run: String, member: String, recorder: JourneyRecorder) {
        let name = requestCabal(run: run)

        recorder.step("S1.7", "open the cabal and see the request") {
            openBySearch(app, name, step: "S1.7")
            XCTAssertTrue(
                waitForLabel(
                    app.element("cabal-join-requests-heading"), containing: "1 request to review",
                    timeout: screenTimeout),
                "S1.7: no 1 request to review on the cabal screen"
            )
            openRequests(app, step: "S1.7")
            let rows = app.descendants(matching: .any).matching(identifier: "cabal-join-request-row")
            XCTAssertEqual(rows.count, 1, "S1.7: expected one request row")
            XCTAssertTrue(rows.firstMatch.label.contains(member), "S1.7: the request does not name \(member)")
            snap(app, "S1-A-request-pending")
        }

        recorder.step("S1.8", "approve it") {
            decide(app, approve: true, step: "S1.8")
            waitForToast(app, "Approved.", step: "S1.8")
            XCTAssertTrue(
                app.element("members-requests-empty").waitForExistence(timeout: 10),
                "S1.8: the request is still on the Members screen"
            )
            app.navigationBars.buttons.firstMatch.tap()
            XCTAssertTrue(
                waitForLabel(app.element("cabal-member-count"), containing: "2 members", timeout: 10),
                "S1.8: the hero does not read 2 members: \(app.element("cabal-member-count").label)"
            )
        }
    }

    static func memberEntersAndJoinsOpen(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S1.9", "open the request cabal as a member") {
            openBySearch(app, requestCabal(run: run), step: "S1.9")
            XCTAssertTrue(
                app.element("cabal-action-fund").waitForExistence(timeout: screenTimeout),
                "S1.9: no member actions on the cabal within \(Int(screenTimeout)) s"
            )
            XCTAssertFalse(app.element("cabal-join-requested").exists, "S1.9: still reads Request pending")
            XCTAssertFalse(app.buttons["cabal-join-button"].exists, "S1.9: still offers to join")
            snap(app, "S1-B-member")
        }
    }

}

extension JoinJourney {
    static func openRequests(_ app: XCUIApplication, step: String) {
        let heading = app.element("cabal-join-requests-heading")
        XCTAssertTrue(
            heading.waitForExistence(timeout: screenTimeout),
            "\(step): no join requests row within \(Int(screenTimeout)) s")
        heading.tap()
        XCTAssertTrue(
            app.element("cabal-join-request-row").waitForExistence(timeout: 10),
            "\(step): the Members screen shows no request within 10 s")
    }

    static func decide(_ app: XCUIApplication, approve: Bool, step: String) {
        let review = app.buttons["cabal-join-review"].firstMatch
        XCTAssertTrue(review.waitForExistence(timeout: 10), "\(step): no Review button within 10 s")
        review.tap()
        let choice = app.buttons[approve ? "cabal-join-approve" : "cabal-join-deny"]
        XCTAssertTrue(
            choice.waitForExistence(timeout: 5), "\(step): the review dialog has no \(approve ? "Approve" : "Deny")")
        choice.tap()
    }

    static func creatorOpensTheCabal(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        let name = openCabal(run: run)

        recorder.step("S2.1", "change the join policy to Open") {
            openBySearch(app, name, step: "S2.1")
            app.buttons["cabal-details-button"].tap()
            app.scrollIntoReach(app.element("cabal-rules"))
            CabalsEditRulesJourney.assertRule(app, "join", reads: "Approval required", step: "S2.1")
            let edit = app.element("cabal-rules-edit")
            app.scrollIntoReach(edit)
            edit.tap()
            let join = app.element("edit-rule-join")
            XCTAssertTrue(join.waitForExistence(timeout: 10), "S2.1: Cabal settings did not show within 10 s")
            join.buttons["Open"].tap()
            XCTAssertTrue(app.buttons["edit-cabal-save"].isEnabled, "S2.1: Save is not enabled after the change")
            app.buttons["edit-cabal-save"].tap()
            waitForToast(app, "Cabal updated.", step: "S2.1")
            app.navigationBars.buttons.firstMatch.tap()
            CabalsEditRulesJourney.assertRule(app, "join", reads: "Open", step: "S2.1")
            app.navigationBars.buttons.firstMatch.tap()
        }
    }

    static func memberJoinsTheOpenCabal(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        let name = openCabal(run: run)

        recorder.step("S2.2", "find the open cabal and read its policy") {
            app.tab("Cabals").tap()
            search(app, for: name, step: "S2.2")
            let row = result(app, named: name).firstMatch
            XCTAssertTrue(row.waitForExistence(timeout: 10), "S2.2: no search result named \(name) within 10 s")
            XCTAssertTrue(row.label.contains("Open"), "S2.2: the row does not read Open: \(row.label)")
            row.tap()
            XCTAssertTrue(
                waitForLabel(
                    app.element("cabal-join-policy"), containing: "Open: anyone can join.", timeout: screenTimeout),
                "S2.2: no Open: anyone can join. on the cabal within \(Int(screenTimeout)) s"
            )
            XCTAssertTrue(
                waitForLabel(app.buttons["cabal-join-button"], containing: "Join", timeout: 2),
                "S2.2: cabal-join-button does not read Join: \(app.buttons["cabal-join-button"].label)"
            )
            snap(app, "S2-B-open")
        }

        recorder.step("S2.3", "join at once") {
            app.buttons["cabal-join-button"].tap()
            waitForToast(app, "You're in.", step: "S2.3")
            XCTAssertTrue(
                app.element("cabal-action-fund").waitForExistence(timeout: screenTimeout),
                "S2.3: no member actions on the cabal within \(Int(screenTimeout)) s"
            )
            XCTAssertFalse(app.buttons["cabal-join-button"].exists, "S2.3: still offers to join")
            XCTAssertFalse(app.element("cabal-join-requested").exists, "S2.3: a request was filed")
        }
    }

    static func memberRequestsTheGate(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S3.1", "request to join an approval cabal") {
            openBySearch(app, gateCabal(run: run), step: "S3.1")
            XCTAssertTrue(
                waitForLabel(
                    app.element("cabal-join-policy"),
                    containing: "Approval required: the creator reviews each request.", timeout: screenTimeout),
                "S3.1: no Approval required policy line on the cabal within \(Int(screenTimeout)) s"
            )
            app.buttons["cabal-join-button"].tap()
            waitForToast(app, requestedToast, step: "S3.1")
            XCTAssertTrue(
                waitForLabel(
                    app.element("cabal-join-requested"), containing: "Request pending", timeout: screenTimeout),
                "S3.1: no Request pending on the cabal screen"
            )
        }
    }

    static func creatorDeclines(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S3.2", "decline the request") {
            openBySearch(app, gateCabal(run: run), step: "S3.2")
            openRequests(app, step: "S3.2")
            decide(app, approve: false, step: "S3.2")
            waitForToast(app, "Denied.", step: "S3.2")
        }
    }

    static func memberAsksAgain(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S3.3", "see the request was declined") {
            openBySearch(app, gateCabal(run: run), step: "S3.3")
            XCTAssertTrue(
                waitForLabel(
                    app.element("cabal-join-declined"), containing: "Request declined", timeout: screenTimeout),
                "S3.3: no Request declined on the cabal within \(Int(screenTimeout)) s"
            )
            XCTAssertTrue(
                waitForLabel(app.buttons["cabal-join-button"], containing: "Ask again", timeout: 2),
                "S3.3: cabal-join-button does not read Ask again: \(app.buttons["cabal-join-button"].label)"
            )
            snap(app, "S3-B-declined")
        }

        recorder.step("S3.4", "ask again") {
            app.buttons["cabal-join-button"].tap()
            waitForToast(app, requestedToast, step: "S3.4")
            XCTAssertTrue(
                waitForLabel(
                    app.element("cabal-join-requested"), containing: "Request pending", timeout: screenTimeout),
                "S3.4: no Request pending on the cabal screen"
            )
            XCTAssertFalse(app.element("cabal-join-declined").exists, "S3.4: still reads Request declined")
        }
    }
}
