import XCTest

nonisolated final class PushActionUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testYesOnAVotePushOpensTheHoldSheetWithYesPicked() throws {
        let app = XCUIApplication()
        app.launch()
        app.terminate()

        let springboard = XCUIApplication(bundleIdentifier: "com.apple.springboard")
        let banner = springboard.otherElements["NotificationShortLookView"].firstMatch
        XCTAssertTrue(banner.waitForExistence(timeout: 10))
        banner.press(forDuration: 1.0)
        let yes = springboard.buttons["Yes"].firstMatch
        XCTAssertTrue(yes.waitForExistence(timeout: 5))
        yes.tap()

        let sheet = app.descendants(matching: .any).matching(identifier: "vote-hold-sheet").firstMatch
        XCTAssertTrue(sheet.waitForExistence(timeout: 10))
        XCTAssertTrue(app.descendants(matching: .any).matching(identifier: "proposal-vote-hint").firstMatch.exists)
    }
}
