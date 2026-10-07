import XCTest

enum CabalsPauseJourney {
    static let id = "cabals/pause"
    static let version = 2

    static func recorder() -> JourneyRecorder {
        JourneyRecorder(journey: id, version: version)
    }

    static func pausedCabal(run: String) -> String { "QA paused \(run)" }
    static func runningCabal(run: String) -> String { "QA running \(run)" }

    static func pausedShowsWhy(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S1.1", "open the paused cabal") {
            JoinJourney.openBySearch(app, pausedCabal(run: run), step: "S1.1")
        }

        recorder.step("S1.2", "the pause warning says when it resumes") {
            let banner = app.element("cabal-pause-banner")
            XCTAssertTrue(
                JoinJourney.waitForLabel(banner, containing: "Funding and cash outs resume after", timeout: 10),
                "S1.2: no pause warning within 10 s (known failure, #657)"
            )
        }
    }

    static func runningShowsNothing(_ app: XCUIApplication, run: String, recorder: JourneyRecorder) {
        recorder.step("S2.1", "open the running cabal") {
            JoinJourney.openBySearch(app, runningCabal(run: run), step: "S2.1")
        }

        recorder.step("S2.2", "there is no pause warning") {
            XCTAssertFalse(
                app.element("cabal-pause-banner").waitForExistence(timeout: 5),
                "S2.2: a cabal that is not paused shows a pause warning"
            )
        }
    }
}
