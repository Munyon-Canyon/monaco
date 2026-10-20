import XCTest

nonisolated final class HomeCabalsSampleUITests: XCTestCase {
    private let bigBallers = "0f1d6a52-0001-7000-8000-000000000001"
    private let techCrew = "0f1d6a52-0001-7000-8000-000000000002"
    private let underwater = "0f1d6a52-0001-7000-8000-000000000003"

    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    private func launch(_ scenario: String) -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = ["-MonacoHomeSample", scenario]
        app.launch()
        return app
    }

    @MainActor
    private func anyElement(_ app: XCUIApplication, _ identifier: String) -> XCUIElement {
        app.descendants(matching: .any).matching(identifier: identifier).firstMatch
    }

    @MainActor
    private func attachScreenshot(_ app: XCUIApplication, name: String) {
        let attachment = XCTAttachment(screenshot: app.screenshot())
        attachment.name = name
        attachment.lifetime = .keepAlways
        add(attachment)
    }

    @MainActor
    private func reveal(_ app: XCUIApplication, _ identifier: String) -> XCUIElement {
        let element = anyElement(app, identifier)
        for _ in 0..<6 where !isOnScreen(app, element) {
            app.swipeUp()
        }
        return element
    }

    @MainActor
    private func isOnScreen(_ app: XCUIApplication, _ element: XCUIElement) -> Bool {
        guard element.exists else { return false }
        let frame = element.frame
        guard frame.width > 1, frame.height > 1, !frame.isNull, !frame.isInfinite else { return false }
        return app.frame.contains(CGPoint(x: frame.midX, y: frame.midY))
    }

    @MainActor
    private func revealInRail(_ app: XCUIApplication, _ id: String, from anchor: String) -> XCUIElement {
        let card = anyElement(app, "cabal-row-\(id)")
        let first = anyElement(app, "cabal-row-\(anchor)")
        for _ in 0..<4 where !isOnScreen(app, card) {
            if first.exists { first.swipeLeft() } else { app.swipeUp() }
        }
        return card
    }

    @MainActor
    func testAMemberWhoFundedNoneSeesEveryCabalNotTheEmptyState() throws {
        let app = launch("notFunded")

        XCTAssertTrue(
            reveal(app, "cabal-row-\(underwater)").waitForExistence(timeout: 20), "no Home card for a member cabal")
        for (id, anchor) in [(techCrew, underwater), (bigBallers, techCrew)] {
            XCTAssertTrue(
                revealInRail(app, id, from: anchor).waitForExistence(timeout: 10),
                "no Home card for the cabal \(id)")
        }
        XCTAssertFalse(anyElement(app, "home-cabals-empty").exists, "the empty state shows for a member")
        XCTAssertTrue(app.staticTexts["Not funded"].firstMatch.exists, "no \"Not funded\" on an unfunded card")
        attachScreenshot(app, name: "home-not-funded")
    }

    @MainActor
    func testAFundedCabalKeepsItsReturnBesideUnfundedOnes() throws {
        let app = launch("partlyFunded")

        let funded = reveal(app, "cabal-row-\(bigBallers)")
        XCTAssertTrue(funded.waitForExistence(timeout: 20), "no card for the funded cabal")
        XCTAssertTrue(
            revealInRail(app, techCrew, from: bigBallers).waitForExistence(timeout: 10),
            "no card for the unfunded cabal")
        XCTAssertTrue(app.staticTexts["Not funded"].firstMatch.exists, "no \"Not funded\" on the unfunded card")
        attachScreenshot(app, name: "home-partly-funded")
    }

    @MainActor
    func testAUserWithNoCabalsStillSeesStartWithYourPeople() throws {
        let app = launch("empty")

        XCTAssertTrue(
            reveal(app, "home-cabals-empty").waitForExistence(timeout: 20),
            "no \"Start with your people\" for no cabals")
    }
}
