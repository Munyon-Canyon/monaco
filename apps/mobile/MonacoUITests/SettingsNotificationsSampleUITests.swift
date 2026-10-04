import XCTest

nonisolated final class SettingsNotificationsSampleUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testNotificationsRowOpensMonacosNotificationSettings() throws {
        let app = XCUIApplication()
        app.launchArguments = ["-settingsHarness", "off"]
        app.launch()

        let row = app.descendants(matching: .any).matching(identifier: "settings-notifications").firstMatch
        XCTAssertTrue(row.waitForExistence(timeout: 10), "the Settings list should show the Notifications row")
        row.tap()

        let preferences = XCUIApplication(bundleIdentifier: "com.apple.Preferences")
        XCTAssertTrue(preferences.wait(for: .runningForeground, timeout: 10), "the row should hand off to iOS Settings")
        let screenshot = XCTAttachment(screenshot: preferences.screenshot())
        screenshot.name = "ios-settings-after-notifications-tap"
        screenshot.lifetime = .keepAlways
        add(screenshot)

        #if targetEnvironment(simulator)
        let options = XCTExpectedFailure.Options()
        options.isStrict = false
        XCTExpectFailure(
            "the iOS 27 simulator opens Settings at its root for any app-settings URL (#2405)",
            options: options
        )
        continueAfterFailure = true
        #endif
        XCTAssertTrue(
            preferences.switches["Allow Notifications"].waitForExistence(timeout: 10),
            "iOS Settings should open on Monaco's notification page, not its root"
        )
    }
}
