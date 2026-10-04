import XCTest

nonisolated final class AccountActivitySampleUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    private func launch(_ mode: String) -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = ["-accountActivityHarness", mode]
        app.launch()
        return app
    }

    @MainActor
    private func element(_ app: XCUIApplication, _ identifier: String) -> XCUIElement {
        app.descendants(matching: .any).matching(identifier: identifier).firstMatch
    }

    @MainActor
    private func row(_ app: XCUIApplication, containing text: String) -> XCUIElement {
        let predicate = NSPredicate(format: "identifier BEGINSWITH 'account-activity-row-' AND label CONTAINS %@", text)
        return app.buttons.matching(predicate).firstMatch
    }

    @MainActor
    private func screenshot(_ app: XCUIApplication, _ name: String) {
        let attachment = XCTAttachment(screenshot: app.screenshot())
        attachment.name = name
        attachment.lifetime = .keepAlways
        add(attachment)
    }

    @MainActor
    func testTheFullHarnessListsItsRowsAndLoadsTheSecondPageAtTheBottom() {
        let app = launch("full")
        XCTAssertTrue(row(app, containing: "Deposit").waitForExistence(timeout: 15), "the first page loads")
        XCTAssertTrue(app.navigationBars["Activity"].exists)
        XCTAssertTrue(row(app, containing: "+$25.00").exists, "a deposit is signed with a plus")
        XCTAssertTrue(row(app, containing: "\u{2212}$10.00").exists, "a fund is signed with a typographic minus")
        XCTAssertTrue(row(app, containing: "Pending").exists, "a pending row says so")
        screenshot(app, "account-activity-full")

        let secondPage = row(app, containing: "Funded a cabal")
        var swipes = 0
        while !secondPage.exists && swipes < 12 {
            app.swipeUp()
            swipes += 1
        }
        XCTAssertTrue(secondPage.waitForExistence(timeout: 10), "the second page loads at the bottom")
        screenshot(app, "account-activity-second-page")
    }

    @MainActor
    func testTappingARowOpensItsReceipt() {
        let app = launch("full")
        let deposit = row(app, containing: "Deposit")
        XCTAssertTrue(deposit.waitForExistence(timeout: 15))
        deposit.tap()

        let done = element(app, "account-txn-receipt-done")
        XCTAssertTrue(done.waitForExistence(timeout: 10), "the receipt opens with a Done button")
        XCTAssertEqual(element(app, "account-txn-receipt-amount").label, "+$25.00")
        XCTAssertTrue(element(app, "account-txn-receipt-status").label.contains("Done"))
        XCTAssertTrue(element(app, "account-txn-receipt-time").exists)
        XCTAssertTrue(element(app, "account-txn-receipt-solscan").exists, "a sent transaction links to Solscan")
        XCTAssertFalse(element(app, "account-txn-receipt-cabal").exists, "a deposit names no cabal")
        screenshot(app, "account-activity-receipt")

        done.tap()
        XCTAssertTrue(done.waitForNonExistence(timeout: 10), "Done closes the receipt")
    }

    @MainActor
    func testAPendingReceiptHasNoSolscanLink() {
        let app = launch("full")
        let pending = row(app, containing: "Pending")
        XCTAssertTrue(pending.waitForExistence(timeout: 15))
        pending.tap()

        XCTAssertTrue(element(app, "account-txn-receipt-done").waitForExistence(timeout: 10))
        XCTAssertTrue(element(app, "account-txn-receipt-status").label.contains("Pending"))
        XCTAssertFalse(element(app, "account-txn-receipt-solscan").exists, "nothing was sent yet")
        screenshot(app, "account-activity-receipt-pending")
    }

    @MainActor
    func testAFundReceiptOpensItsCabalAndClosesTheSheet() {
        let app = launch("full")
        let fund = row(app, containing: "Funded QA pot")
        XCTAssertTrue(fund.waitForExistence(timeout: 15))
        fund.tap()

        let cabal = element(app, "account-txn-receipt-cabal")
        XCTAssertTrue(cabal.waitForExistence(timeout: 10), "a fund names its cabal")
        XCTAssertTrue(cabal.label.contains("QA pot"))
        screenshot(app, "account-activity-receipt-cabal")
        cabal.tap()

        XCTAssertTrue(element(app, "account-txn-receipt-done").waitForNonExistence(timeout: 10), "the sheet closes")
        XCTAssertTrue(
            app.navigationBars.buttons["Activity"].waitForExistence(timeout: 10), "the cabal opens over Activity")
        screenshot(app, "account-activity-cabal-opened")
    }

    @MainActor
    func testTheEmptyHarnessSaysNoActivityYet() {
        let app = launch("empty")
        XCTAssertTrue(app.staticTexts["No activity yet"].waitForExistence(timeout: 15))
        XCTAssertTrue(app.staticTexts["Deposits, withdrawals and cabal moves show up here."].exists)
        screenshot(app, "account-activity-empty")
    }
}
