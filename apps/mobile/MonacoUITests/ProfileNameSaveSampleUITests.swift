import XCTest

nonisolated final class ProfileNameSaveSampleUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    private func launchApp(_ scenario: String) -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = ["-MonacoProfileSample", scenario]
        app.launch()
        return app
    }

    @MainActor
    private func attachScreenshot(_ app: XCUIApplication, name: String) {
        let attachment = XCTAttachment(screenshot: app.screenshot())
        attachment.name = name
        attachment.lifetime = .keepAlways
        add(attachment)
    }

    @MainActor
    private func anyElement(_ app: XCUIApplication, _ identifier: String) -> XCUIElement {
        app.descendants(matching: .any).matching(identifier: identifier).firstMatch
    }

    @MainActor
    func testReadyScreenHasNothingToSaveAndShowsTheBio() throws {
        let app = launchApp("editProfileReady")

        XCTAssertTrue(app.textFields["profile-name-field"].waitForExistence(timeout: 10))
        XCTAssertTrue(app.textFields["edit-profile-username-field"].exists)
        XCTAssertFalse(app.buttons["profile-name-save"].isEnabled, "nothing changed, so Save changes stays off")
        XCTAssertTrue(anyElement(app, "edit-profile-bio-field").exists, "the bio field is live")
        XCTAssertTrue(anyElement(app, "edit-profile-share-performance").exists, "the 1M toggle is live")
        attachScreenshot(app, name: "01-ready")
    }

    @MainActor
    func testInvalidUsernameNamesTheRuleAndBlocksSave() throws {
        let app = launchApp("editProfileInvalid")

        let username = app.textFields["edit-profile-username-field"]
        XCTAssertTrue(username.waitForExistence(timeout: 10))
        XCTAssertEqual(username.value as? String, "Choose a valid username.")
        XCTAssertFalse(app.buttons["profile-name-save"].isEnabled)
        attachScreenshot(app, name: "02-invalid")
    }

    @MainActor
    func testSavingShowsTheNoticeAndDisablesSave() throws {
        let app = launchApp("editProfileSaving")

        XCTAssertTrue(anyElement(app, "edit-profile-saving").waitForExistence(timeout: 10))
        XCTAssertFalse(app.buttons["profile-name-save"].isEnabled)
        attachScreenshot(app, name: "03-saving")
    }

    @MainActor
    func testSavedShowsTheNotice() throws {
        let app = launchApp("editProfileSaved")

        XCTAssertTrue(anyElement(app, "edit-profile-saved").waitForExistence(timeout: 10))
        attachScreenshot(app, name: "04-saved")
    }

    @MainActor
    func testRejectedSaveToastsAndKeepsSaveAvailable() throws {
        let app = launchApp("editProfileSaveFailure")

        let field = app.textFields["profile-name-field"]
        XCTAssertTrue(field.waitForExistence(timeout: 10))
        field.tap()
        field.typeText("a")

        let save = app.buttons["profile-name-save"]
        XCTAssertTrue(save.isEnabled, "a changed, valid name can be saved")
        save.tap()

        XCTAssertTrue(
            anyElement(app, "monaco-toast-banner").waitForExistence(timeout: 10),
            "a rejected save must say so"
        )
        attachScreenshot(app, name: "05-save-rejected")
        XCTAssertTrue(save.isEnabled, "the button comes back so the member can retry")
    }

    @MainActor
    func testLoadFailureOffersTryAgain() throws {
        let app = launchApp("editProfileError")

        XCTAssertTrue(anyElement(app, "edit-profile-error").waitForExistence(timeout: 10))
        attachScreenshot(app, name: "06-error")
    }
}
