import XCTest

nonisolated final class CabalsDiscoverJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let recorder = CabalsDiscoverJourney.recorder()
        let run = try JourneyRun.id()

        try session.scenario("S1") {
            try session.act(as: "A")
            CabalsDiscoverJourney.emptyStates(app, recorder: recorder)
            attachScreenshot(of: app, named: "S1-A-discover-empty")
        }

        try session.scenario("S2") {
            try session.act(as: "A")
            CabalsDiscoverJourney.opensAndAsks(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S2-A-discover-requested")
        }
    }
}
