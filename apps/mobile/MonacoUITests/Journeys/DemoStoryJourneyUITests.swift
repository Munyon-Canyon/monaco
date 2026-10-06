import XCTest

nonisolated final class DemoStoryJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let recorder = DemoStoryJourney.recorder()
        let run = try JourneyRun.id()

        try session.scenario("S1") {
            let account = try session.act(as: "A")
            let code = try DemoStoryJourney.startAndShareTheCode(app, as: account, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S1-A-invite-code")

            try session.act(as: "B")
            DemoStoryJourney.friendJoins(app, run: run, code: code, recorder: recorder)
            attachScreenshot(of: app, named: "S1-B-joined")
        }

        try session.scenario("S2") {
            try session.act(as: "B")
            DemoStoryJourney.fundThePot(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S2-B-funded")
        }

        try session.scenario("S3") {
            try session.act(as: "B")
            DemoStoryJourney.browseStocks(app, recorder: recorder)
            attachScreenshot(of: app, named: "S3-B-pre-ipo")
        }

        try session.scenario("S4") {
            try session.act(as: "B")
            DemoStoryJourney.proposeABuy(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S4-B-proposed")

            try session.act(as: "A")
            DemoStoryJourney.voteAndBuy(app, recorder: recorder)
            attachScreenshot(of: app, named: "S4-A-bought")
        }

        try session.scenario("S5") {
            try session.act(as: "B")
            DemoStoryJourney.chatFirst(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S5-B-chat")

            try session.act(as: "A")
            DemoStoryJourney.chatReply(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S5-A-chat")
        }

        try session.scenario("S6") {
            try session.act(as: "A")
            DemoStoryJourney.addABot(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S6-A-bot-proposed")

            try session.act(as: "B")
            DemoStoryJourney.connectTheBot(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S6-B-bot")
        }

        try session.scenario("S7") {
            try session.act(as: "A")
            DemoStoryJourney.cashOut(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S7-A-cash-out")
        }

        try session.scenario("S8") {
            try session.act(as: "A")
            let friend = try session.account("B")
            DemoStoryJourney.seeWhosUp(app, friend: friend.name, recorder: recorder)
            attachScreenshot(of: app, named: "S8-A-board")
        }
    }
}
