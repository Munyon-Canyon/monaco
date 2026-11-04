import XCTest

nonisolated final class FeedReactAndReplyJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let recorder = FeedReactAndReplyJourney.recorder()
        let run = try JourneyRun.id()

        try session.scenario("S1") {
            try session.act(as: "A")
            let item = try JourneyHandoff.read("sale_item")
            FeedReactAndReplyJourney.reactAndReplyFromCard(app, item: item, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S1-A-replied")
        }

        try session.scenario("S2") {
            let item = try JourneyHandoff.read("sale_item")
            FeedReactAndReplyJourney.postPage(app, item: item, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S2-A-post-page")
        }

        try session.scenario("S3") {
            let item = try JourneyHandoff.read("public_item")
            let proposal = try JourneyHandoff.read("public_proposal")
            FeedReactAndReplyJourney.publicPost(app, item: item, proposal: proposal, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S3-A-public-post")
        }

        try session.scenario("S4") {
            let item = try JourneyHandoff.read("sale_item")
            FeedReactAndReplyJourney.reportPost(app, item: item, recorder: recorder)
            attachScreenshot(of: app, named: "S4-A-reported-post")
        }

        try session.scenario("S5") {
            let item = try JourneyHandoff.read("vote_item")
            let proposal = try JourneyHandoff.read("vote_proposal")
            FeedReactAndReplyJourney.voteFromFeed(app, item: item, proposal: proposal, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S5-A-voted-from-feed")
        }
    }
}
