import XCTest

nonisolated final class CabalClosedSampleUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    private func launch(_ state: String, dark: Bool) -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = [
            "-cabalClosedHarness", state, "-AppleInterfaceStyle", dark ? "Dark" : "Light",
            "-UIPreferredContentSizeCategoryName", "UICTContentSizeCategoryAccessibilityXL",
        ]
        app.launch()
        return app
    }

    @MainActor
    private func screenshot(_ app: XCUIApplication, _ name: String) {
        let attachment = XCTAttachment(screenshot: app.screenshot())
        attachment.name = name
        attachment.lifetime = .keepAlways
        add(attachment)
    }

    @MainActor
    func testABannedCabalShowsTheWarningRowInLightAndDark() {
        for dark in [false, true] {
            let app = launch("banned", dark: dark)
            let row = app.descendants(matching: .any).matching(identifier: "cabal-closed-row").firstMatch
            XCTAssertTrue(row.waitForExistence(timeout: 15))
            screenshot(app, "cabal-closed-\(dark ? "dark" : "light")")
            app.terminate()
        }
    }

    @MainActor
    func testAnActiveCabalShowsNothing() {
        let app = launch("active", dark: false)
        XCTAssertFalse(
            app.descendants(matching: .any).matching(identifier: "cabal-closed-row").firstMatch.waitForExistence(
                timeout: 3))
    }
}
