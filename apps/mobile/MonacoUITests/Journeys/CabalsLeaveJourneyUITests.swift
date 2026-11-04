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
            let creator = try session.account("A").name
            CabalsLeaveJourney.memberLeaves(app, run: run, creator: creator, recorder: recorder)
            attachScreenshot(of: app, named: "S1-B-left")
        }

        try session.scenario("S2") {
            try session.act(as: "A")
            let member = try session.account("B").name
            CabalsLeaveJourney.creatorStays(app, run: run, member: member, recorder: recorder)
            attachScreenshot(of: app, named: "S2-A-creator-note")
        }
    }
}
