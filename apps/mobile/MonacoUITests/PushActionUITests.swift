import XCTest

nonisolated final class PushActionUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testYesOnAVotePushOpensTheHoldSheetWithYesPicked() throws {
        let app = XCUIApplication()
        app.launchArguments = [
            "-MonacoProposalSample", "pushVote",
            "-MonacoSampleVotePush", "01920000-0000-7000-8000-000000000a01",
        ]
        app.launch()

        let springboard = XCUIApplication(bundleIdentifier: "com.apple.springboard")
        let allow = springboard.alerts.buttons["Allow"].firstMatch
        let banner = springboard.descendants(matching: .any).matching(
            NSPredicate(format: "identifier == 'NotificationShortLookView' OR label CONTAINS '3 votes left'")
        ).firstMatch
        let deadline = Date().addingTimeInterval(15)
        while !banner.exists, Date() < deadline {
            if allow.exists { allow.tap() }
            Thread.sleep(forTimeInterval: 0.5)
        }
        guard banner.exists else {
            throw XCTSkip(
                "No local notification banner appeared within 15 s; if this simulator was slimmed with SimSlim "
                    + "base-slim, re-slim with except: [\"push\"].")
        }
        banner.press(forDuration: 1.0)
        let yes = springboard.buttons["Yes"].firstMatch
        XCTAssertTrue(yes.waitForExistence(timeout: 5))
        yes.tap()

        let sheet = app.descendants(matching: .any).matching(identifier: "vote-hold-sheet").firstMatch
        XCTAssertTrue(sheet.waitForExistence(timeout: 10))
        let hint = app.descendants(matching: .any).matching(identifier: "proposal-vote-hint").firstMatch
        XCTAssertTrue(hint.waitForExistence(timeout: 10))
    }
}
