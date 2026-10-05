import XCTest

enum MoneyDepositJourney {
    static let id = "money/deposit"
    static let version = 1

    static let screenTimeout: TimeInterval = 15

    static let howItWorks = [
        "Send USDC to the address above from an exchange or another app.",
        "Your account balance updates a few seconds after it arrives.",
        "Fund a cabal to move it into the pot and grow your slice.",
    ]

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func dollars(_ label: String) -> Decimal? {
        guard let range = label.range(of: #"\$[0-9,]+(\.[0-9]+)?"#, options: .regularExpression) else { return nil }
        return Decimal(string: label[range].dropFirst().replacingOccurrences(of: ",", with: ""))
    }

    static func waitForBalance(_ app: XCUIApplication, atLeast minimum: Decimal, timeout: TimeInterval) -> Bool {
        let value = app.element("platform-balance-value")
        let deadline = Date().addingTimeInterval(timeout)
        repeat {
            if value.exists, let amount = dollars(value.label), amount >= minimum { return true }
            RunLoop.current.run(until: Date().addingTimeInterval(0.2))
        } while Date() < deadline
        return false
    }

    static func openAddMoney(_ app: XCUIApplication, step: String) {
        app.tab("Home").tap()
        let link = app.element("home-add-money-link")
        XCTAssertTrue(link.waitForExistence(timeout: screenTimeout), "\(step): no Add money on Home")
        link.tap()
        XCTAssertTrue(
            app.navigationBars["Add money"].waitForExistence(timeout: screenTimeout),
            "\(step): the Add money screen did not show within \(Int(screenTimeout)) s"
        )
    }

    static func copyAddress(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S1.1", "the runner's deposit is in the balance") {
            app.tab("Home").tap()
            XCTAssertTrue(
                waitForBalance(app, atLeast: 1, timeout: 30),
                "S1.1: the account balance is under $1.00 after 30 s: fund A first (\(app.element("platform-balance-value").label))"
            )
        }

        recorder.step("S1.2", "open Add money") {
            openAddMoney(app, step: "S1.2")
            let crypto = app.buttons["Crypto"]
            XCTAssertTrue(crypto.waitForExistence(timeout: 10), "S1.2: no Crypto on Add money")
            crypto.tap()
            XCTAssertTrue(
                app.staticTexts["Your deposit address"].waitForExistence(timeout: screenTimeout),
                "S1.2: no Your deposit address")
            XCTAssertTrue(app.staticTexts["Your deposit address"].exists, "S1.2: no Your deposit address")
            XCTAssertTrue(
                app.element("deposit-address-value").waitForExistence(timeout: screenTimeout),
                "S1.2: the deposit address did not show within \(Int(screenTimeout)) s"
            )
        }

        recorder.step("S1.3", "copy the address") {
            let copy = app.buttons["deposit-address-copy-button"]
            XCTAssertEqual(copy.label, "Copy address", "S1.3: the copy button's label")
            copy.tap()
            XCTAssertTrue(
                app.staticTexts["Address copied."].firstMatch.waitForExistence(timeout: 5),
                "S1.3: the toast \"Address copied.\" did not show within 5 s"
            )
            XCTAssertTrue(
                app.staticTexts["Only send USDC on Solana to this address."].exists,
                "S1.3: no network note under the address"
            )
            JoinJourney.snap(app, "S1-A-address-copied")
        }

        recorder.step("S1.4", "read How it works") {
            let last = app.staticTexts[howItWorks[2]]
            app.scrollIntoReach(last)
            XCTAssertTrue(app.staticTexts["How it works"].exists, "S1.4: no How it works header")
            for line in howItWorks {
                XCTAssertTrue(app.staticTexts[line].waitForExistence(timeout: 10), "S1.4: no \"\(line)\"")
            }
            XCTAssertTrue(app.element("deposit-screen-balance-value").exists, "S1.4: no balance row on Add money")
        }
    }

    static func chooseMethod(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S2.1", "Add money offers Card and Crypto") {
            openAddMoney(app, step: "S2.1")
            XCTAssertTrue(
                app.buttons["Card"].waitForExistence(timeout: 10) && app.buttons["Crypto"].exists,
                "S2.1: Add money has no Card and Crypto chooser"
            )
        }
    }
}
