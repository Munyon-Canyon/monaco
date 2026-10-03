import XCTest

nonisolated final class Flow00HarnessUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testInvalidInputToastsTheServerMessage() {
        let app = launch("invalidInput")

        XCTAssertTrue(app.staticTexts["The note is too long."].firstMatch.waitForExistence(timeout: 10))
        XCTAssertTrue(app.staticTexts["Not sent"].exists)
    }

    @MainActor
    func testUnauthorizedPromptsSignIn() {
        let app = launch("unauthorized")

        let prompt = app.descendants(matching: .any).matching(identifier: "system-ping-signed-out").firstMatch
        XCTAssertTrue(prompt.waitForExistence(timeout: 10))
        XCTAssertTrue(prompt.label.contains("You're signed out"), prompt.label)
        XCTAssertTrue(prompt.label.contains("Sign in again to send a ping."), prompt.label)
    }

    @MainActor
    func testInterruptedSaysOffline() {
        let app = launch("interrupted")

        XCTAssertTrue(app.staticTexts["You're offline. Try again."].firstMatch.waitForExistence(timeout: 10))
    }

    @MainActor
    private func launch(_ scenario: String) -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = ["-MonacoFlow", "00", scenario]
        app.launch()
        XCTAssertTrue(app.buttons["Send"].waitForExistence(timeout: 10), "\(scenario) did not open")
        return app
    }
}
