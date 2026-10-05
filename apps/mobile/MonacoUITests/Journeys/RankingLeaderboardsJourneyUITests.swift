import XCTest

nonisolated final class RankingLeaderboardsJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    private func signedIn() throws -> (XCUIApplication, RankingLeaderboardsJourney.Seed) {
        let account = try JourneyAccount.load()
        let seed = try RankingLeaderboardsJourney.Seed.handedOff()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        return (app, seed)
    }

    @MainActor
    func testS1Phase1ATopInvestors() throws {
        let (app, seed) = try signedIn()
        RankingLeaderboardsJourney.topInvestors(app, seed: seed, recorder: RankingLeaderboardsJourney.recorder())
        attachScreenshot(of: app, named: "S1-A-top-investors")
    }

    @MainActor
    func testS2Phase1AMemberBoard() throws {
        let (app, seed) = try signedIn()
        RankingLeaderboardsJourney.memberBoard(app, seed: seed, recorder: RankingLeaderboardsJourney.recorder())
        attachScreenshot(of: app, named: "S2-A-member-board")
    }

    @MainActor
    func testS3Phase1ATopCabals() throws {
        let (app, _) = try signedIn()
        RankingLeaderboardsJourney.topCabals(app, recorder: RankingLeaderboardsJourney.recorder())
        attachScreenshot(of: app, named: "S3-A-top-cabals")
    }
}
