import XCTest

enum CabalsPotJourney {
    static let id = "cabals/pot"
    static let version = 6

    static let screenTimeout: TimeInterval = 15

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func cabal(run: String) -> String { "QA slice \(run)" }

    static func potAndSlice(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S1.1", "open the cabal and find the pot") {
            JoinJourney.openBySearch(app, cabal(run: run), step: "S1.1")
            XCTAssertTrue(
                app.element("cabal-pot-value").waitForExistence(timeout: screenTimeout),
                "S1.1: the cabal shows no pot value within 15 s"
            )
        }

        recorder.step("S1.2", "the pot value reads $0.00") {
            let value = app.element("cabal-pot-value")
            XCTAssertTrue(
                JoinJourney.waitForLabel(value, containing: "$0.00", timeout: 10),
                "S1.2: no pot value of $0.00 within 10 s"
            )
        }

        recorder.step("S1.3", "your slice reads $0.00 with the no-stake caption") {
            let value = app.element("cabal-slice-value")
            XCTAssertTrue(
                JoinJourney.waitForLabel(value, containing: "$0.00", timeout: 10),
                "S1.3: no slice value of $0.00 within 10 s"
            )
            XCTAssertTrue(app.staticTexts["Your share"].exists, "S1.3: no Your share label")
            XCTAssertTrue(app.element("cabal-slice-none").exists, "S1.3: the share is not the no-stake cell")
        }
    }

    static func shareGain(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S2.1", "open the cabal") {
            JoinJourney.openBySearch(app, cabal(run: run), step: "S2.1")
        }

        recorder.step("S2.2", "the share button shows the gain card") {
            let button = app.buttons["cabal-share-button"]
            XCTAssertTrue(button.waitForExistence(timeout: screenTimeout), "S2.2: no share button within 15 s")
            button.tap()
            let gain = app.element("cabal-invite-gain-row")
            XCTAssertTrue(gain.waitForExistence(timeout: screenTimeout), "S2.2: the invite sheet has no gain row")
            gain.tap()
            XCTAssertTrue(
                app.element("share-card-image").waitForExistence(timeout: screenTimeout),
                "S2.2: the share sheet drew no card within 15 s"
            )
        }

        recorder.step("S2.3", "copy the link") {
            app.buttons["share-card-copy"].tap()
            XCTAssertTrue(
                app.staticTexts["Link copied."].waitForExistence(timeout: 5),
                "S2.3: no 'Link copied.' toast within 5 s"
            )
        }

        recorder.step("S2.4", "close the sheet") {
            app.buttons["share-card-done"].tap()
            app.buttons["Close"].tap()
            XCTAssertTrue(
                app.element("cabal-header-name").waitForExistence(timeout: 5),
                "S2.4: the cabal screen did not come back within 5 s"
            )
        }
    }

    static func holdings(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S3.1", "open the cabal") {
            JoinJourney.openBySearch(app, cabal(run: run), step: "S3.1")
        }

        recorder.step("S3.2", "scroll to Holdings") {
            let header = app.staticTexts["Holdings"]
            app.scrollIntoReach(header)
            XCTAssertTrue(header.waitForExistence(timeout: 10), "S3.2: no Holdings section within 10 s")
        }

        recorder.step("S3.3", "a zero pot says how to start") {
            let empty = app.element("cabal-holdings-empty")
            app.scrollIntoReach(empty)
            XCTAssertTrue(
                JoinJourney.waitForLabel(empty, containing: "Fund, then propose the first buy.", timeout: 10),
                "S3.3: no zero-pot holdings copy within 10 s"
            )
        }
    }
}
