import XCTest

enum CabalsPotJourney {
    static let id = "cabals/pot"
    static let version = 11

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

        recorder.step("S1.3", "Your share reads $0 and 0%, with no bot wording in the band") {
            let share = app.element("cabal-share-value")
            XCTAssertTrue(share.waitForExistence(timeout: 10), "S1.3: no Your share within 10 s")
            for text in ["Your share", "$0", "0 percent"] {
                XCTAssertTrue(
                    JoinJourney.waitForLabel(share, containing: text, timeout: 10),
                    "S1.3: Your share reads '\(share.label)', not \(text)")
            }
            XCTAssertFalse(
                share.label.localizedCaseInsensitiveContains("bot"), "S1.3: Your share reads '\(share.label)'")
            XCTAssertFalse(
                app.element("cabal-pot-value").label.localizedCaseInsensitiveContains("bot"),
                "S1.3: the Treasury line mentions the bot")
            let bot = app.staticTexts.matching(NSPredicate(format: "label BEGINSWITH[c] %@", "Bot")).firstMatch
            XCTAssertFalse(bot.exists, "S1.3: the band shows a bot line: '\(bot.label)'")
        }
    }

    static func shareGain(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S2.1", "open the cabal") {
            app.popToRoot()
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

        recorder.step("S2.4", "Copy image says Image copied.") {
            let button = app.buttons["cabal-share-button"]
            XCTAssertTrue(button.waitForExistence(timeout: screenTimeout), "S2.4: no share button within 15 s")
            button.tap()
            XCTAssertTrue(
                app.element("share-screen").waitForExistence(timeout: screenTimeout),
                "S2.4: the share screen did not open within 15 s"
            )
            for (id, title) in [("pnl", "Show $"), ("trades", "Show trades"), ("code", "Show code")] {
                let chip = app.element("share-eye-\(id)")
                XCTAssertTrue(
                    JoinJourney.waitForLabel(chip, containing: title, timeout: 5), "S2.4: the dock shows no \(title)")
            }
            let send = app.buttons["share-send"]
            XCTAssertTrue(send.waitForExistence(timeout: 5), "S2.4: no Share button in the dock")
            let copy = app.buttons["share-copy-image"]
            XCTAssertTrue(copy.waitForExistence(timeout: 5), "S2.4: no Copy image button in the dock")
            copy.tap()
            JoinJourney.waitForToast(app, "Image copied.", step: "S2.4")
            XCTAssertTrue(send.exists, "S2.4: Share is gone after Copy image")
        }

        recorder.step("S2.5", "back returns to the cabal") {
            app.tapBack()
            XCTAssertTrue(
                app.element("cabal-header-name").waitForExistence(timeout: 5),
                "S2.5: the cabal screen did not come back within 5 s"
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
