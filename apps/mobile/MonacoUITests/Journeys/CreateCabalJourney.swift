import UIKit
import XCTest

enum CreateCabalJourney {
    static let id = "cabals/create-cabal"
    static let version = 12

    private static let listTimeout: TimeInterval = 15
    private static let formTimeout: TimeInterval = 10
    private static let createdTimeout: TimeInterval = 20

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func uniqueName() -> String {
        "QA pot \(String(Int(Date().timeIntervalSince1970) % 100_000))"
    }

    static func openForm(_ app: XCUIApplication, recorder: JourneyRecorder) {
        if app.textFields["create-group-name"].exists { return }
        let plus = app.buttons["cabals-new-button"]

        recorder.step("S1.1", "open the Cabals tab") {
            app.tab("Cabals").tap()
            XCTAssertTrue(
                plus.waitForExistence(timeout: listTimeout),
                "S1.1: the + button did not show on the Cabals tab within \(Int(listTimeout)) s"
            )
        }

        recorder.step("S1.2", "open the create form from the +") {
            plus.tap()
            XCTAssertTrue(
                app.textFields["create-group-name"].waitForExistence(timeout: formTimeout),
                "S1.2: the + did not open Start a cabal within \(Int(formTimeout)) s"
            )
            XCTAssertFalse(app.buttons["new-cabal-create-row"].exists, "S1.2: the New cabal sheet still shows")
        }

        recorder.step("S1.3", "see the rule blocks") {
            for rule in ["create-rule-join", "create-rule-voters", "create-rule-expiry"] {
                XCTAssertTrue(app.element(rule).exists, "S1.3: the form has no \(rule)")
            }
            let week = app.element("create-rule-expiry").buttons["1 week"]
            XCTAssertTrue(week.isSelected, "S1.3: '1 week' is not selected in create-rule-expiry")
        }
    }

    static func create(_ app: XCUIApplication, named name: String, recorder: JourneyRecorder) {
        let submit = app.buttons["create-group-submit"]

        recorder.step("S2.1", "type the name") {
            let field = app.textFields["create-group-name"]
            field.tap()
            field.typeText(name + "\n")
            XCTAssertTrue(submit.isEnabled, "S2.1: Create cabal stayed disabled for '\(name)'")
        }

        recorder.step("S2.2", "pick Approval required, People I pick and 1 hour") {
            for (rule, label) in [
                ("create-rule-join", "Approval required"), ("create-rule-voters", "People I pick"),
                ("create-rule-expiry", "1 hour"),
            ] {
                let choice = app.element(rule).buttons[label]
                app.scrollIntoReach(choice)
                for _ in 0..<4 where choice.frame.maxY > submit.frame.minY {
                    app.swipeUp()
                }
                XCTAssertTrue(choice.exists, "S2.2: no '\(label)' in \(rule)")
                choice.tap()
                XCTAssertTrue(choice.isSelected, "S2.2: '\(label)' is not selected in \(rule)")
            }
        }

        var createdToastLabel = ""

        recorder.step("S2.3", "double-tap Create cabal and land on the cabal") {
            app.scrollIntoReach(submit)
            submit.doubleTap()
            let toast = app.element("monaco-toast-banner")
            if toast.waitForExistence(timeout: createdTimeout) {
                createdToastLabel = toast.label
            }
            let hero = app.element("cabal-header-name")
            let landed = JourneyDiagnostics.wait(for: hero, in: app, timeout: createdTimeout)
            XCTAssertTrue(
                landed,
                "S2.3: the cabal screen did not show within \(Int(createdTimeout)) s"
            )
            XCTAssertEqual(hero.label, name, "S2.3: the hero does not name the cabal")
            XCTAssertFalse(app.textFields["create-group-name"].exists, "S2.3: the create form is still on screen")
        }

        recorder.step("S2.4", "see the Cabal created toast") {
            XCTAssertFalse(createdToastLabel.isEmpty, "S2.4: no toast within \(Int(createdTimeout)) s of Create cabal")
            XCTAssertTrue(createdToastLabel.contains("Cabal created."), "S2.4: the toast reads '\(createdToastLabel)'")
        }

        recorder.step("S2.5", "see one member") {
            let count = app.element("cabal-member-count")
            XCTAssertTrue(count.waitForExistence(timeout: formTimeout), "S2.5: no member count on the cabal")
            XCTAssertEqual(count.label, "1 member", "S2.5: the member count is wrong")
        }

        recorder.step("S2.6", "go back to the Cabals tab and find the cabal tile once") {
            app.waitForToastGone()
            app.dismissPushPrePromptIfShown()
            app.tapBack()
            let grid = app.element("cabals-grid")
            XCTAssertTrue(grid.waitForExistence(timeout: formTimeout), "S2.6: Back did not land on the Cabals tab")
            let named = app.buttons.matching(
                NSPredicate(
                    format: "identifier BEGINSWITH 'cabals-tile-' AND (label == %@ OR label BEGINSWITH %@)",
                    name, name + ","))
            XCTAssertTrue(
                named.firstMatch.waitForExistence(timeout: formTimeout),
                "S2.6: '\(name)' is not a tile on the Cabals tab")
            XCTAssertEqual(named.count, 1, "S2.6: '\(name)' is a tile on the Cabals tab \(named.count) times")
        }
    }

