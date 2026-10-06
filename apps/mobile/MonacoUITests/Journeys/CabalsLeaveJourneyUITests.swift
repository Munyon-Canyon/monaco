import XCTest

nonisolated final class CabalsLeaveJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let run = try JourneyRun.id()
        let recorder = CabalsLeaveJourney.recorder()

        try session.scenario("S1") {
            try session.act(as: "B")
            CabalsLeaveJourney.memberLeaves(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S1-B-left")
        }

        try session.scenario("S2") {
            try session.act(as: "A")
            CabalsLeaveJourney.creatorStays(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S2-A-creator-note")
        }

        try session.scenario("S3") {
            try session.act(as: "B")
            CabalsLeaveJourney.memberSellsAndLeaves(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S3-B-leave-dialog")
        }
    }
}
