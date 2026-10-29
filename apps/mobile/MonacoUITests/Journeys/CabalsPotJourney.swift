import XCTest

enum CabalsPotJourney {
    static let id = "cabals/pot"
    static let version = 8

    static let screenTimeout: TimeInterval = 15

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func cabal(run: String) -> String { "QA slice \(run)" }

    static func potAndSlice(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S1.1", "open the cabal and find the pot") {
            app.openCabal(named: cabal(run: run))
            XCTAssertTrue(
                app.element("cabal-pot-value").waitForExistence(timeout: screenTimeout),
                "S1.1: the cabal shows no treasury value within 15 s"
            )
        }

        recorder.step("S1.2", "the treasury value reads $0.00") {
            let value = app.element("cabal-pot-value")
            XCTAssertTrue(
                JoinJourney.waitForLabel(value, containing: "$0.00", timeout: 10),
                "S1.2: no treasury value of $0.00 within 10 s"
            )
        }

        recorder.step("S1.3", "the stat block reads Your share $0, Of treasury 0%, Open votes 0, Bot Off") {
            let block = app.element("cabal-stat-block")
            XCTAssertTrue(block.waitForExistence(timeout: 10), "S1.3: no stat block within 10 s")
            for (label, value) in [("Your share", "$0"), ("Of treasury", "0%"), ("Open votes", "0"), ("Bot", "Off")] {
                let cell = block.descendants(matching: .any)
                    .matching(NSPredicate(format: "label == %@", "\(label), \(value)")).firstMatch
                XCTAssertTrue(
                    cell.waitForExistence(timeout: 10), "S1.3: no \(label) \(value) in the stat block")
            }
        }
    }

    static func shareGain(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S2.1", "open the cabal") {
            app.openCabal(named: cabal(run: run))
        }

        recorder.step("S2.2", "the share button opens the share screen") {
            let button = app.buttons["cabal-share-button"]
            XCTAssertTrue(button.waitForExistence(timeout: screenTimeout), "S2.2: no share button within 15 s")
            button.tap()
            XCTAssertTrue(
                app.element("share-screen").waitForExistence(timeout: screenTimeout),
                "S2.2: the share screen did not open within 15 s"
            )
        }

        recorder.step("S2.3", "back returns to the cabal") {
            app.tapBack()
            XCTAssertTrue(
                app.element("cabal-header-name").waitForExistence(timeout: 5),
                "S2.3: the cabal screen did not come back within 5 s"
            )
        }
    }

    static func holdings(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S3.1", "open the cabal") {
            app.openCabal(named: cabal(run: run))
            app.openCabalTab("Treasury")
        }

        recorder.step("S3.2", "an empty treasury says how to start") {
            let empty = app.element("cabal-holdings-empty")
            app.scrollIntoReach(empty)
            XCTAssertTrue(
                JoinJourney.waitForLabel(empty, containing: "Fund, then propose the first buy.", timeout: 10),
                "S3.2: no empty-treasury copy within 10 s"
            )
        }
    }
}
