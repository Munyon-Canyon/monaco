import XCTest

nonisolated final class FeedBrowseJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testS1Phase1AChipsAndSearch() throws {
        let account = try JourneyAccount.load()
        let run = try JourneyRun.id()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        FeedBrowseJourney.chipsAndSearch(app, run: run, recorder: FeedBrowseJourney.recorder())
        attachScreenshot(of: app, named: "S1-A-search")
    }

    @MainActor
    func testS2Phase1AFollowing() throws {
        let account = try JourneyAccount.load()
        let run = try JourneyRun.id()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        FeedBrowseJourney.following(app, run: run, recorder: FeedBrowseJourney.recorder())
        attachScreenshot(of: app, named: "S2-A-following")
    }

    @MainActor
    func testS3Phase1ACellTaps() throws {
        let account = try JourneyAccount.load()
        let run = try JourneyRun.id()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        FeedBrowseJourney.cellTaps(app, run: run, recorder: FeedBrowseJourney.recorder())
        attachScreenshot(of: app, named: "S3-A-cells")
    }
}
