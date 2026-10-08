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

        try session.scenario("S1") {
            try session.act(as: "A")
            InviteJourney.acceptAndInvite(app, recorder: recorder)
            attachScreenshot(of: app, named: "S1-A-invited")

            try session.act(as: "B")
            InviteJourney.decline(app, recorder: recorder)
            attachScreenshot(of: app, named: "S1-B-declined")
        }
    }
}
