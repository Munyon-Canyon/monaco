import XCTest

enum HomeActivityJourney {
    static let id = "home/activity"
    static let version = 1

    static let comingSoon = "Your activity shows up here soon."

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func opensFromBell(_ app: XCUIApplication, recorder: JourneyRecorder) {
        let bell = app.element("home-notifications-button")

        recorder.step("S1.1", "tap the Home bell") {
            app.tab("Home").tap()
            XCTAssertTrue(bell.waitForExistence(timeout: 15), "S1.1: no bell on Home")
            bell.tap()
            XCTAssertTrue(app.navigationBars["Activity"].waitForExistence(timeout: 15), "S1.1: Activity did not open")
        }

        recorder.step("S1.2", "Activity says it opens soon") {
            let caption = app.element("activity-coming-soon")
            XCTAssertTrue(caption.waitForExistence(timeout: 15), "S1.2: no activity-coming-soon within 15 s")
            XCTAssertEqual(caption.label, comingSoon, "S1.2: the caption reads '\(caption.label)', not '\(comingSoon)'")
        }

        recorder.step("S1.3", "no made-up rows") {
            XCTAssertFalse(app.element("activity-mark-all-read").exists, "S1.3: Mark all read shows with no list")
            XCTAssertFalse(app.element("activity-empty").exists, "S1.3: the empty state shows instead of the caption")
            let items = app.descendants(matching: .any).matching(
                NSPredicate(format: "identifier BEGINSWITH 'activity-item-'"))
            XCTAssertEqual(items.count, 0, "S1.3: \(items.count) activity rows show with no notifications API")
            XCTAssertFalse(app.element("monaco-toast-banner").exists, "S1.3: a toast showed on Activity")
        }

        recorder.step("S1.4", "go back to Home") {
            app.navigationBars.buttons.element(boundBy: 0).tap()
            XCTAssertTrue(bell.waitForExistence(timeout: 10), "S1.4: the bell did not come back on Home")
            XCTAssertTrue(app.tab("Home").exists, "S1.4: the tab bar is gone")
        }
    }
}