    static func blankNameStaysDisabled(_ app: XCUIApplication, recorder: JourneyRecorder) {
        let submit = app.buttons["create-group-submit"]

        recorder.step("S3.1", "see Create cabal disabled with no name") {
            app.scrollIntoReach(submit)
            XCTAssertTrue(submit.exists, "S3.1: no Create cabal button")
            XCTAssertFalse(submit.isEnabled, "S3.1: Create cabal is enabled with an empty name")
        }

        recorder.step("S3.2", "type spaces and see Create cabal stay disabled") {
            let field = app.textFields["create-group-name"]
            field.tap()
            field.typeText("   ")
            XCTAssertFalse(submit.isEnabled, "S3.2: Create cabal is enabled for a blank name")
        }
    }

    static let duoCabalKey = "duo-cabal"

    static func duoCabal(run: String) -> String { "QA duo \(run)" }

    static func creatorStartsAnOpenCabal(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) throws {
        let name = duoCabal(run: run)
        let submit = app.buttons["create-group-submit"]

        recorder.step("S4.1", "open the create form") {
            app.tab("Cabals").tap()
            let plus = app.buttons["cabals-new-button"]
            if !plus.exists, app.buttons["BackButton"].exists { app.buttons["BackButton"].tap() }
            XCTAssertTrue(plus.waitForExistence(timeout: listTimeout), "S4.1: no + on the Cabals tab")
            plus.tap()
            XCTAssertTrue(
                app.textFields["create-group-name"].waitForExistence(timeout: formTimeout),
                "S4.1: the name field did not show within \(Int(formTimeout)) s"
            )
        }

        recorder.step("S4.2", "name it and create it") {
            let field = app.textFields["create-group-name"]
            field.tap()
            field.typeText(name + "\n")
            let approval = app.element("create-rule-join").buttons["Approval required"]
            app.scrollIntoReach(approval)
            approval.tap()
            XCTAssertTrue(approval.isSelected, "S4.2: 'Approval required' is not selected in create-rule-join")
            app.scrollIntoReach(submit)
            submit.tap()
            XCTAssertTrue(
                app.staticTexts["Cabal created."].firstMatch.waitForExistence(timeout: createdTimeout),
                "S4.2: the toast \"Cabal created.\" did not show within \(Int(createdTimeout)) s"
            )
            XCTAssertTrue(
                JoinJourney.waitForLabel(app.element("cabal-header-name"), containing: name, timeout: createdTimeout),
                "S4.2: the cabal screen for \(name) did not show within \(Int(createdTimeout)) s"
            )
        }

        try JourneyHandoff.write(duoCabalKey, name)
    }

    static func friendRequestsToJoin(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        let name = duoCabal(run: run)

        recorder.step("S4.3", "find the cabal by name") {
            app.openCabal(named: name)
            XCTAssertTrue(
                app.buttons["cabal-join-button"].waitForExistence(timeout: listTimeout),
                "S4.3: no Request to join on the cabal within \(Int(listTimeout)) s"
            )
        }

        recorder.step("S4.4", "request to join") {
            app.buttons["cabal-join-button"].tap()
            JoinJourney.waitForToast(
                app, "Request sent. You'll be in once the creator says yes.", step: "S4.4")
            XCTAssertTrue(
                app.element("cabal-join-requested").waitForExistence(timeout: listTimeout),
                "S4.4: the cabal does not read Request pending for the user who asked"
            )
        }
    }

    static func creatorSeesTheFriend(_ app: XCUIApplication, run: String, friend: String, recorder: JourneyRecorder) {
        let name = duoCabal(run: run)

        recorder.step("S4.5", "approve the friend from the Members tab and find them under Members") {
            app.openCabal(named: name)
            app.openCabalTab("members")
            let all = app.element("cabal-members-see-all")
            if !all.waitForExistence(timeout: 3) { app.swipeUp() }
            XCTAssertTrue(all.waitForExistence(timeout: listTimeout), "S4.5: no See all on the Members tab")
            all.tap()
            let approve = app.buttons["cabal-join-approve"].firstMatch
            XCTAssertTrue(approve.waitForExistence(timeout: listTimeout), "S4.5: no Approve on the request row")
            approve.tap()
            JoinJourney.waitForToast(app, "Approved.", step: "S4.5")
            XCTAssertTrue(
                approve.waitForNonExistence(timeout: listTimeout), "S4.5: the request row is still on Members")
            let row = app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", friend)).firstMatch
            XCTAssertTrue(row.waitForExistence(timeout: listTimeout), "S4.5: no Members row names \(friend)")
            app.tapBack()
            XCTAssertTrue(
                JoinJourney.waitForLabel(
                    app.element("cabal-member-count"), containing: "2 members", timeout: listTimeout),
                "S4.5: the hero does not read 2 members: \(app.element("cabal-member-count").label)"
            )
        }
    }
}
