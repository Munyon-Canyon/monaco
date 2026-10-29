import XCTest

nonisolated final class PublicPostSampleUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testAPublicPostThreadIsReadOnlyWithTheMembersLine() {
        let app = XCUIApplication()
        app.launchArguments = ["-commentThreadHarness", "pilates"]
        app.launch()
        let any = { (id: String) in app.descendants(matching: .any).matching(identifier: id).firstMatch }

        XCTAssertTrue(any("comment-row-pilates-zoe").waitForExistence(timeout: 15), "the replies are drawn")
        XCTAssertTrue(any("comment-members-only").waitForExistence(timeout: 5), "the members line is drawn")
        XCTAssertTrue(any("comment-members-only").label.contains("Members of Pilates reply here"))
        XCTAssertFalse(any("comment-composer-field").exists, "there is no composer")
        let replyButtons = app.buttons.matching(NSPredicate(format: "identifier BEGINSWITH 'comment-reply-'"))
        XCTAssertEqual(replyButtons.count, 0, "no reply is offered")
    }
}
