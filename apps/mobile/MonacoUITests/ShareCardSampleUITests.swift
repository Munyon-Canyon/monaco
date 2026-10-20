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
    private func assertSystemShareSheet(_ app: XCUIApplication, _ message: String) {
        let sheet = element(app, "ActivityListView")
        XCTAssertTrue(sheet.waitForExistence(timeout: 30), message)
        XCTAssertFalse(element(app, "share-card-title").exists, "a Monaco sheet opened before the system sheet")
        attach(app.screenshot(), "system-share-sheet")
    }

    @MainActor
    func testEverySampleCardRenders() throws {
        let names = [
            "cabalGain": "share-card-cabal",
            "cabalLoss": "share-card-cabal-loss",
            "cabalShort": "share-card-cabal-short",
            "meGain": "share-card-me",
            "meNoCode": "share-card-me-no-code",
        ]
        for (scenario, name) in names.sorted(by: { $0.key < $1.key }) {
            let app = launch(["-MonacoShareCardSample", scenario])
            let card = element(app, "share-card-image")
            XCTAssertTrue(card.waitForExistence(timeout: 30), "\(scenario): no card")
            _ = XCTWaiter().wait(for: [XCTestExpectation()], timeout: 1.5)
            attach(card.screenshot(), name)
            app.terminate()
        }
    }

    @MainActor
    func testTheCabalScreenOpensTheSystemShareSheetInOneTap() throws {
        let app = launch(["-MonacoGroupDetailSample", "ready"])
        let button = app.buttons["cabal-share-button"]
        XCTAssertTrue(button.waitForExistence(timeout: 30), "no share button on a member's cabal")
        button.tap()
        assertSystemShareSheet(app, "the cabal Share did not open the system share sheet")
    }

    @MainActor
    func testTheCabalScreenOfANonMemberHasNoShareButton() throws {
        let app = launch(["-MonacoGroupDetailSample", "outsider"])

        XCTAssertTrue(element(app, "cabal-header-name").waitForExistence(timeout: 30), "the cabal never loaded")
        XCTAssertFalse(app.buttons["cabal-share-button"].exists, "a non-member can share the cabal")
    }

    @MainActor
    func testProfileOpensTheSystemShareSheetInOneTap() throws {
        let app = launch(["-MonacoProfileSample", "cabals"])
        let button = app.buttons["profile-share-button"]
        XCTAssertTrue(button.waitForExistence(timeout: 30), "no share button on Profile")
        attach(app.screenshot(), "share-profile-button")
        button.tap()
        assertSystemShareSheet(app, "the Profile Share did not open the system share sheet")
    }
}
