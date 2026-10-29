import XCTest

nonisolated final class CabalsInfoAndMembersJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let recorder = CabalsInfoAndMembersJourney.recorder()

        try session.scenario("S1") {
            let seed = try CabalsInfoAndMembersJourney.Seed.handedOff()
            try session.act(as: "A")
            CabalsInfoAndMembersJourney.membersTab(app, seed: seed, recorder: recorder)
            attachScreenshot(of: app, named: "S1-A-members")
        }

        try session.scenario("S2") {
            let seed = try CabalsInfoAndMembersJourney.Seed.handedOff()
            try session.act(as: "A")
            CabalsInfoAndMembersJourney.makeAVoter(app, seed: seed, recorder: recorder)
            attachScreenshot(of: app, named: "S2-A-voters-saved")
        }
    }
}
