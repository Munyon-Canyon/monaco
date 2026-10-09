import XCTest

nonisolated final class WelcomeSampleUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    private func launch(_ scenario: String) -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = ["-MonacoWelcomeSample", scenario]
        app.launch()
        return app
    }

    @MainActor
    private func element(_ app: XCUIApplication, _ identifier: String) -> XCUIElement {
        app.descendants(matching: .any).matching(identifier: identifier).firstMatch
    }

    @MainActor
    private func assertDrawn(
        _ app: XCUIApplication, _ shown: XCUIElement, _ what: String, file: StaticString = #filePath,
        line: UInt = #line
    ) {
        XCTAssertTrue(
            shown.waitForExistence(timeout: 40),
            "\(what) should be on screen. On screen:\n\(app.debugDescription.suffix(6000))",
            file: file, line: line
        )
        XCTAssertEqual(app.state, .runningForeground, "the sample must still be running", file: file, line: line)
    }

    @MainActor
    func testSignedOutShowsLoginWithTheReasonAndNoCrash() throws {
        let app = launch("signedOut")
        assertDrawn(app, element(app, "smsPhoneField"), "the phone field")
        XCTAssertTrue(element(app, "signOutReasonNotice").exists, "the reason the session ended")
    }

    @MainActor
    func testCodeShowsTheCodeFieldAfterAPhoneNumber() throws {
        let app = launch("code")
        assertDrawn(app, element(app, "smsCodeField"), "the code field")
    }

    @MainActor
    func testEmailCodeShowsTheCodeFieldAfterAnEmailAddress() throws {
        let app = launch("emailCode")
        assertDrawn(app, element(app, "emailCodeField"), "the email code field")
    }

    @MainActor
    func testCodeRejectedKeepsTheCodeFieldUp() throws {
        let app = launch("codeRejected")
        assertDrawn(app, element(app, "smsCodeField"), "the code field")
    }

    @MainActor
    func testRestoringShowsTheLaunchMark() throws {
        let app = launch("restoring")
        assertDrawn(app, element(app, "sessionRestoringView"), "the restoring screen")
    }

    @MainActor
    func testRestoreFailedShowsItsTitle() throws {
        let app = launch("restoreFailed")
        assertDrawn(app, app.staticTexts["Can't sign you in yet"], "the restore failure")
    }

    @MainActor
    func testGateLoadingShowsHomesShape() throws {
        let app = launch("gateLoading")
        assertDrawn(app, app.navigationBars.firstMatch, "the gate skeleton under its bar")
    }

    @MainActor
    func testGateFailedShowsItsTitle() throws {
        let app = launch("gateFailed")
        assertDrawn(app, app.staticTexts["Your account didn't load"], "the open failure")
    }

    @MainActor
    func testEmptyStatesShowTheirSections() throws {
        let app = launch("emptyStates")
        assertDrawn(app, app.staticTexts["Nothing bought yet"], "the first empty state")
    }
}
