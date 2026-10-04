import XCTest

enum CreateCabalJourney {
    static let id = "cabals/create-cabal"
    static let version = 1

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
            app.navigationBars.buttons.element(boundBy: 0).tap()
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

    static func ensureCabalCreated(_ app: XCUIApplication, as account: JourneyAccount) -> String {
        SignInJourney.ensureSignedIn(app, as: account)
        let recorder = recorder()
        let name = uniqueName()
        openForm(app, recorder: recorder)
        create(app, named: name, recorder: recorder)
        return name
    }
}
