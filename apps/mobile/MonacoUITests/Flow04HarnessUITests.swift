import XCTest

nonisolated final class Flow04HarnessUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testHoldingSharesToastsTheServerMessageAndOffersCashOut() {
        let app = leave(["-MonacoFlow", "04", "leaveHoldsShares"])

        expectToast(app, "Cash out your share of the pot before you leave this cabal.")
        XCTAssertTrue(app.buttons["cabalLeaveCashOutButton"].waitForExistence(timeout: 5))
        attachScreenshot(app, "flow-04-leave-holds-shares")
    }

    @MainActor
    func testEveryOtherRefusalToastsItsMessageAndStays() {
        let refusals = [
            "leaveLastMemberPotNotEmpty": "The pot still holds money, so the last member cannot leave yet.",
            "leaveCreatorWithMembers": "The creator cannot leave while other members remain.",
            "notCabalMember": "You are not a member of this cabal.",
            "priceUnavailable": "Prices are temporarily unavailable. Try again in a moment.",
            "unauthorized": "Please sign in again.",
            "interrupted": "You're offline. Try again.",
        ]
        for (scenario, message) in refusals.sorted(by: { $0.key < $1.key }) {
            let app = leave(["-MonacoFlow", "04", scenario])

            expectToast(app, message)
            XCTAssertTrue(app.buttons["cabalLeaveButton"].exists, "\(scenario) should stay on the screen")
            XCTAssertFalse(app.buttons["cabalLeaveCashOutButton"].exists, "\(scenario) should not offer cash out")
            attachScreenshot(app, "flow-04-\(scenario)")
            app.terminate()
        }
    }

    @MainActor
    func testAMemberLeavesWithASuccessToast() {
        let app = leave(["-MonacoCabalLeaveSample", "member"])

        expectToast(app, "You left QA pot.")
        attachScreenshot(app, "cabal-leave-member")
    }

    @MainActor
    func testTheCreatorWithMembersSeesWhyThereIsNoLeave() {
        let app = XCUIApplication()
        app.launchArguments = ["-MonacoCabalLeaveSample", "creator"]
        app.launch()

        let note = app.staticTexts["cabalLeaveCreatorNote"]
        XCTAssertTrue(note.waitForExistence(timeout: 10))
        XCTAssertEqual(note.label, "You can leave once everyone else has left.")
        XCTAssertFalse(app.buttons["cabalLeaveButton"].exists)
        attachScreenshot(app, "cabal-leave-creator")
    }

    @MainActor
    private func leave(_ arguments: [String]) -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = arguments
        app.launch()
        let button = app.buttons["cabalLeaveButton"]
        XCTAssertTrue(button.waitForExistence(timeout: 10), "\(arguments) did not open")
        button.tap()
        let confirm = app.sheets.buttons["Leave cabal"].firstMatch
        XCTAssertTrue(confirm.waitForExistence(timeout: 5), "the leave confirmation did not show")
        confirm.tap()
        return app
    }

    @MainActor
    private func expectToast(_ app: XCUIApplication, _ message: String) {
        let toast = app.descendants(matching: .any).matching(identifier: "monaco-toast-banner").firstMatch
        XCTAssertTrue(toast.waitForExistence(timeout: 10), "no toast for \(message)")
        XCTAssertTrue(toast.label.contains(message), toast.label)
    }

    @MainActor
    private func attachScreenshot(_ app: XCUIApplication, _ name: String) {
        let attachment = XCTAttachment(screenshot: app.screenshot())
        attachment.name = name
        attachment.lifetime = .keepAlways
        add(attachment)
    }
}
