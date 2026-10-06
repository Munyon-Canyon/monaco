import XCTest

nonisolated final class ProfileOverviewJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let recorder = ProfileOverviewJourney.recorder()

        try session.scenario("S1") {
            let cabal = try ProfileOverviewJourney.Cabal.handedOff()
            try session.act(as: "A")
            ProfileOverviewJourney.sections(app, cabal: cabal, recorder: recorder)
            attachScreenshot(of: app, named: "S1 Profile sections")
        }

        try session.scenario("S2") {
            let cabal = try ProfileOverviewJourney.Cabal.handedOff()
            try session.act(as: "A")
            ProfileOverviewJourney.pullToRefresh(app, cabal: cabal, recorder: recorder)
            attachScreenshot(of: app, named: "S2 Profile after refresh")
        }

        try session.scenario("S3") {
            let cabal = try ProfileOverviewJourney.Cabal.handedOff()
            try session.act(as: "A")
            ProfileOverviewJourney.openCabal(app, cabal: cabal, recorder: recorder)
            attachScreenshot(of: app, named: "S3 cabal from Profile")
        }

        try session.scenario("S4") {
            try session.act(as: "A")
            ProfileOverviewJourney.openExplorer(app, recorder: recorder)
            attachScreenshot(of: app, named: "S4 back from Solscan")
        }

        try session.scenario("S5") {
            try session.act(as: "A")
            ProfileOverviewJourney.signOut(app, recorder: recorder)
            attachScreenshot(of: app, named: "S5 signed out")
        }

        try session.scenario("S6") {
            let cabal = try ProfileOverviewJourney.Cabal.handedOff()
            try session.act(as: "A")
            ProfileOverviewJourney.totals(app, cabal: cabal, recorder: recorder)
            attachScreenshot(of: app, named: "S6 totals")
        }
    }
}
