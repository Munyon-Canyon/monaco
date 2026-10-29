import XCTest

nonisolated final class GifSampleUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    private func launch(_ scenario: String) -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = ["-gifHarness", scenario]
        app.launch()
        return app
    }

    @MainActor
    private func element(_ app: XCUIApplication, _ identifier: String) -> XCUIElement {
        app.descendants(matching: .any).matching(identifier: identifier).firstMatch
    }

    @MainActor
    func testAGifPostedFromTheSheetShowsAsABubbleThatOpensFullScreen() {
        let app = launch("bubble")
        XCTAssertTrue(element(app, "gif-bubble-g1").waitForExistence(timeout: 15), "no seeded GIF reply")
        let button = app.buttons["comment-composer-gif"]
        XCTAssertTrue(button.waitForExistence(timeout: 15), "no GIF button")
        button.tap()
        let cell = element(app, "gif-cell-sample-gif-3")
        XCTAssertTrue(cell.waitForExistence(timeout: 15), "no GIFs in the sheet")
        cell.tap()

        let bubble = element(app, "gif-bubble-c9")
        XCTAssertTrue(bubble.waitForExistence(timeout: 15), "the posted GIF did not draw as a bubble")
        XCTAssertEqual(bubble.label, "GIF: Sample GIF 3")

        bubble.tap()
        let viewer = element(app, "media-viewer")
        XCTAssertTrue(viewer.waitForExistence(timeout: 10), "the bubble did not open the viewer")
        app.buttons["media-viewer-close"].tap()
        XCTAssertTrue(viewer.waitForNonExistence(timeout: 10), "the close button left the viewer open")
    }

    @MainActor
    func testSwipingDownClosesTheViewer() {
        let app = launch("viewer")
        let viewer = element(app, "media-viewer")
        XCTAssertTrue(viewer.waitForExistence(timeout: 15), "no viewer")
        let start = app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.3))
        let end = app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.9))
        start.press(forDuration: 0.1, thenDragTo: end)
        XCTAssertTrue(viewer.waitForNonExistence(timeout: 10), "the swipe left the viewer open")
    }

    @MainActor
    func testReduceMotionShowsAPlayControlOnTheBubble() {
        let app = launch("reduceMotion")
        XCTAssertTrue(element(app, "gif-bubble-g1").waitForExistence(timeout: 15), "no GIF reply")
        let play = app.buttons["gif-play-g1"]
        XCTAssertTrue(play.waitForExistence(timeout: 10), "no play control with Reduce Motion on")
        play.tap()
        XCTAssertTrue(play.waitForNonExistence(timeout: 10), "the play control stayed after it ran the GIF")
    }
}
