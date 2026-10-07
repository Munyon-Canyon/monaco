import UIKit
import XCTest

enum CreateCabalJourney {
    static let id = "cabals/create-cabal"
    static let version = 4

    private static let listTimeout: TimeInterval = 15
    private static let formTimeout: TimeInterval = 10
    private static let createdTimeout: TimeInterval = 20
    private static let toastTimeout: TimeInterval = 5

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func uniqueName() -> String {
        "QA pot \(String(Int(Date().timeIntervalSince1970) % 100_000))"
    }

    static func openForm(_ app: XCUIApplication, recorder: JourneyRecorder) {
        let plus = app.buttons["cabals-new-button"]
        let start = app.buttons["new-cabal-create-row"]

        recorder.step("S1.1", "open the Cabals tab") {
            app.tab("Cabals").tap()
            XCTAssertTrue(
                plus.waitForExistence(timeout: listTimeout),
                "S1.1: the + button did not show on the Cabals tab within \(Int(listTimeout)) s"
            )
        }

        recorder.step("S1.2", "open the New cabal sheet") {
            plus.tap()
            XCTAssertTrue(start.waitForExistence(timeout: formTimeout), "S1.2: the sheet has no Start a cabal row")
            XCTAssertTrue(
                app.buttons["new-cabal-join-row"].exists, "S1.2: the sheet has no Join with an invite code row")
        }

        recorder.step("S1.3", "open the create form") {
            start.tap()
            XCTAssertTrue(
                app.textFields["create-group-name"].waitForExistence(timeout: formTimeout),
                "S1.3: the name field did not show within \(Int(formTimeout)) s"
            )
            for rule in ["create-rule-join", "create-rule-voters", "create-rule-threshold", "create-rule-expiry"] {
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

        recorder.step("S2.2", "pick I approve, Just me, Everyone agrees and 1 hour") {
            for (rule, label) in [
                ("create-rule-join", "I approve"), ("create-rule-voters", "Just me"),
                ("create-rule-threshold", "Everyone agrees"), ("create-rule-expiry", "1 hour"),
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

        recorder.step("S2.3", "double-tap Create cabal and land on the cabal") {
            app.scrollIntoReach(submit)
            let frame = submit.frame
            let spot = app.coordinate(withNormalizedOffset: .zero)
                .withOffset(CGVector(dx: frame.midX, dy: frame.midY))
            submit.tap()
            spot.tap()
            let hero = app.element("cabal-header-name")
            XCTAssertTrue(
                hero.waitForExistence(timeout: createdTimeout),
                "S2.3: the cabal screen did not show within \(Int(createdTimeout)) s"
            )
            XCTAssertEqual(hero.label, name, "S2.3: the hero does not name the cabal")
            XCTAssertFalse(app.textFields["create-group-name"].exists, "S2.3: the create form is still on screen")
        }

        recorder.step("S2.4", "see the Cabal created toast") {
            let toast = app.element("monaco-toast-banner")
            XCTAssertTrue(toast.waitForExistence(timeout: toastTimeout), "S2.4: no toast within \(Int(toastTimeout)) s")
            XCTAssertTrue(toast.label.contains("Cabal created."), "S2.4: the toast reads '\(toast.label)'")
        }

        recorder.step("S2.5", "see one member") {
            let count = app.element("cabal-member-count")
            XCTAssertTrue(count.waitForExistence(timeout: formTimeout), "S2.5: no member count on the cabal")
            XCTAssertEqual(count.label, "1 member", "S2.5: the member count is wrong")
        }

        recorder.step("S2.6", "go back to the Cabals list and find the cabal once") {
            app.waitForToastGone()
            app.dismissPushPrePromptIfShown()
            app.tapBack()
            let list = app.element("cabals-list")
            XCTAssertTrue(list.waitForExistence(timeout: formTimeout), "S2.6: Back did not land on the Cabals list")
            let named = app.buttons.matching(
                NSPredicate(
                    format: "identifier BEGINSWITH 'cabals-list-card-' AND (label == %@ OR label BEGINSWITH %@)",
                    name, name + ","))
            XCTAssertTrue(
                named.firstMatch.waitForExistence(timeout: formTimeout), "S2.6: '\(name)' is not on the Cabals list")
            XCTAssertEqual(named.count, 1, "S2.6: '\(name)' is on the Cabals list \(named.count) times")
            let newCard = app.buttons["cabals-list-new"]
            for _ in 0..<20 where !newCard.exists {
                list.swipeLeft(velocity: .fast)
            }
            XCTAssertTrue(newCard.exists, "S2.6: no + New cabal card on the Cabals list")
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

    static let duoCodeKey = "duo-invite-code"

    static func duoCabal(run: String) -> String { "QA duo \(run)" }

    static func creatorStartsAnOpenCabal(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) throws
        -> String
    {
        let name = duoCabal(run: run)
        let submit = app.buttons["create-group-submit"]

        recorder.step("S4.1", "open the create form") {
            app.tab("Cabals").tap()
            let plus = app.buttons["cabals-new-button"]
            XCTAssertTrue(plus.waitForExistence(timeout: listTimeout), "S4.1: no + on the Cabals tab")
            plus.tap()
            let start = app.buttons["new-cabal-create-row"]
            XCTAssertTrue(start.waitForExistence(timeout: formTimeout), "S4.1: the sheet has no Start a cabal row")
            start.tap()
            XCTAssertTrue(
                app.textFields["create-group-name"].waitForExistence(timeout: formTimeout),
                "S4.1: the name field did not show within \(Int(formTimeout)) s"
            )
        }

        recorder.step("S4.2", "name it, let anyone join, and create it") {
            let field = app.textFields["create-group-name"]
            field.tap()
            field.typeText(name + "\n")
            let anyone = app.element("create-rule-join").buttons["Anyone"]
            app.scrollIntoReach(anyone)
            XCTAssertTrue(anyone.exists, "S4.2: no Anyone in create-rule-join")
            anyone.tap()
            XCTAssertTrue(anyone.isSelected, "S4.2: Anyone is not selected in create-rule-join")
            app.scrollIntoReach(submit)
            submit.tap()
            XCTAssertTrue(
                JoinJourney.waitForLabel(app.element("cabal-header-name"), containing: name, timeout: createdTimeout),
                "S4.2: the cabal screen for \(name) did not show within \(Int(createdTimeout)) s"
            )
            JoinJourney.waitForToast(app, "Cabal created.", step: "S4.2")
        }

        var code = ""
        recorder.step("S4.3", "read the invite code") {
            app.buttons["cabal-details-button"].tap()
            let card = app.element("cabal-invite-card")
            XCTAssertTrue(card.waitForExistence(timeout: formTimeout), "S4.3: no invite card within 10 s")
            code = app.staticTexts["cabal-invite-code"].label
            XCTAssertEqual(code.count, 10, "S4.3: the invite code '\(code)' is not 10 characters")
        }
        try JourneyHandoff.write(duoCodeKey, code)
        return code
    }

    static func friendJoinsWithTheCode(_ app: XCUIApplication, run: String, code: String, recorder: JourneyRecorder) {
        let name = duoCabal(run: run)

        recorder.step("S4.4", "open Join with an invite code") {
            app.tab("Cabals").tap()
            let plus = app.buttons["cabals-new-button"]
            XCTAssertTrue(plus.waitForExistence(timeout: listTimeout), "S4.4: no + on the Cabals tab")
            plus.tap()
            let join = app.buttons["new-cabal-join-row"]
            XCTAssertTrue(join.waitForExistence(timeout: formTimeout), "S4.4: no Join with an invite code row")
            join.tap()
            XCTAssertTrue(
                app.textFields["join-group-id"].waitForExistence(timeout: formTimeout),
                "S4.4: the code field did not show within 10 s"
            )
        }

        recorder.step("S4.5", "paste the code") {
            UIPasteboard.general.string = code
            app.buttons["join-group-paste"].tap()
            XCTAssertTrue(
                JoinJourney.waitForLabel(app.element("join-group-name"), containing: name, timeout: formTimeout),
                "S4.5: the preview does not name \(name) within 10 s"
            )
            XCTAssertTrue(
                JoinJourney.waitForLabel(
                    app.buttons["join-group-submit"], containing: "Join cabal", timeout: formTimeout),
                "S4.5: the button does not read Join cabal: \(app.buttons["join-group-submit"].label)"
            )
        }

        recorder.step("S4.6", "join") {
            app.buttons["join-group-submit"].tap()
            JoinJourney.waitForToast(app, "You're in.", step: "S4.6")
            XCTAssertTrue(
                JoinJourney.waitForLabel(
                    app.element("cabal-member-count"), containing: "2 members", timeout: listTimeout),
                "S4.6: the hero does not read 2 members: \(app.element("cabal-member-count").label)"
            )
        }
    }

    static func creatorSeesTheFriend(_ app: XCUIApplication, run: String, friend: String, recorder: JourneyRecorder) {
        let name = duoCabal(run: run)

        recorder.step("S4.7", "open the cabal and find the friend on the member board") {
            JoinJourney.openBySearch(app, name, step: "S4.7")
            XCTAssertTrue(
                JoinJourney.waitForLabel(
                    app.element("cabal-member-count"), containing: "2 members", timeout: listTimeout),
                "S4.7: the hero does not read 2 members: \(app.element("cabal-member-count").label)"
            )
            let row = app.buttons.matching(
                NSPredicate(format: "identifier BEGINSWITH 'cabal-member-' AND label CONTAINS %@", friend)
            ).firstMatch
            app.scrollIntoReach(row, maxSwipes: 12)
            XCTAssertTrue(row.waitForExistence(timeout: listTimeout), "S4.7: no member board row names \(friend)")
        }
    }
}
