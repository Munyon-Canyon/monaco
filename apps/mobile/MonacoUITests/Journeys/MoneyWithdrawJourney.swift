import XCTest

enum MoneyWithdrawJourney {
    static let id = "money/withdraw"
    static let version = 2

    static let screenTimeout: TimeInterval = 15
    static let badAddress = "not a solana address"
    static let confirmCopy = [
        "You're withdrawing", "To", "From", "Account balance", "Arrives", "About a minute",
        "Double-check the address. Transfers can't be undone.",
    ]

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func refundAddress(environment: [String: String] = ProcessInfo.processInfo.environment) throws -> String {
        guard let address = environment["MONACO_QA_REFUND_ADDRESS"], !address.isEmpty else {
            XCTFail("no {QA.refund_address}: scripts/qa/journey.py passes MONACO_QA_REFUND_ADDRESS to a funds journey")
            throw CocoaError(.keyValueValidation)
        }
        return address
    }

    static func withdrawAll(_ app: XCUIApplication, to address: String, recorder: JourneyRecorder) {
        let field = app.element("withdraw-address-field")
        let problem = app.element("withdraw-address-problem")
        let continueButton = app.buttons["withdraw-continue-button"]

        recorder.step("S1.1", "open Withdraw from Home") {
            app.tab("Home").tap()
            let link = app.element("home-withdraw-link")
            XCTAssertTrue(link.waitForExistence(timeout: screenTimeout), "S1.1: no Withdraw on Home")
            link.tap()
            XCTAssertTrue(
                app.navigationBars["Withdraw"].waitForExistence(timeout: screenTimeout),
                "S1.1: the Withdraw screen did not show within 15 s"
            )
            let helper = app.element("amount-entry-helper")
            XCTAssertTrue(
                JoinJourney.waitForLabel(helper, containing: "available", timeout: screenTimeout),
                "S1.1: no balance helper within 15 s"
            )
            XCTAssertFalse(helper.label.hasPrefix("$0.00"), "S1.1: the account is not funded: \(helper.label)")
        }

        recorder.step("S1.2", "a bad address is refused inline") {
            field.tap()
            field.typeText(badAddress)
            XCTAssertTrue(problem.waitForExistence(timeout: 5), "S1.2: no inline address error within 5 s")
            XCTAssertTrue(
                app.staticTexts["A Solana address that accepts USDC. Transfers can't be undone."].exists,
                "S1.2: no caveat under the address"
            )
            XCTAssertFalse(continueButton.isEnabled, "S1.2: Continue is enabled for a bad address")
        }

        recorder.step("S1.3", "Max and the agent wallet address") {
            app.buttons["Max"].firstMatch.tap()
            field.tap()
            field.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: badAddress.count))
            field.typeText(address)
            XCTAssertTrue(
                problem.waitForNonExistence(timeout: 5), "S1.3: the inline address error stayed for \(address)")
            XCTAssertTrue(continueButton.isEnabled, "S1.3: Continue is disabled with Max and a good address")
        }

        recorder.step("S1.4", "Confirm shows the withdrawal") {
            continueButton.tap()
            XCTAssertTrue(
                app.navigationBars["Confirm"].waitForExistence(timeout: 10), "S1.4: Confirm did not show within 10 s")
            for text in confirmCopy {
                XCTAssertTrue(
                    app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", text)).firstMatch.exists,
                    "S1.4: Confirm has no \"\(text)\""
                )
            }
            XCTAssertTrue(
                app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", address)).firstMatch.exists,
                "S1.4: Confirm does not show the whole address \(address)"
            )
            JoinJourney.snap(app, "S1-A-withdraw-confirm")
        }

        recorder.step("S1.5", "Withdraw sends and toasts") {
            let withdraw = app.buttons["withdraw-confirm-button"]
            XCTAssertTrue(withdraw.isEnabled, "S1.5: Withdraw is disabled")
            withdraw.tap()
            let toast = app.staticTexts.matching(
                NSPredicate(format: "label BEGINSWITH 'Withdrawing $' AND label ENDSWITH 'It lands in about a minute.'")
            ).firstMatch
            XCTAssertTrue(toast.waitForExistence(timeout: 10), "S1.5: no Withdrawing toast within 10 s")
        }
    }
}
