import XCTest

nonisolated final class CreateCabalJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let recorder = CreateCabalJourney.recorder()
        let run = try JourneyRun.id()

        try session.scenario("S1") {
            try session.act(as: "A")
            CreateCabalJourney.openForm(app, recorder: recorder)
            attachScreenshot(of: app, named: "S1-create-form")
        }

        try session.scenario("S2") {
            try session.act(as: "A")
            let name = CreateCabalJourney.uniqueName()
            CreateCabalJourney.openForm(app, recorder: recorder)
            try JourneyHandoff.write("cabalName", name)
            CreateCabalJourney.create(app, named: name, recorder: recorder)
            attachScreenshot(of: app, named: "S2-cabals-list")
        }

        try session.scenario("S3") {
            try session.act(as: "A")
            CreateCabalJourney.openForm(app, recorder: recorder)
            CreateCabalJourney.blankNameStaysDisabled(app, recorder: recorder)
            attachScreenshot(of: app, named: "S3-blank-name")
        }

        try session.scenario("S4") {
            try session.act(as: "A")
            try CreateCabalJourney.creatorStartsAnOpenCabal(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S4-A-created")

            let friend = try session.act(as: "B")
            CreateCabalJourney.friendRequestsToJoin(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S4-B-joined")

            try session.act(as: "A")
            CreateCabalJourney.creatorSeesTheFriend(app, run: run, friend: friend.name, recorder: recorder)
            attachScreenshot(of: app, named: "S4-A-member-board")
        }
    }
}
