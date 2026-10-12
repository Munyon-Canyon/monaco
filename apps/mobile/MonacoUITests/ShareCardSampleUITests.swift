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
    func testEverySampleCardRendersInTheSheet() throws {
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
            attach(app.screenshot(), "\(name)-sheet")
            XCTAssertTrue(app.buttons["share-card-share"].exists, "\(scenario): no Share button")
            XCTAssertTrue(app.buttons["share-card-copy"].exists, "\(scenario): no Copy link button")
            app.terminate()
        }
    }

    @MainActor
    func testTheCabalScreenSharesItsGainAndCopiesTheLink() throws {
        let app = launch(["-MonacoGroupDetailSample", "populated"])
        let button = app.buttons["cabal-share-button"]
        XCTAssertTrue(button.waitForExistence(timeout: 30), "no share button on a member's cabal")
        button.tap()
        let shown = element(app, "share-card-image").waitForExistence(timeout: 30)
        attach(app.screenshot(), "share-cabal-sheet")
        let tree = XCTAttachment(string: app.debugDescription)
        tree.name = "tree"
        tree.lifetime = .keepAlways
        add(tree)
        XCTAssertTrue(shown, "no card in the sheet")

        app.buttons["share-card-copy"].tap()

        XCTAssertTrue(app.staticTexts["Link copied."].waitForExistence(timeout: 5), "no toast after Copy link")
        app.buttons["share-card-done"].tap()
        XCTAssertTrue(app.buttons["cabal-share-button"].waitForExistence(timeout: 5), "the sheet did not close")
    }

    @MainActor
    func testTheCabalScreenOfANonMemberHasNoShareButton() throws {
        let app = launch(["-MonacoGroupDetailSample", "outsider"])

        XCTAssertTrue(element(app, "cabal-header-name").waitForExistence(timeout: 30), "the cabal never loaded")
        XCTAssertFalse(app.buttons["cabal-share-button"].exists, "a non-member can share the cabal")
    }

    @MainActor
    func testHomeSharesTheReturnFromBesideThePortfolioChip() throws {
        let app = launch(["-MonacoHomeSample", "populated"])
        let button = app.buttons["home-share-button"]
        XCTAssertTrue(button.waitForExistence(timeout: 30), "no share button beside the portfolio chip")
        attach(app.screenshot(), "share-home-button")
        button.tap()
        XCTAssertTrue(element(app, "share-card-image").waitForExistence(timeout: 30), "no card in the sheet")
        attach(app.screenshot(), "share-home-sheet")
    }

    @MainActor
    func testProfileSharesTheReturnFromItsToolbar() throws {
        let app = launch(["-MonacoProfileSample", "cabals"])
        let button = app.buttons["profile-share-button"]
        XCTAssertTrue(button.waitForExistence(timeout: 30), "no share button on Profile")
        attach(app.screenshot(), "share-profile-button")
        button.tap()
        XCTAssertTrue(element(app, "share-card-image").waitForExistence(timeout: 30), "no card in the sheet")
        attach(app.screenshot(), "share-profile-sheet")
    }
}
