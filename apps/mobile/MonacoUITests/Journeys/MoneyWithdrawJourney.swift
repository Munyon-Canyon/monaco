import XCTest

enum MoneyWithdrawJourney {
    static let id = "money/withdraw"
    static let version = 8

    static let screenTimeout: TimeInterval = 15
    static let badAddress = "not a solana address"
    static let badBaseAddress = "not a base address"
    static let baseAddress = "0x000000000000000000000000000000000000dEaD"

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

    static func openWithdraw(_ app: XCUIApplication, step: String) {
        app.popToRoot()
        app.tab("Profile").tap()
        let settings = app.element("profile-settings-row")
        app.scrollIntoReach(settings)
        XCTAssertTrue(settings.waitForExistence(timeout: screenTimeout), "\(step): no Settings on Profile")
        settings.tap()
        let row = app.element("settings-withdraw")
        XCTAssertTrue(row.waitForExistence(timeout: screenTimeout), "\(step): no Withdraw in Settings")
        row.tap()
        XCTAssertTrue(
            app.element("amount-keypad-screen").waitForExistence(timeout: screenTimeout),
            "\(step): the Withdraw keypad did not show within \(Int(screenTimeout)) s"
        )
    }

    static func expectFunded(_ app: XCUIApplication, step: String) {
        let source = app.element("amount-keypad-source")
        XCTAssertTrue(
            JoinJourney.waitForLabel(source, containing: "Cash $", timeout: screenTimeout),
            "\(step): the source pill does not read Cash $…: \(source.label)")
        XCTAssertFalse(source.label.contains("$0.00"), "\(step): the account is not funded: \(source.label)")
    }

