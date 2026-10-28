import XCTest

nonisolated final class ReactionsSampleUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    private func launch(_ scenario: String) -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = ["-reactionsHarness", scenario]
        app.launch()
        return app
    }

    @MainActor
    private func element(_ app: XCUIApplication, _ identifier: String) -> XCUIElement {
        app.descendants(matching: .any).matching(identifier: identifier).firstMatch
    }

    @MainActor
    private func first(_ app: XCUIApplication, prefix: String) -> XCUIElement {
        app.descendants(matching: .any).matching(NSPredicate(format: "identifier BEGINSWITH %@", prefix)).firstMatch
    }

    @MainActor
    private func screenshot(_ app: XCUIApplication, _ name: String) {
        let attachment = XCTAttachment(screenshot: app.screenshot())
        attachment.name = name
        attachment.lifetime = .keepAlways
        add(attachment)
    }

    @MainActor
    func testPickingFromTheTrayAddsAnOutlinedChip() {
        let app = launch("trayOpen")
        let party = element(app, "reaction-tray-1F389")
        XCTAssertTrue(party.waitForExistence(timeout: 15), "the tray is open")
        party.tap()
        let chip = app.buttons.matching(NSPredicate(format: "label BEGINSWITH 'party popper, 1 reaction'")).firstMatch
        XCTAssertTrue(chip.waitForExistence(timeout: 10), "a party popper chip joins the row")
        XCTAssertTrue(chip.label.contains("yours"), "the new chip is the viewer's, so outlined")
    }

    @MainActor
    func testThePickerOpensFromTheTraysPlusAndReactsWithAnEmojiFoundBySearch() {
        let app = launch("trayOpen")
        let more = first(app, prefix: "reaction-tray-more-")
        XCTAssertTrue(more.waitForExistence(timeout: 15), "the tray shows its plus")
        more.tap()
        let picker = element(app, "emoji-picker")
        XCTAssertTrue(picker.waitForExistence(timeout: 10), "the plus opens the picker")
        screenshot(app, "emoji-picker")

        let search = element(app, "emoji-search")
        search.tap()
        search.typeText("eyes")
        let eyes = element(app, "emoji-cell-1F440")
        XCTAssertTrue(eyes.waitForExistence(timeout: 10), "searching eyes finds the eyes")
        XCTAssertEqual(eyes.label, "eyes", "the cell reads its CLDR name")
        screenshot(app, "emoji-picker-search")
        eyes.tap()

        XCTAssertTrue(picker.waitForNonExistence(timeout: 10), "picking dismisses the sheet")
        let chip = app.buttons.matching(NSPredicate(format: "label BEGINSWITH 'eyes, 3 reactions'")).firstMatch
        XCTAssertTrue(chip.waitForExistence(timeout: 10), "the eyes chip count goes up")
    }

    @MainActor
    func testACategoryGlyphScrollsToItsSectionAndMovesTheWell() {
        let app = launch("picker")
        XCTAssertTrue(element(app, "emoji-picker").waitForExistence(timeout: 15))
        let nature = element(app, "emoji-category-nature")
        XCTAssertFalse(nature.isSelected, "the well starts on another category")
        nature.tap()
        let leaf = NSPredicate(format: "isSelected == true")
        expectation(for: leaf, evaluatedWith: nature)
        waitForExpectations(timeout: 10)
        XCTAssertTrue(
            app.staticTexts["Animals and nature"].waitForExistence(timeout: 10), "the nature section is on screen")
        screenshot(app, "emoji-picker-nature")
    }

    @MainActor
    func testEveryCellIsFortyFourSquare() {
        let app = launch("picker")
        let cell = element(app, "emoji-frequent-1F44D")
        XCTAssertTrue(cell.waitForExistence(timeout: 15))
        let scale = element(app, "emoji-picker").frame.width / app.frame.width
        XCTAssertGreaterThanOrEqual(cell.frame.width / scale, 44 - 0.5, "the cell is 44 wide in the sheet's own points")
        XCTAssertGreaterThanOrEqual(
            cell.frame.height / scale, 44 - 0.5, "the cell is 44 tall in the sheet's own points")
        XCTAssertEqual(cell.label, "thumbs up")
    }
}
