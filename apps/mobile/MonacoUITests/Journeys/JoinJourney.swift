import UIKit
import XCTest

enum JoinJourney {
    static let id = "cabals/join"
    static let version = 2

    static let codeKey = "invite-code"
    static let screenTimeout: TimeInterval = 15

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func requestCabal(run: String) -> String { "QA pot \(run)" }
    static func openCabal(run: String) -> String { "QA open \(run)" }

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
            NSPredicate(format: "identifier == 'cabals-search-field' OR label == 'Find a cabal by name'")
        ).firstMatch
    }

    static func search(_ app: XCUIApplication, for query: String, step: String) {
        let field = searchField(app)
        XCTAssertTrue(field.waitForExistence(timeout: 10), "\(step): no search field on the Cabals tab")
        let clear = app.buttons["Clear search"]
        if clear.exists { clear.tap() }
        field.tap()
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

    static func creatorCopiesCode(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) throws {
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

        var code = ""
        recorder.step("S1.3", "open details and read the invite card") {
            app.buttons["cabal-details-button"].tap()
            XCTAssertTrue(
                app.element("cabal-invite-card").waitForExistence(timeout: 10),
                "S1.3: no invite card in the details within 10 s"
            )
            code = app.staticTexts["cabal-invite-code"].label
            XCTAssertEqual(code.count, 10, "S1.3: the invite code '\(code)' is not 10 characters")
            XCTAssertEqual(app.buttons["cabal-invite-copy"].label, "Copy code", "S1.3: the copy button's label")
            XCTAssertEqual(app.buttons["cabal-invite-share"].label, "Share", "S1.3: the share button's label")
        }
        try JourneyHandoff.write(codeKey, code)

        recorder.step("S1.4", "copy the code") {
            app.buttons["cabal-invite-copy"].tap()
            XCTAssertTrue(
                waitForLabel(app.buttons["cabal-invite-copy"], containing: "Copied", timeout: 2),
                "S1.4: the copy button did not read Copied within 2 s"
            )
            snap(app, "S1-A-invite-card-copied")
        }

        recorder.step("S1.5", "close details and see no pending requests") {
            app.buttons["cabal-details-done"].tap()
            XCTAssertTrue(
                app.buttons["cabal-details-button"].waitForExistence(timeout: 10),
                "S1.5: the cabal screen did not come back within 10 s"
            )
            XCTAssertFalse(
                app.element("cabal-join-requests-heading").exists,
                "S1.5: a new request cabal shows a pending-request block"
            )
        }
    }

    static func memberRequestsByCode(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) throws {
        let name = requestCabal(run: run)
        let code = try JourneyHandoff.read(codeKey)

        recorder.step("S1.6", "search for a cabal no one has named") {
            app.tab("Cabals").tap()
            search(app, for: "zzqq", step: "S1.6")
            let empty = app.element("cabals-search-empty")
            XCTAssertTrue(empty.waitForExistence(timeout: 10), "S1.6: no empty search state within 10 s")
            XCTAssertTrue(
                app.staticTexts["No cabal called \u{201C}zzqq\u{201D}"].exists,
                "S1.6: the empty state does not read No cabal called “zzqq”"
            )
        }

        recorder.step("S1.7", "clear the search and open Join with an invite code") {
            app.buttons["Clear search"].tap()
            app.buttons["cabals-new-button"].firstMatch.tap()
            let join = app.buttons["new-cabal-join-row"]
            XCTAssertTrue(join.waitForExistence(timeout: 10), "S1.7: no Join with an invite code row")
            join.tap()
            XCTAssertTrue(
                app.textFields["join-group-id"].waitForExistence(timeout: 10),
                "S1.7: the code field did not show within 10 s"
            )
            XCTAssertTrue(app.navigationBars["Join a cabal"].exists, "S1.7: the screen is not titled Join a cabal")
        }

        recorder.step("S1.8", "paste the code") {
            UIPasteboard.general.string = code
            app.buttons["join-group-paste"].tap()
            XCTAssertTrue(
                waitForLabel(app.element("join-group-name"), containing: name, timeout: 10),
                "S1.8: the preview does not name \(name) within 10 s"
            )
            XCTAssertTrue(
                waitForLabel(app.buttons["join-group-submit"], containing: "Request to join", timeout: 10),
                "S1.8: the button does not read Request to join: \(app.buttons["join-group-submit"].label)"
            )
        }

        recorder.step("S1.9", "ask to join") {
            app.buttons["join-group-submit"].tap()
            waitForToast(app, "Request sent. You'll be in once the creator says yes.", step: "S1.9")
            XCTAssertTrue(
                waitForLabel(app.element("cabal-header-name"), containing: name, timeout: screenTimeout),
                "S1.9: the cabal screen for \(name) did not show"
            )
            XCTAssertTrue(
                waitForLabel(app.buttons["cabal-join-requested"], containing: "Request sent", timeout: screenTimeout),
                "S1.9: no Request sent on the cabal screen"
            )
            XCTAssertTrue(app.buttons["cabal-join-cancel"].exists, "S1.9: no Cancel request")
            snap(app, "S1-B-request-sent")
        }
    }

    static func creatorApproves(_ app: XCUIApplication, run: String, member: String, recorder: JourneyRecorder) {
        let name = requestCabal(run: run)

        recorder.step("S1.10", "open the cabal and see the request") {
            openBySearch(app, name, step: "S1.10")
            XCTAssertTrue(
                waitForLabel(
                    app.element("cabal-join-requests-heading"), containing: "1 person wants to join", timeout: 2),
                "S1.10: no 1 person wants to join within 2 s of the cabal screen"
            )
            let rows = app.descendants(matching: .any).matching(identifier: "cabal-join-request-row")
            XCTAssertEqual(rows.count, 1, "S1.10: expected one request row")
            XCTAssertTrue(rows.firstMatch.label.contains(member), "S1.10: the request does not name \(member)")
            snap(app, "S1-A-request-pending")
        }

        recorder.step("S1.11", "approve it") {
            app.buttons["cabal-join-approve"].firstMatch.tap()
            waitForToast(app, "Approved.", step: "S1.11")
            XCTAssertTrue(
                app.element("cabal-join-requests-heading").waitForNonExistence(timeout: 10),
                "S1.11: the request block is still on the screen"
            )
            XCTAssertTrue(
                waitForLabel(app.element("cabal-member-count"), containing: "2 members", timeout: 10),
                "S1.11: the hero does not read 2 members: \(app.element("cabal-member-count").label)"
            )
        }
    }

    static func memberEntersAndJoinsOpen(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S1.12", "open the request cabal as a member") {
            openBySearch(app, requestCabal(run: run), step: "S1.12")
            XCTAssertTrue(
                app.element("cabal-action-fund").waitForExistence(timeout: screenTimeout),
                "S1.12: no member actions on the cabal within \(Int(screenTimeout)) s"
            )
            XCTAssertFalse(app.buttons["cabal-join-requested"].exists, "S1.12: still reads Request sent")
            XCTAssertFalse(app.buttons["cabal-join-button"].exists, "S1.12: still offers to join")
            snap(app, "S1-B-member")
        }

        recorder.step("S1.13", "find the open cabal by name") {
            openBySearch(app, openCabal(run: run), step: "S1.13")
            XCTAssertTrue(
                waitForLabel(app.buttons["cabal-join-button"], containing: "Join cabal", timeout: screenTimeout),
                "S1.13: no Join cabal on the open cabal within \(Int(screenTimeout)) s"
            )
        }

        recorder.step("S1.14", "join it") {
            app.buttons["cabal-join-button"].tap()
            waitForToast(app, "You're in.", step: "S1.14")
            XCTAssertTrue(
                waitForLabel(app.element("cabal-member-count"), containing: "2 members", timeout: screenTimeout),
                "S1.14: the hero does not read 2 members: \(app.element("cabal-member-count").label)"
            )
            XCTAssertTrue(
                app.element("cabal-action-fund").waitForExistence(timeout: screenTimeout),
                "S1.14: no member actions on the cabal within \(Int(screenTimeout)) s"
            )
        }
    }
}
