import XCTest

nonisolated final class CabalsApproveRequestJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let run = try JourneyRun.id()
        let recorder = CabalsApproveRequestJourney.recorder()

        try session.scenario("S1") {
            let member = try session.act(as: "B")
            CabalsApproveRequestJourney.memberAsksAndCancels(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S1-B-requested")
            try session.act(as: "A")
            CabalsApproveRequestJourney.creatorDenies(app, run: run, member: member.name, recorder: recorder)
            attachScreenshot(of: app, named: "S1-A-denied")
        }

        try session.scenario("S2") {
            try session.act(as: "A")
            let member = try session.account("B")
            CabalsApproveRequestJourney.creatorApproves(app, run: run, member: member.name, recorder: recorder)
            attachScreenshot(of: app, named: "S2-A-approved")
        }
    }
}
