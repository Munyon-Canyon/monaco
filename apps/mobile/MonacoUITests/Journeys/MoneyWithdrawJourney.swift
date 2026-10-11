import XCTest

enum MoneyWithdrawJourney {
    static let id = "money/withdraw"
    static let version = 3

    static let screenTimeout: TimeInterval = 15
    static let badAddress = "not a solana address"
    static let badBaseAddress = "not a base address"
    static let baseAddress = "0x000000000000000000000000000000000000dEaD"
    static let confirmCopy = [
        "You're withdrawing", "To", "Network", "From", "Account balance", "Arrives", "About a minute",
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
            XCTAssertTrue(
                networkSegment(app, "Solana").isSelected, "S1.2: the network picker does not start on Solana")
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
            XCTAssertTrue(confirmShows(app, "Solana"), "S1.4: Confirm does not name Solana")
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

    static func networkSegment(_ app: XCUIApplication, _ name: String) -> XCUIElement {
        app.segmentedControls["withdraw-network-picker"].buttons[name]
    }

    static func confirmShows(_ app: XCUIApplication, _ text: String) -> Bool {
        app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", text)).firstMatch.exists
    }

    static func checkBase(_ app: XCUIApplication, recorder: JourneyRecorder) {
        let field = app.element("withdraw-address-field")
        let problem = app.element("withdraw-address-problem")
        let continueButton = app.buttons["withdraw-continue-button"]

        recorder.step("S2.1", "open Withdraw and pick Base") {
            app.tab("Home").tap()
            let link = app.element("home-withdraw-link")
            XCTAssertTrue(link.waitForExistence(timeout: screenTimeout), "S2.1: no Withdraw on Home")
            link.tap()
            XCTAssertTrue(
                app.navigationBars["Withdraw"].waitForExistence(timeout: screenTimeout),
                "S2.1: the Withdraw screen did not show within 15 s"
            )
            let helper = app.element("amount-entry-helper")
            XCTAssertTrue(
                JoinJourney.waitForLabel(helper, containing: "available", timeout: screenTimeout),
                "S2.1: no balance helper within 15 s"
            )
            XCTAssertFalse(helper.label.hasPrefix("$0.00"), "S2.1: the account is not funded: \(helper.label)")
            let base = networkSegment(app, "Base")
            XCTAssertTrue(base.waitForExistence(timeout: 5), "S2.1: no Base in the network picker")
            base.tap()
            XCTAssertTrue(
                app.staticTexts["A Base address that accepts USDC. Transfers can't be undone."]
                    .waitForExistence(timeout: 5),
                "S2.1: no Base caveat under the address"
            )
        }

        recorder.step("S2.2", "an address that isn't Base is refused inline") {
            field.tap()
            field.typeText(badBaseAddress)
            XCTAssertTrue(
                JoinJourney.waitForLabel(
                    problem, containing: "Check the address. It doesn't look like a Base address.", timeout: 5),
                "S2.2: no Base address error within 5 s: \(problem.label)"
            )
            XCTAssertFalse(continueButton.isEnabled, "S2.2: Continue is enabled for a bad Base address")
        }

        recorder.step("S2.3", "Max, a Base address and the estimate") {
            app.buttons["Max"].firstMatch.tap()
            field.tap()
            field.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: badBaseAddress.count))
            field.typeText(baseAddress)
            XCTAssertTrue(
                problem.waitForNonExistence(timeout: 5), "S2.3: the inline address error stayed for \(baseAddress)")
            let receive = app.element("withdraw-receive")
            XCTAssertTrue(
                JoinJourney.waitForLabel(receive, containing: "You'll receive ~$", timeout: screenTimeout),
                "S2.3: no estimate within \(Int(screenTimeout)) s: \(receive.label)"
            )
            XCTAssertTrue(continueButton.isEnabled, "S2.3: Continue is disabled with Max and a Base address")
        }

        recorder.step("S2.4", "Confirm names Base and B does not send") {
            continueButton.tap()
            XCTAssertTrue(
                app.navigationBars["Confirm"].waitForExistence(timeout: 10), "S2.4: Confirm did not show within 10 s")
            XCTAssertTrue(confirmShows(app, baseAddress), "S2.4: Confirm does not show the whole address")
            XCTAssertTrue(confirmShows(app, "Network"), "S2.4: Confirm has no Network row")
            XCTAssertTrue(confirmShows(app, "Base"), "S2.4: Confirm does not name Base")
            XCTAssertTrue(app.buttons["withdraw-confirm-button"].isEnabled, "S2.4: Withdraw is disabled")
            JoinJourney.snap(app, "S2-B-base-confirm")
        }
    }

    static func refundOnSolana(_ app: XCUIApplication, to address: String, recorder: JourneyRecorder) {
        recorder.step("S2.5", "back out and withdraw on Solana") {
            app.popToRoot()
            withdrawAll(app, to: address, recorder: MoneyWithdrawJourney.recorder())
        }
    }
}