    static func replaceAddress(_ app: XCUIApplication, with text: String, clearing old: String = "") {
        let ending = app.element("withdraw-address-ending")
        if ending.exists { ending.tap() }
        let field = app.element("withdraw-address-field")
        XCTAssertTrue(field.waitForExistence(timeout: 5), "no editable address field")
        field.tap()
        if !old.isEmpty {
            field.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: old.count))
        }
        field.typeText(text)
        app.dismissKeyboard()
    }

    static func tapMax(_ app: XCUIApplication, step: String) {
        let max = app.buttons["amount-keypad-preset-Max"]
        XCTAssertTrue(max.waitForExistence(timeout: 10), "\(step): no Max chip")
        max.tap()
    }

    static func withdrawAll(_ app: XCUIApplication, to address: String, recorder: JourneyRecorder) {
        withdraw(app, to: address, recorder: recorder, checksBadAddress: false, reopensAddress: false)
    }

    static func expectFeeLine(_ app: XCUIApplication, step: String) {
        let line = app.element("amount-keypad-line")
        XCTAssertTrue(
            JoinJourney.waitForLabel(line, containing: " fee · arrives in about ", timeout: screenTimeout),
            "\(step): the line does not read … fee · arrives in about …: \(line.label)")
        XCTAssertFalse(line.label.contains("≈"), "\(step): the line carries ≈: \(line.label)")
        XCTAssertFalse(line.label.contains("USDC"), "\(step): the line names USDC: \(line.label)")
    }

    static func withdraw(
        _ app: XCUIApplication, to address: String, recorder: JourneyRecorder, checksBadAddress: Bool,
        reopensAddress: Bool = true
    ) {
        let problem = app.element("withdraw-address-problem")

        recorder.step("S1.1", "open Withdraw from Settings") {
            openWithdraw(app, step: "S1.1")
            expectFunded(app, step: "S1.1")
            let field = app.element("withdraw-address-field")
            XCTAssertTrue(field.waitForExistence(timeout: 5), "S1.1: no address field")
            XCTAssertTrue(app.buttons["withdraw-paste"].exists, "S1.1: no paste")
            XCTAssertTrue(app.buttons["withdraw-scan"].exists, "S1.1: no scan")
            XCTAssertTrue(
                app.buttons["withdraw-network-solana"].isSelected, "S1.1: the Solana chip is not selected")
            XCTAssertTrue(app.buttons["withdraw-network-base"].exists, "S1.1: no Base chip")
            let chips = app.buttons.matching(NSPredicate(format: "identifier BEGINSWITH 'withdraw-network-'"))
            XCTAssertEqual(chips.count, 2, "S1.1: the network chips are not just Solana and Base: \(chips.count)")
        }

        if checksBadAddress {
            recorder.step("S1.2", "a bad address is refused inline") {
                replaceAddress(app, with: badAddress)
                XCTAssertTrue(problem.waitForExistence(timeout: 5), "S1.2: no inline address error within 5 s")
                XCTAssertFalse(app.element("amount-keypad-action").exists, "S1.2: a button shows for a bad address")
            }
        }

        recorder.step("S1.3", "Max and the agent wallet address") {
            replaceAddress(app, with: address, clearing: checksBadAddress ? badAddress : "")
            XCTAssertTrue(problem.waitForNonExistence(timeout: 5), "S1.3: the address error stayed for \(address)")
            tapMax(app, step: "S1.3")
            let ending = app.element("withdraw-address-ending")
            let expected = "Address ending " + String(address.suffix(4))
            XCTAssertTrue(
                JoinJourney.waitForLabel(ending, containing: expected, timeout: 5),
                "S1.3: the address row does not read \(expected): \(ending.label)")
            XCTAssertFalse(
                app.descendants(matching: .any).matching(NSPredicate(format: "label CONTAINS %@", address)).count > 0,
                "S1.3: the full address is on screen")
            expectFeeLine(app, step: "S1.3")
            XCTAssertNotEqual(
                app.element("amount-keypad-amount").value as? String, "$0", "S1.3: Max left the amount at $0")
        }

        if reopensAddress {
            recorder.step("S1.5", "the address row opens the full address") {
                let ending = app.element("withdraw-address-ending")
                XCTAssertTrue(ending.waitForExistence(timeout: 5), "S1.5: no address row")
                ending.tap()
                let field = app.element("withdraw-address-field")
                XCTAssertTrue(field.waitForExistence(timeout: 5), "S1.5: the row did not open the address field")
                XCTAssertEqual(field.value as? String, address, "S1.5: the field does not hold the full address")
                XCTAssertFalse(problem.exists, "S1.5: an address error shows for \(address)")
                app.dismissKeyboard()
                XCTAssertTrue(
                    JoinJourney.waitForLabel(ending, containing: "Address ending ", timeout: 5),
                    "S1.5: the row does not read Address ending … again")
            }
        }

        recorder.step("S1.4", "Withdraw sends and settles") {
            let withdraw = app.buttons["amount-keypad-action"]
            XCTAssertTrue(withdraw.waitForExistence(timeout: screenTimeout), "S1.4: no Withdraw button")
            XCTAssertTrue(withdraw.label.hasPrefix("Withdraw $"), "S1.4: the button reads \(withdraw.label)")
            withdraw.tap()
            let settled = app.element("amount-keypad-settled")
            XCTAssertTrue(settled.waitForExistence(timeout: 120), "S1.4: no settled button within 120 s")
            XCTAssertTrue(settled.label.contains(" to "), "S1.4: the settled button reads \(settled.label)")
            XCTAssertTrue(
                JoinJourney.waitForLabel(
                    app.element("amount-keypad-line"),
                    containing: "Sent just now · arrives on Solana in about a minute",
                    timeout: 5),
                "S1.4: the line does not read Sent just now: \(app.element("amount-keypad-line").label)")
            JoinJourney.snap(app, "S1-A-withdraw-settled")
        }
    }

    static func checkBase(_ app: XCUIApplication, recorder: JourneyRecorder) {
        let problem = app.element("withdraw-address-problem")

        recorder.step("S2.1", "open Withdraw and pick Base") {
            openWithdraw(app, step: "S2.1")
            expectFunded(app, step: "S2.1")
            let base = app.buttons["withdraw-network-base"]
            XCTAssertTrue(base.waitForExistence(timeout: 5), "S2.1: no Base chip")
            base.tap()
            XCTAssertTrue(base.isSelected, "S2.1: the Base chip is not selected")
            XCTAssertEqual(
                app.element("withdraw-address-field").placeholderValue, "USDC address on Base",
                "S2.1: the placeholder does not name Base")
            expectFeeLine(app, step: "S2.1")
        }

        recorder.step("S2.2", "an address that isn't Base is refused inline") {
            replaceAddress(app, with: badBaseAddress)
            XCTAssertTrue(
                JoinJourney.waitForLabel(
                    problem, containing: "Check the address. It doesn't look like a Base address.", timeout: 5),
                "S2.2: no Base address error within 5 s: \(problem.label)"
            )
            XCTAssertFalse(app.element("amount-keypad-action").exists, "S2.2: a button shows for a bad Base address")
        }

        recorder.step("S2.3", "Max, a Base address and the estimate") {
            replaceAddress(app, with: baseAddress, clearing: badBaseAddress)
            XCTAssertTrue(
                problem.waitForNonExistence(timeout: 5), "S2.3: the address error stayed for \(baseAddress)")
            tapMax(app, step: "S2.3")
            let ending = app.element("withdraw-address-ending")
            XCTAssertTrue(
                JoinJourney.waitForLabel(ending, containing: "Address ending dEaD", timeout: 5),
                "S2.3: the address row does not read Address ending dEaD: \(ending.label)")
            expectFeeLine(app, step: "S2.3")
            XCTAssertTrue(app.buttons["amount-keypad-action"].exists, "S2.3: no Withdraw button")
            JoinJourney.snap(app, "S2-B-base-keypad")
        }
    }

    static func refundOnSolana(_ app: XCUIApplication, to address: String, recorder: JourneyRecorder) {
        recorder.step("S2.4", "close without sending and withdraw on Solana") {
            app.buttons["amount-keypad-close"].tap()
            app.popToRoot()
            withdrawAll(app, to: address, recorder: MoneyWithdrawJourney.recorder())
        }
    }
}
