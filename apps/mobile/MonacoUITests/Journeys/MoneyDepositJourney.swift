import XCTest

enum MoneyDepositJourney {
    static let id = "money/deposit"
    static let version = 6

    static let screenTimeout: TimeInterval = 15

    static let evmArrival = "Arrives in a few minutes"

    static func howItWorks(on network: String) -> [String] {
        ["Send USDC on \(network)", "It lands in your balance", "Fund a cabal"]
    }

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func dollars(_ label: String) -> Decimal? {
        guard let range = label.range(of: #"\$[0-9,]+(\.[0-9]+)?"#, options: .regularExpression) else { return nil }
        return Decimal(string: label[range].dropFirst().replacingOccurrences(of: ",", with: ""))
    }

    static func balanceReading(_ element: XCUIElement) -> String {
        if let value = element.value as? String, dollars(value) != nil { return value }
        return element.label
    }

    static func waitForBalanceReading(
        _ element: XCUIElement, containing text: String, timeout: TimeInterval
    ) -> Bool {
        let deadline = Date().addingTimeInterval(timeout)
        repeat {
            if element.exists, balanceReading(element).contains(text) { return true }
            RunLoop.current.run(until: Date().addingTimeInterval(0.1))
        } while Date() < deadline
        return element.exists && balanceReading(element).contains(text)
    }

    static func waitForBalance(_ app: XCUIApplication, atLeast minimum: Decimal, timeout: TimeInterval) -> Bool {
        let value = app.element("platform-balance-value")
        let deadline = Date().addingTimeInterval(timeout)
        repeat {
            if value.exists, let amount = dollars(balanceReading(value)), amount >= minimum { return true }
            RunLoop.current.run(until: Date().addingTimeInterval(0.2))
        } while Date() < deadline
        return false
    }

    static func openAddMoney(_ app: XCUIApplication, step: String) {
        app.tab("Home").tap()
        let link = app.element("home-add-money-link")
        XCTAssertTrue(link.waitForExistence(timeout: screenTimeout), "\(step): no Deposit on Home")
        XCTAssertEqual(link.label, "Deposit", "\(step): the cash row's link reads \(link.label)")
        link.tap()
        XCTAssertTrue(
            app.element("deposit-sheet-title").waitForExistence(timeout: screenTimeout),
            "\(step): the Deposit sheet did not show within \(Int(screenTimeout)) s"
        )
    }

    static func openNetworks(_ app: XCUIApplication, step: String) {
        let crypto = app.element("deposit-crypto-row")
        XCTAssertTrue(crypto.waitForExistence(timeout: screenTimeout), "\(step): no Send crypto on Add money")
        crypto.tap()
        XCTAssertTrue(
            app.navigationBars["Choose a network"].waitForExistence(timeout: screenTimeout),
            "\(step): Choose a network did not show within \(Int(screenTimeout)) s"
        )
    }

    static func openAddress(_ app: XCUIApplication, network: String, name: String, step: String) {
        let row = app.element("deposit-network-\(network)")
        XCTAssertTrue(row.waitForExistence(timeout: screenTimeout), "\(step): no \(name) in the network list")
        row.tap()
        XCTAssertTrue(
            app.navigationBars["Receive USDC"].waitForExistence(timeout: screenTimeout),
            "\(step): Send USDC on \(name) did not show within \(Int(screenTimeout)) s"
        )
        XCTAssertTrue(
            app.element("deposit-address-value").waitForExistence(timeout: screenTimeout),
            "\(step): the \(name) deposit address did not show within \(Int(screenTimeout)) s"
        )
    }

    static func copy(_ app: XCUIApplication, step: String) {
        let copy = app.buttons["deposit-address-copy-button"]
        XCTAssertEqual(copy.label, "Copy address", "\(step): the copy button's label")
        copy.tap()
        XCTAssertTrue(
            app.staticTexts["Address copied."].firstMatch.waitForExistence(timeout: 5),
            "\(step): the toast \"Address copied.\" did not show within 5 s"
        )
    }

    static func expectWarning(_ app: XCUIApplication, on name: String, step: String) {
        let note = app.element("deposit-network-warning")
            .descendants(matching: .any)
            .matching(NSPredicate(format: "label != ''"))
            .firstMatch
        XCTAssertTrue(
            note.waitForExistence(timeout: screenTimeout),
            "\(step): the network note above the address did not show within \(Int(screenTimeout)) s"
        )
        XCTAssertEqual(
            note.label, "Only USDC on \(name)",
            "\(step): the network note above the address")
    }

    static func copySolana(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S1.1", "the runner's deposit is in the balance") {
            app.tab("Home").tap()
            XCTAssertTrue(
                waitForBalance(app, atLeast: 1, timeout: 30),
                "S1.1: the account balance is under $1.00 after 30 s: fund A first (\(balanceReading(app.element("platform-balance-value"))))"
            )
        }

        recorder.step("S1.2", "Deposit offers Card, then Send crypto") {
            openAddMoney(app, step: "S1.2")
            let card = app.element("deposit-card-row")
            let crypto = app.element("deposit-crypto-row")
            XCTAssertTrue(card.waitForExistence(timeout: screenTimeout), "S1.2: no Card on Add money")
            XCTAssertTrue(crypto.waitForExistence(timeout: screenTimeout), "S1.2: no Send crypto on Add money")
            XCTAssertEqual(card.label, "Card or Apple Pay", "S1.2: the card row's label")
            XCTAssertEqual(crypto.label, "Send crypto", "S1.2: the crypto row's label")
            XCTAssertLessThan(card.frame.minY, crypto.frame.minY, "S1.2: Card is not above Send crypto")
            XCTAssertTrue(app.element("deposit-screen-balance-value").exists, "S1.2: no balance on Deposit")
        }

        recorder.step("S1.3", "Send crypto lists Solana first, then Base") {
            openNetworks(app, step: "S1.3")
            let solana = app.element("deposit-network-solana")
            let base = app.element("deposit-network-base")
            XCTAssertTrue(solana.waitForExistence(timeout: screenTimeout), "S1.3: no Solana in the network list")
            XCTAssertTrue(base.waitForExistence(timeout: screenTimeout), "S1.3: no Base in the network list")
            XCTAssertLessThan(solana.frame.minY, base.frame.minY, "S1.3: Solana is not first")
            JoinJourney.snap(app, "S1-A-networks")
        }

        recorder.step("S1.4", "open the Solana address") {
            openAddress(app, network: "solana", name: "Solana", step: "S1.4")
            XCTAssertTrue(app.staticTexts["Your deposit address"].exists, "S1.4: no Your deposit address")
            XCTAssertFalse(app.element("deposit-arrival-note").exists, "S1.4: Solana shows an arrival note")
        }

        recorder.step("S1.5", "copy the Solana address") {
            copy(app, step: "S1.5")
            expectWarning(app, on: "Solana", step: "S1.5")
            JoinJourney.snap(app, "S1-A-address-copied")
        }

        recorder.step("S1.6", "read How it works") {
            let steps = howItWorks(on: "Solana")
            let last = app.element("deposit-how-it-works")
            app.scrollIntoReach(last)
            XCTAssertTrue(app.staticTexts["How it works"].exists, "S1.6: no How it works header")
            for (index, line) in steps.enumerated() {
                XCTAssertTrue(
                    GovernanceProposeBuyJourney.labelled(app, containing: "Step \(index + 1): \(line)")
                        .waitForExistence(timeout: 10),
                    "S1.6: no step \"\(line)\"")
            }
        }
    }

