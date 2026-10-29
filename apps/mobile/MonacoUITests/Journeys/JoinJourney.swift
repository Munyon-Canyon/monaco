import UIKit
import XCTest

enum JoinJourney {
    static let id = "cabals/join"
    static let version = 10

    static let screenTimeout: TimeInterval = 15

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func requestCabal(run: String) -> String { "QA pot \(run)" }
    static func openCabal(run: String) -> String { "QA open \(run)" }
    static func gateCabal(run: String) -> String { "QA gate \(run)" }
    static let requestedToast = "Request sent. You'll be in once the creator says yes."

    static func waitForToast(_ app: XCUIApplication, _ message: String, step: String) {
        let banner = app.element("monaco-toast-banner")
        let text = app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", message)).firstMatch
        XCTAssertTrue(
            FollowAndBoardsJourney.waitUntil(10) { (banner.exists && banner.label.contains(message)) || text.exists },
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
        app.tab("Cabals").tap()
        app.popToRoot()
        CabalsDiscoverJourney.openFind(app, step: step)
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

        recorder.step("S1.1", "open the request cabal as its creator") {
            app.openCabal(named: name)
            XCTAssertTrue(
                waitForLabel(app.element("cabal-header-name"), containing: name, timeout: screenTimeout),
                "S1.1: the cabal screen for \(name) did not show within \(Int(screenTimeout)) s"
            )
        }

        recorder.step("S1.2", "read the header") {
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
            app.openCabal(named: name)
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
            app.openCabal(named: name)
            openRequests(app, step: "S1.7")
            let rows = app.descendants(matching: .any).matching(identifier: "cabal-join-request-row")
            XCTAssertEqual(rows.count, 1, "S1.7: expected one request row")
            XCTAssertTrue(rowNames(rows.firstMatch, member), "S1.7: the request does not name \(member)")
            XCTAssertTrue(rows.firstMatch.label.contains("Asked to join"), "S1.7: the row does not read Asked to join")
            snap(app, "S1-A-request-pending")
        }

        recorder.step("S1.8", "approve it") {
            decide(app, approve: true, step: "S1.8")
            waitForToast(app, "Approved.", step: "S1.8")
            XCTAssertTrue(
                waitForRequestsGone(app, timeout: 10),
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
            app.openCabal(named: requestCabal(run: run))
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
        app.openCabalTab("members")
        let all = app.element("cabal-members-see-all")
        app.scrollIntoReach(all)
        XCTAssertTrue(
            all.waitForExistence(timeout: screenTimeout),
            "\(step): no See all members row within \(Int(screenTimeout)) s")
        all.tap()
        XCTAssertTrue(
            app.element("cabal-join-request-row").waitForExistence(timeout: screenTimeout),
            "\(step): Members shows no request within \(Int(screenTimeout)) s")
    }

    static func rowNames(_ row: XCUIElement, _ text: String) -> Bool {
        row.label.contains(text)
            || row.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", text)).firstMatch.exists
    }

    static func waitForRequestsGone(_ app: XCUIApplication, timeout: TimeInterval) -> Bool {
        let deadline = Date().addingTimeInterval(timeout)
        repeat {
            if !app.element("cabal-join-request-row").exists { return true }
            RunLoop.current.run(until: Date().addingTimeInterval(0.1))
        } while Date() < deadline
        return !app.element("cabal-join-request-row").exists
    }

    static func decide(_ app: XCUIApplication, approve: Bool, step: String) {
        let row = app.descendants(matching: .any).matching(identifier: "cabal-join-request-row").firstMatch
        XCTAssertTrue(row.waitForExistence(timeout: 10), "\(step): no request row within 10 s")
        if approve {
            let button = app.buttons["cabal-join-approve"].firstMatch
            XCTAssertTrue(button.waitForExistence(timeout: 5), "\(step): the request row has no Approve")
            button.tap()
        } else {
            row.press(forDuration: 1.2)
            let decline = app.buttons["cabal-join-deny"].firstMatch
            XCTAssertTrue(decline.waitForExistence(timeout: 5), "\(step): the row's menu has no Decline")
            decline.tap()
        }
    }

    static func creatorOpensTheCabal(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        let name = openCabal(run: run)

        recorder.step("S2.1", "propose Who can join · Open and vote Yes") {
            app.openCabal(named: name)
            app.openCabalTab("rules")
            CabalsEditRulesJourney.assertRule(app, "join", reads: "Approval required", step: "S2.1")
            app.element("cabal-rules-join").tap()
            let open = app.buttons["Open"].firstMatch
            XCTAssertTrue(open.waitForExistence(timeout: 10), "S2.1: the join proposal has no Open choice within 10 s")
            open.tap()
            let propose = app.buttons["cabal-rule-proposal-propose"]
            XCTAssertTrue(propose.waitForExistence(timeout: 5), "S2.1: no Propose button on the proposal")
            propose.tap()
            app.pickAndHold("yes")
            app.openCabalTab("rules")
            CabalsEditRulesJourney.assertRule(app, "join", reads: "Open", step: "S2.1")
        }
    }

    static func memberJoinsTheOpenCabal(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        let name = openCabal(run: run)

        recorder.step("S2.2", "find the open cabal and read its policy") {
            app.openCabal(named: name)
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
            app.openCabal(named: gateCabal(run: run))
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
            app.openCabal(named: gateCabal(run: run))
            openRequests(app, step: "S3.2")
            decide(app, approve: false, step: "S3.2")
            waitForToast(app, "Denied.", step: "S3.2")
        }
    }

    static func memberAsksAgain(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S3.3", "see the request was declined") {
            app.openCabal(named: gateCabal(run: run))
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
