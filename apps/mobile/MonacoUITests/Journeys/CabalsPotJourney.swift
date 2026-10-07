import XCTest

enum CabalsPotJourney {
    static let id = "cabals/pot"
    static let version = 2

    static let screenTimeout: TimeInterval = 15

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func cabal(run: String) -> String { "QA slice \(run)" }

    static func potAndSlice(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S1.1", "open the cabal and find the pot") {
            JoinJourney.openBySearch(app, cabal(run: run), step: "S1.1")
            XCTAssertTrue(
                app.staticTexts["In the pot"].waitForExistence(timeout: screenTimeout),
                "S1.1: the hero has no In the pot within 15 s"
            )
        }

        recorder.step("S1.2", "the pot value reads $0.00") {
            let value = app.element("cabal-pot-value")
            XCTAssertTrue(
                JoinJourney.waitForLabel(value, containing: "$0.00", timeout: 10),
                "S1.2: no pot value of $0.00 within 10 s (known failure, #2136 #2137)"
            )
        }

        recorder.step("S1.3", "your slice reads $0.00 with the no-stake caption") {
            let value = app.element("cabal-slice-value")
            XCTAssertTrue(
                JoinJourney.waitForLabel(value, containing: "$0.00", timeout: 10),
                "S1.3: no slice value of $0.00 within 10 s (known failure, #2136 #2137)"
            )
            XCTAssertTrue(app.staticTexts["Your slice"].exists, "S1.3: no Your slice header")
            XCTAssertTrue(app.staticTexts["Fund to get a slice"].exists, "S1.3: no Fund to get a slice")
        }
    }

    static func holdings(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S2.1", "open the cabal") {
            JoinJourney.openBySearch(app, cabal(run: run), step: "S2.1")
        }

        recorder.step("S2.2", "scroll to Holdings") {
            let header = app.staticTexts["Holdings"]
            app.scrollIntoReach(header)
            XCTAssertTrue(header.waitForExistence(timeout: 10), "S2.2: no Holdings section within 10 s")
        }

        recorder.step("S2.3", "a zero pot says how to start") {
            let empty = app.element("cabal-holdings-empty")
            app.scrollIntoReach(empty)
            XCTAssertTrue(
                JoinJourney.waitForLabel(empty, containing: "Fund, then propose the first buy.", timeout: 10),
                "S2.3: no zero-pot holdings copy within 10 s (known failure, #2136 #2137)"
            )
        }

        recorder.step("S2.4", "the Cash row opens its asset") {
            let cash = app.element("cabal-holdings-cash")
            app.scrollIntoReach(cash)
            XCTAssertTrue(cash.waitForExistence(timeout: 10), "S2.4: no Cash row within 10 s (known failure, #2136)")
            cash.tap()
            XCTAssertTrue(
                app.navigationBars.staticTexts["Cash"].waitForExistence(timeout: 10),
                "S2.4: the Cash asset screen did not show within 10 s"
            )
        }
    }
}
