import XCTest

nonisolated final class CabalInvitesSampleUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testEachInviteShowsTheCabalAndWhoInvited() {
        let app = launch()
        let rows = app.descendants(matching: .any).matching(identifier: "cabal-invite-row")
        XCTAssertEqual(rows.count, 2, "the inbox lists both sample invites")
        XCTAssertTrue(app.staticTexts["Friday Fund"].exists, "the first invite names its cabal")
        XCTAssertTrue(app.staticTexts["@kaicenat invited you"].exists, "the first invite names who invited")
        XCTAssertTrue(app.staticTexts["Long Haul"].exists, "the second invite names its cabal")
        XCTAssertTrue(app.staticTexts["Mara invited you"].exists, "the second invite names who invited")
        XCTAssertFalse(app.staticTexts["4 members"].exists, "an invite row shows the inviter only, no member count")
    }

    @MainActor
    func testDeclineRemovesTheRowAndToasts() {
        let app = launch()
        app.buttons.matching(identifier: "cabal-invite-decline").firstMatch.tap()
        XCTAssertTrue(
            app.staticTexts["Invite declined."].waitForExistence(timeout: 5), "declining toasts Invite declined.")
        let gone = NSPredicate(format: "exists == false")
        expectation(for: gone, evaluatedWith: app.staticTexts["Friday Fund"])
        waitForExpectations(timeout: 5)
        XCTAssertTrue(app.staticTexts["Long Haul"].exists, "the other invite stays")
    }

    @MainActor
    func testAcceptToastsYoureIn() {
        let app = launch()
        app.buttons.matching(identifier: "cabal-invite-accept").firstMatch.tap()
        XCTAssertTrue(app.staticTexts["You're in."].waitForExistence(timeout: 5), "accepting toasts You're in.")
    }

    @MainActor
    private func launch() -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = ["-MonacoCabalInvitesSample"]
        app.launch()
        XCTAssertTrue(
            app.descendants(matching: .any).matching(identifier: "cabal-invite-row").firstMatch
                .waitForExistence(timeout: 10),
            "the invites harness did not open"
        )
        return app
    }
}
