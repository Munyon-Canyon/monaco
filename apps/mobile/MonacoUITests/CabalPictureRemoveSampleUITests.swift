import XCTest

nonisolated final class CabalPictureRemoveSampleUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    private func launch(_ scenario: String) -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = ["-MonacoGroupDetailSample", scenario]
        app.launch()
        return app
    }

    @MainActor
    private func element(_ app: XCUIApplication, _ identifier: String) -> XCUIElement {
        app.descendants(matching: .any).matching(identifier: identifier).firstMatch
    }

    @MainActor
    private func screenshot(_ app: XCUIApplication, _ name: String) {
        let attachment = XCTAttachment(screenshot: app.screenshot())
        attachment.name = name
        attachment.lifetime = .keepAlways
        add(attachment)
    }

    @MainActor
    private func askToRemove(_ app: XCUIApplication, _ mark: XCUIElement) {
        mark.press(forDuration: 1.2)
        let remove = element(app, "cabal-picture-remove")
        XCTAssertTrue(remove.waitForExistence(timeout: 10), "a long press on the mark offers Remove picture")
        remove.tap()
        XCTAssertTrue(
            element(app, "cabal-picture-remove-confirm").waitForExistence(timeout: 10),
            "choosing Remove picture asks before it removes")
    }

    @MainActor
    func testTheHeroKeepsItsIdentityRowOnTheGutterAndAsksBeforeRemovingThePicture() {
        let app = launch("picture")
        let mark = element(app, "cabal-header-picture")
        XCTAssertTrue(mark.waitForExistence(timeout: 40), "the creator's hero shows the mark")

        XCTAssertFalse(
            element(app, "cabal-picture-remove-button").exists, "no red button sits under the mark in the hero")
        XCTAssertEqual(mark.frame.minX, 16, accuracy: 0.5, "the mark starts on the gutter")
        XCTAssertEqual(
            element(app, "cabal-header-name").frame.minX, 76, accuracy: 0.5, "the name sits 12 pt after the mark")
        screenshot(app, "hero")

        askToRemove(app, mark)
        screenshot(app, "confirm")
        app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.6)).tap()
        XCTAssertTrue(
            element(app, "cabal-picture-remove-confirm").waitForNonExistence(timeout: 10),
            "the dialog has no Cancel button, a tap outside dismisses it")
        XCTAssertFalse(
            app.staticTexts["Picture removed."].waitForExistence(timeout: 2), "dismissing leaves the picture alone")

        askToRemove(app, mark)
        element(app, "cabal-picture-remove-confirm").tap()
        XCTAssertTrue(
            app.staticTexts["Picture removed."].waitForExistence(timeout: 10), "confirming removes it and toasts")
        screenshot(app, "removed")
    }

    @MainActor
    func testCabalSettingsKeepsItsRemoveButtonAndAsksBeforeUsingIt() {
        let app = launch("picture")
        let details = element(app, "cabal-details-button")
        XCTAssertTrue(details.waitForExistence(timeout: 40), "the hero opens Cabal details")
        details.tap()
        let edit = element(app, "cabal-rules-edit")
        app.scrollIntoReach(edit)
        XCTAssertTrue(edit.waitForExistence(timeout: 15), "the creator can open Cabal settings")
        edit.tap()
        let remove = element(app, "cabal-picture-remove-button")
        XCTAssertTrue(remove.waitForExistence(timeout: 15), "Cabal settings keeps Remove picture under its mark")
        screenshot(app, "settings")
        remove.tap()
        XCTAssertTrue(
            element(app, "cabal-picture-remove-confirm").waitForExistence(timeout: 10),
            "the button asks before it removes")
    }
}
