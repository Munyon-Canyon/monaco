import XCTest

nonisolated final class ProfileOverviewJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    private func start() throws -> (XCUIApplication, ProfileOverviewJourney.Cabal) {
        let account = try JourneyAccount.load()
        let cabal = try ProfileOverviewJourney.Cabal.handedOff()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        return (app, cabal)
    }

    @MainActor
    func testS1Sections() throws {
        let (app, cabal) = try start()
        ProfileOverviewJourney.sections(app, cabal: cabal, recorder: ProfileOverviewJourney.recorder())
        attachScreenshot(of: app, named: "S1 Profile sections")
    }

    @MainActor
    func testS2PullToRefresh() throws {
        let (app, cabal) = try start()
        ProfileOverviewJourney.pullToRefresh(app, cabal: cabal, recorder: ProfileOverviewJourney.recorder())
        attachScreenshot(of: app, named: "S2 Profile after refresh")
    }

    @MainActor
    func testS3OpenCabal() throws {
        let (app, cabal) = try start()
        ProfileOverviewJourney.openCabal(app, cabal: cabal, recorder: ProfileOverviewJourney.recorder())
        attachScreenshot(of: app, named: "S3 cabal from Profile")
    }

    @MainActor
    func testS4OpenExplorer() throws {
        let (app, _) = try start()
        ProfileOverviewJourney.openExplorer(app, recorder: ProfileOverviewJourney.recorder())
        attachScreenshot(of: app, named: "S4 back from Solscan")
    }

    @MainActor
    func testS5SignOut() throws {
        let (app, _) = try start()
        ProfileOverviewJourney.signOut(app, recorder: ProfileOverviewJourney.recorder())
        attachScreenshot(of: app, named: "S5 signed out")
    }

    @MainActor
    func testS6Totals() throws {
        let (app, cabal) = try start()
        ProfileOverviewJourney.totals(app, cabal: cabal, recorder: ProfileOverviewJourney.recorder())
        attachScreenshot(of: app, named: "S6 totals")
    }
}