    static func copyBase(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S2.1", "open the network list again") {
            app.popToRoot()
            openAddMoney(app, step: "S2.1")
            openNetworks(app, step: "S2.1")
            XCTAssertTrue(
                app.element("deposit-network-base").waitForExistence(timeout: screenTimeout),
                "S2.1: no Base in the network list")
        }

        recorder.step("S2.2", "open the Base address") {
            openAddress(app, network: "base", name: "Base", step: "S2.2")
            let address = app.element("deposit-address-value").label
            XCTAssertTrue(address.lowercased().hasPrefix("0x"), "S2.2: the Base address does not start 0x: \(address)")
            expectWarning(app, on: "Base", step: "S2.2")
            XCTAssertEqual(app.element("deposit-arrival-note").label, evmArrival, "S2.2: the arrival note")
            JoinJourney.snap(app, "S2-A-base-address")
        }

        recorder.step("S2.3", "copy the Base address") {
            copy(app, step: "S2.3")
        }
    }

    static func openFromProfile(_ app: XCUIApplication, recorder: JourneyRecorder) {
        recorder.step("S3.1", "Profile's docked Deposit opens the sheet") {
            app.popToRoot()
            app.tab("Profile").tap()
            let dock = app.element("profile-add-money-link")
            XCTAssertTrue(dock.waitForExistence(timeout: screenTimeout), "S3.1: no docked Deposit on Profile")
            XCTAssertEqual(dock.label, "Deposit", "S3.1: the dock reads \(dock.label)")
            dock.tap()
            XCTAssertTrue(
                app.element("deposit-sheet-title").waitForExistence(timeout: screenTimeout),
                "S3.1: the Deposit sheet did not show within \(Int(screenTimeout)) s"
            )
            XCTAssertTrue(app.element("deposit-crypto-row").exists, "S3.1: no Send crypto on the sheet")
        }
    }
}
