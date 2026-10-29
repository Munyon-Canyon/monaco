import XCTest

nonisolated final class ShareCardSampleUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    private func element(_ app: XCUIApplication, _ identifier: String) -> XCUIElement {
        app.descendants(matching: .any).matching(identifier: identifier).firstMatch
    }

    @MainActor
    private func attach(_ image: XCUIScreenshot, _ name: String) {
        let attachment = XCTAttachment(screenshot: image)
        attachment.name = name
        attachment.lifetime = .keepAlways
        add(attachment)
    }

    @MainActor
    private func launch(_ arguments: [String]) -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = arguments
        app.launch()
        return app
    }

    @MainActor
    func testEverySampleCardRenders() throws {
        for scenario in ["stake", "portfolio"] {
            let app = launch(["-MonacoShareSample", scenario])
            let card = element(app, "share-card")
            XCTAssertTrue(card.waitForExistence(timeout: 30), "\(scenario): no card")
            _ = XCTWaiter().wait(for: [XCTestExpectation()], timeout: 1.5)
            attach(card.screenshot(), "share-card-\(scenario)")
            app.terminate()
        }
    }

    @MainActor
    func testTheCabalScreenOpensTheShareScreenInOneTap() throws {
        let app = launch(["-MonacoGroupDetailSample", "ready"])
        let button = app.buttons["cabal-share-button"]
        XCTAssertTrue(button.waitForExistence(timeout: 30), "no share button on a member's cabal")
        button.tap()
        let screen = element(app, "share-screen")
        XCTAssertTrue(screen.waitForExistence(timeout: 30), "the cabal Share did not open the share screen")
        attach(app.screenshot(), "cabal-share-screen")
    }

    @MainActor
    func testTheCabalScreenOfANonMemberHasNoShareButton() throws {
        let app = launch(["-MonacoGroupDetailSample", "outsider"])

        XCTAssertTrue(element(app, "cabal-header-name").waitForExistence(timeout: 30), "the cabal never loaded")
        XCTAssertFalse(app.buttons["cabal-share-button"].exists, "a non-member can share the cabal")
    }

    @MainActor
    func testProfileOpensTheShareScreenInOneTap() throws {
        let app = launch(["-MonacoProfileSample", "ready"])
        let button = app.buttons["profile-share-button"]
        XCTAssertTrue(button.waitForExistence(timeout: 30), "no share button on Profile")
        attach(app.screenshot(), "share-profile-button")
        button.tap()
        let screen = element(app, "share-screen")
        XCTAssertTrue(screen.waitForExistence(timeout: 30), "the Profile Share did not open the share screen")
    }
}
