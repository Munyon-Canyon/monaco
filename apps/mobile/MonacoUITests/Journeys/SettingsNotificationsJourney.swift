import XCTest

enum SettingsNotificationsJourney {
    static let id = "settings/notifications"
    static let version = 6

    static let title = "Hear how votes end?"
    static let body = "Get each result when a vote closes. No more vote reminders after you vote."

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func waitUntil(_ timeout: TimeInterval, _ condition: () -> Bool) -> Bool {
        FirstRunJourney.waitUntil(timeout, condition)
    }

    static func openSettings(_ app: XCUIApplication, step: String) {
        app.tab("Profile").tap()
        if app.navigationBars["Settings"].waitForExistence(timeout: 3) { return }
        let settings = app.element("profile-settings-row")
        _ = app.element("profile-card").waitForExistence(timeout: 10)
        app.scrollIntoReach(settings, maxSwipes: 30)
        XCTAssertTrue(settings.waitForExistence(timeout: 15), "\(step): no Settings row on Profile")
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

    static func prePrompt(_ app: XCUIApplication, proposalID: String, recorder: JourneyRecorder) {
        let sheet = app.element("push-pre-prompt")

        recorder.step("S2.1", "vote Yes on the open proposal from Home") {
            app.tab("Home").tap()
            app.homeVoteCardPickAndHold(proposalID: proposalID, choice: "yes", timeout: 20)
        }

        recorder.step("S2.2", "the sheet waits for the toast, then shows") {
            let toast = app.element("monaco-toast-banner")
            if toast.waitForExistence(timeout: 10) {
                XCTAssertFalse(sheet.exists, "S2.2: the pre-prompt showed while the vote's toast was still up")
            }
            app.waitForToastGone(timeout: 10)
            XCTAssertTrue(sheet.waitForExistence(timeout: 15), "S2.2: the pre-prompt did not show after the first vote")
            XCTAssertTrue(app.staticTexts[title].exists, "S2.2: the pre-prompt title is not '\(title)'")
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

    static func switchReads(_ app: XCUIApplication, _ identifier: String, _ value: String, step: String) {
        let toggle = app.switches[identifier]
        XCTAssertTrue(toggle.waitForExistence(timeout: 10), "\(step): no switch '\(identifier)'")
        XCTAssertTrue(
            waitUntil(10) { toggle.isEnabled && (toggle.value as? String) == value },
            "\(step): '\(identifier)' reads '\(toggle.value as? String ?? "nothing")', not '\(value)'")
    }

    static func categorySwitchSaves(_ app: XCUIApplication, recorder: JourneyRecorder) {
        let decisions = "settings-category-decisions-switch"
        let replies = "settings-category-replies-switch"

        recorder.step("S4.1", "both switches read on") {
            openSettings(app, step: "S4.1")
            switchReads(app, decisions, "1", step: "S4.1")
            switchReads(app, replies, "1", step: "S4.1")
            XCTAssertFalse(
                app.staticTexts["Notification choices open soon."].exists,
                "S4.1: Settings still reads 'Notification choices open soon.'")
        }

        recorder.step("S4.2", "turn Decision requests off") {
            app.switches[decisions].tap()
            switchReads(app, decisions, "0", step: "S4.2")
            switchReads(app, replies, "1", step: "S4.2")
        }

        recorder.step("S4.3", "leave and come back") {
            app.navigationBars.buttons.firstMatch.tap()
            openSettings(app, step: "S4.3")
            switchReads(app, decisions, "0", step: "S4.3")
            switchReads(app, replies, "1", step: "S4.3")
        }
    }
}
