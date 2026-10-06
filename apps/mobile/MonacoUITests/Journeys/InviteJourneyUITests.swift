import XCTest

nonisolated final class InviteJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let recorder = InviteJourney.recorder()
        let run = try JourneyRun.id()

        try session.scenario("S1") {
            try session.act(as: "A")
            InviteJourney.acceptAndInvite(app, recorder: recorder)
            attachScreenshot(of: app, named: "S1-A-invited")

            try session.act(as: "B")
            InviteJourney.decline(app, recorder: recorder)
            attachScreenshot(of: app, named: "S1-B-declined")
        }

        try session.scenario("S2") {
            try session.act(as: "A")
            InviteJourney.copyAndShareTheCode(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S2-A-shared")
        }
    }
}
