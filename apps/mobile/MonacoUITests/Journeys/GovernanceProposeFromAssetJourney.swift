import XCTest

enum GovernanceProposeFromAssetJourney {
    static let id = "governance/propose-from-asset"
    static let version = 10

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func assetCabal(run: String) -> String { "QA asset \(run)" }

    static func proposeFromAsset(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        let name = assetCabal(run: run)
        let buy = GovernanceProposeBuyJourney.self

        recorder.step("S1.1", "open GOOGL from the Markets tab") {
            app.element("tab-assets").tap()
            XCTAssertTrue(
                app.element("assets-root").waitForExistence(timeout: 15), "S1.1: the Markets tab did not open")
            let field = StocksBrowseJourney.searchField(app)
            XCTAssertTrue(field.waitForExistence(timeout: 10), "S1.1: no stock search on the Markets tab")
            field.tap()
            field.typeText("GOOGL")
            let row = app.descendants(matching: .any)
                .matching(NSPredicate(format: "identifier BEGINSWITH 'assets-row-GOOGL'")).firstMatch
            XCTAssertTrue(row.waitForExistence(timeout: 15), "S1.1: no GOOGL row within 15 s")
            row.tap()
            XCTAssertTrue(app.element("asset-detail-root").waitForExistence(timeout: 15), "S1.1: no asset screen")
            let propose = app.buttons["asset-detail-propose-buy"]
            XCTAssertTrue(propose.waitForExistence(timeout: 15), "S1.1: no Propose buy")
            app.scrollIntoReach(propose)
            XCTAssertEqual(propose.label, "Propose buy", "S1.1: the asset CTA")
        }

        recorder.step("S1.2", "pick a cabal") {
            app.scrollIntoReach(app.buttons["asset-detail-propose-buy"])
            app.buttons["asset-detail-propose-buy"].tap()
            let sheet = app.element("propose-pick-cabal-sheet")
            XCTAssertTrue(sheet.waitForExistence(timeout: 10), "S1.2: no cabal picker sheet within 10 s")
            XCTAssertEqual(sheet.label, "Choose a cabal", "S1.2: the picker sheet title")
            buy.expectText(app, "Which cabal should buy GOOGL?", step: "S1.2")
        }

        recorder.step("S1.3", "pick the run's cabal") {
            let row = app.buttons.matching(NSPredicate(format: "label CONTAINS %@", name)).firstMatch
            app.scrollIntoReach(row)
            XCTAssertTrue(row.waitForExistence(timeout: 10), "S1.3: no \(name) in the picker")
            row.tap()
            XCTAssertTrue(app.element("propose-amount-screen").waitForExistence(timeout: 10), "S1.3: no keypad")
            let source = app.element("amount-keypad-source")
            XCTAssertTrue(source.waitForExistence(timeout: 10), "S1.3: no keypad source")
            XCTAssertTrue(source.label.contains(name), "S1.3: the source reads \(source.label), not \(name)")
            for chip in ["$25", "$50", "$100", "Max"] {
                XCTAssertTrue(
                    app.element("amount-keypad-preset-\(chip)").waitForExistence(timeout: 10),
                    "S1.3: no \(chip) chip on the keypad")
            }
            XCTAssertFalse(app.element("monaco-search-field").exists, "S1.3: Amount asks for a stock again")
        }

        recorder.step("S1.4", "an amount over the empty pot is refused") {
            app.element("amount-keypad-5").tap()
            XCTAssertTrue(
                app.element("amount-keypad-limit-line").waitForExistence(timeout: 10),
                "S1.4: no limit line within 10 s")
            XCTAssertFalse(app.element("amount-keypad-action").exists, "S1.4: an action shows over the pot")
        }
    }
}
