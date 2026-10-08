import XCTest

nonisolated final class JoinJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let recorder = JoinJourney.recorder()
        let run = try JourneyRun.id()

        try session.scenario("S1") {
            try session.act(as: "A")
            JoinJourney.creatorOpensCabal(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S1-A-opened")

            let member = try session.act(as: "B")
            JoinJourney.memberRequests(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S1-B-requested")

            try session.act(as: "A")
            JoinJourney.creatorApproves(app, run: run, member: member.name, recorder: recorder)
            attachScreenshot(of: app, named: "S1-A-approved")

            try session.act(as: "B")
            JoinJourney.memberEntersAndJoinsOpen(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S1-B-joined-open")
        }

        try session.scenario("S2") {
            try session.act(as: "A")
            JoinJourney.creatorOpensTheCabal(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S2-A-opened-up")

            try session.act(as: "B")
            JoinJourney.memberJoinsTheOpenCabal(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S2-B-joined")
        }

        try session.scenario("S3") {
            try session.act(as: "B")
            JoinJourney.memberRequestsTheGate(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S3-B-requested")

            try session.act(as: "A")
            JoinJourney.creatorDeclines(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S3-A-declined")

            try session.act(as: "B")
            JoinJourney.memberAsksAgain(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S3-B-asked-again")
        }
    }
}
