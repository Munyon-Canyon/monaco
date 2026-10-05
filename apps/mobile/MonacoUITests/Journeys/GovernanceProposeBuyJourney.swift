import XCTest

enum GovernanceProposeBuyJourney {
    static let id = "governance/propose-buy"
    static let version = 1

    static let screenTimeout: TimeInterval = 15

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func buyCabal(run: String) -> String { "QA buy \(run)" }

    static func expectTitle(_ app: XCUIApplication, _ title: String, step: String) {
        XCTAssertTrue(
            app.navigationBars[title].waitForExistence(timeout: 10),
            "\(step): no screen titled \(title) within 10 s (known failure while #613 is open)"
        )
    }

    static func expectText(_ app: XCUIApplication, _ text: String, step: String) {
        XCTAssertTrue(
            app.staticTexts[text].firstMatch.waitForExistence(timeout: 10),
            "\(step): no \"\(text)\" within 10 s"
        )
    }

    static func expectTexts(_ app: XCUIApplication, _ texts: [String], step: String) {
        for text in texts {
            expectText(app, text, step: step)
        }
    }

    static func tapLabel(_ app: XCUIApplication, _ label: String, step: String) {
        let target = app.buttons[label].firstMatch
        XCTAssertTrue(target.waitForExistence(timeout: 10), "\(step): no \(label) to tap within 10 s")
        target.tap()
    }

    static func openCabalAsVoter(_ app: XCUIApplication, _ name: String, step: String) {
        JoinJourney.openBySearch(app, name, step: step)
        let propose = app.buttons["cabal-action-propose"]
        app.scrollIntoReach(propose)
        XCTAssertTrue(propose.waitForExistence(timeout: screenTimeout), "\(step): no Propose action")
        XCTAssertTrue(propose.isEnabled, "\(step): Propose is disabled for a voter")
        XCTAssertFalse(
            app.element("cabal-action-propose-caption").exists,
            "\(step): a voter sees Only voters can propose"
        )
    }

    static func openChooser(_ app: XCUIApplication, step: String) {
        app.buttons["cabal-action-propose"].tap()
        expectTitle(app, "Propose", step: step)
    }

    static func review(_ app: XCUIApplication, chip: String, reads summary: String, step: String) {
        tapLabel(app, chip, step: step)
        tapLabel(app, "Review", step: step)
        expectTitle(app, "Review", step: step)
        expectText(app, summary, step: step)
    }

    static func send(_ app: XCUIApplication, to cabal: String, step: String) {
        tapLabel(app, "Send to cabal", step: step)
        JoinJourney.waitForToast(app, "Proposal sent to \(cabal)", step: step)
    }

    static func proposeBuy(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        let name = buyCabal(run: run)

        recorder.step("S1.1", "open the cabal as a voter") {
            openCabalAsVoter(app, name, step: "S1.1")
        }

        recorder.step("S1.2", "open the Propose chooser") {
            openChooser(app, step: "S1.2")
            expectTexts(app, ["Buy a stock", "Your cabal votes on it first"], step: "S1.2")
        }

        recorder.step("S1.3", "pick Buy a stock") {
            tapLabel(app, "Buy a stock", step: "S1.3")
            expectTitle(app, "Buy", step: "S1.3")
            expectText(app, "Popular", step: "S1.3")
        }

        recorder.step("S1.4", "pick GOOGL") {
            let field = app.textFields["Search Apple, Tesla, NVDA…"].firstMatch
            XCTAssertTrue(field.waitForExistence(timeout: 10), "S1.4: no stock search")
            field.tap()
            field.typeText("GOOGL")
            let row = app.buttons.matching(NSPredicate(format: "label CONTAINS 'GOOGL'")).firstMatch
            XCTAssertTrue(row.waitForExistence(timeout: 10), "S1.4: no GOOGL row within 10 s")
            row.tap()
            expectTitle(app, "Amount", step: "S1.4")
            for chip in ["$25", "$50", "$100", "Max"] {
                XCTAssertTrue(app.buttons[chip].exists, "S1.4: no \(chip) chip")
            }
        }

        recorder.step("S1.5", "review $25 of GOOGL") {
            review(app, chip: "$25", reads: "Buy $25.00 of GOOGL", step: "S1.5")
            expectTexts(app, ["Cabal gets", "Price", "Pot", "Who votes"], step: "S1.5")
        }

        recorder.step("S1.6", "send to the cabal") {
            send(app, to: name, step: "S1.6")
            XCTAssertTrue(
                JoinJourney.waitForLabel(app.element("cabal-header-name"), containing: name, timeout: 10),
                "S1.6: the flow did not close back to the cabal screen"
            )
        }
    }
}
