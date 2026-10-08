import XCTest

enum SettingsNotificationsJourney {
    static let id = "settings/notifications"
    static let version = 4

    static let title = "Know when your cabal votes and trades"
    static let body = "We'll tell you when a vote opens, passes, or a trade fills."

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func waitUntil(_ timeout: TimeInterval, _ condition: () -> Bool) -> Bool {
        FirstRunJourney.waitUntil(timeout, condition)
    }

    static func openSettings(_ app: XCUIApplication, step: String) {
        app.tab("Profile").tap()
        let settings = app.element("profile-settings-row")
        XCTAssertTrue(settings.waitForExistence(timeout: 15), "\(step): no Settings row on Profile")
        app.scrollIntoReach(settings)
        settings.tap()
        XCTAssertTrue(app.navigationBars["Settings"].waitForExistence(timeout: 10), "\(step): Settings did not open")
    }

    static func rowReads(_ app: XCUIApplication, _ state: String, step: String) {
        let row = app.element("settings-notifications")
        XCTAssertTrue(row.waitForExistence(timeout: 5), "\(step): no Notifications row")
        XCTAssertTrue(
            waitUntil(10) { row.label.contains("Notifications") && row.label.contains(state) },
            "\(step): the row reads '\(row.label)', not 'Notifications' and '\(state)'")
    }

    static func offBeforeAsking(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S1.1", "open Settings") {
            openSettings(app, step: "S1.1")
        }

        recorder.step("S1.2", "Notifications reads Off") {
            rowReads(app, "Off", step: "S1.2")
        }
    }

    static func prePrompt(_ app: XCUIApplication, cabalName: String, recorder: JourneyRecorder) {
        let sheet = app.element("push-pre-prompt")

        recorder.step("S2.1", "find the invite on the Cabals tab") {
            app.tab("Cabals").tap()
            XCTAssertTrue(
                app.staticTexts[cabalName].firstMatch.waitForExistence(timeout: 15),
                "S2.1: no invite to '\(cabalName)' on the Cabals tab")
        }

        recorder.step("S2.2", "accept and see the pre-prompt") {
            let accept = app.buttons.matching(identifier: "cabal-invite-accept").firstMatch
            XCTAssertTrue(accept.exists, "S2.2: no Accept button on the invite")
            accept.tap()
            XCTAssertTrue(
                app.staticTexts["You're in."].firstMatch.waitForExistence(timeout: 10), "S2.2: no 'You're in.' toast")
            XCTAssertTrue(sheet.waitForExistence(timeout: 15), "S2.2: the pre-prompt did not show after the join")
            XCTAssertEqual(app.element("push-pre-prompt-title").label, title, "S2.2: the pre-prompt title is wrong")
            XCTAssertTrue(app.staticTexts[body].exists, "S2.2: the pre-prompt does not read '\(body)'")
            XCTAssertEqual(
                app.buttons["push-pre-prompt-turn-on"].label, "Turn on notifications",
                "S2.2: the primary button does not read 'Turn on notifications'")
            XCTAssertEqual(
                app.buttons["push-pre-prompt-not-now"].label, "Not now", "S2.2: no 'Not now' button")
        }

        recorder.step("S2.3", "turn on and allow") {
            app.buttons["push-pre-prompt-turn-on"].tap()
            XCTAssertTrue(app.answerSystemAlert(allow: true), "S2.3: the iOS permission alert never showed")
            XCTAssertTrue(
                sheet.waitForNonExistence(timeout: 15),
                "S2.3: the pre-prompt did not close after Turn on notifications")
        }
    }

    static func onAndOpensSettings(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S3.1", "Notifications reads On") {
            openSettings(app, step: "S3.1")
            rowReads(app, "On", step: "S3.1")
        }

        recorder.step("S3.2", "the row opens iOS Settings") {
            app.element("settings-notifications").tap()
            let settings = XCUIApplication(bundleIdentifier: "com.apple.Preferences")
            XCTAssertTrue(settings.wait(for: .runningForeground, timeout: 15), "S3.2: iOS Settings did not open")
            app.activate()
        }
    }
}
