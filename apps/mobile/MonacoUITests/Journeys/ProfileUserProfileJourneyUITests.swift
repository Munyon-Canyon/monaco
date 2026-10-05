import XCTest

nonisolated final class ProfileUserProfileJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    private func start() throws -> (XCUIApplication, ProfileUserProfileJourney.Seed) {
        let account = try JourneyAccount.load()
        let seed = try ProfileUserProfileJourney.Seed.handedOff()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        return (app, seed)
    }

    @MainActor
    func testS1Header() throws {
        let (app, seed) = try start()
        ProfileUserProfileJourney.header(app, seed: seed, recorder: ProfileUserProfileJourney.recorder())
        attachScreenshot(of: app, named: "S1 member profile")
    }

    @MainActor
    func testS2FollowUnfollow() throws {
        let (app, seed) = try start()
        ProfileUserProfileJourney.follow(app, seed: seed, recorder: ProfileUserProfileJourney.recorder())
        attachScreenshot(of: app, named: "S2 after unfollow")
    }

    @MainActor
    func testS3SharedCabals() throws {
        let (app, seed) = try start()
        ProfileUserProfileJourney.sharedCabals(app, seed: seed, recorder: ProfileUserProfileJourney.recorder())
        attachScreenshot(of: app, named: "S3 shared cabals")
    }

    @MainActor
    func testS4ReportAndBlock() throws {
        let (app, seed) = try start()
        ProfileUserProfileJourney.reportAndBlock(app, seed: seed, recorder: ProfileUserProfileJourney.recorder())
        attachScreenshot(of: app, named: "S4 more menu")
    }

    @MainActor
    func testS5OwnRow() throws {
        let (app, seed) = try start()
        ProfileUserProfileJourney.ownRow(app, seed: seed, recorder: ProfileUserProfileJourney.recorder())
        attachScreenshot(of: app, named: "S5 own profile")
    }
}
