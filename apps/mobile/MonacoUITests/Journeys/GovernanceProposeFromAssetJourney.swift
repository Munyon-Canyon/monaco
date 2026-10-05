import XCTest

enum GovernanceProposeFromAssetJourney {
    static let id = "governance/propose-from-asset"
    static let version = 1

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func assetCabal(run: String) -> String { "QA asset \(run)" }

    static func proposeFromAsset(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        let name = assetCabal(run: run)
        let buy = GovernanceProposeBuyJourney.self

        recorder.step("S1.1", "open GOOGL from the Stocks tab") {
            app.tab("Stocks").tap()
            let field = app.textFields["Search Apple, Tesla, NVDA…"].firstMatch
            XCTAssertTrue(field.waitForExistence(timeout: 10), "S1.1: no stock search on the Stocks tab")
            field.tap()
            field.typeText("GOOGL")
            let row = app.descendants(matching: .any)
                .matching(NSPredicate(format: "identifier BEGINSWITH 'assets-row-GOOGL'")).firstMatch
            XCTAssertTrue(row.waitForExistence(timeout: 15), "S1.1: no GOOGL row within 15 s")
            row.tap()
            XCTAssertTrue(app.element("asset-detail-root").waitForExistence(timeout: 15), "S1.1: no asset screen")
            let propose = app.buttons["asset-detail-propose-buy"]
            XCTAssertTrue(propose.waitForExistence(timeout: 15), "S1.1: no Propose buy")
            XCTAssertEqual(propose.label, "Propose buy", "S1.1: the asset CTA")
            buy.expectText(app, "Your cabal votes before anything is bought", step: "S1.1")
        }

        recorder.step("S1.2", "pick a cabal") {
            app.buttons["asset-detail-propose-buy"].tap()
            buy.expectTitle(app, "Pick a cabal", step: "S1.2")
            buy.expectText(app, "Which cabal should buy GOOGL?", step: "S1.2")
        }

        recorder.step("S1.3", "pick the run's cabal") {
            let row = app.buttons.matching(NSPredicate(format: "label CONTAINS %@", name)).firstMatch
            app.scrollIntoReach(row)
            XCTAssertTrue(row.waitForExistence(timeout: 10), "S1.3: no \(name) in the picker")
            row.tap()
            buy.expectTitle(app, "Amount", step: "S1.3")
        }

        recorder.step("S1.4", "review $25 of GOOGL") {
            buy.review(app, chip: "$25", reads: "Buy $25.00 of GOOGL", step: "S1.4")
        }

        recorder.step("S1.5", "send to the cabal") {
            buy.send(app, to: name, step: "S1.5")
        }
    }
}
