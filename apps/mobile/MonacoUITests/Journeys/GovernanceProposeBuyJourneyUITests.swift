import XCTest

nonisolated final class GovernanceProposeBuyJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let recorder = GovernanceProposeBuyJourney.recorder()
        let run = try JourneyRun.id()

        try session.scenario("S1") {
            let cabal = try JourneyHandoff.read("cabalName")
            try session.act(as: "A")
            GovernanceProposeBuyJourney.proposeBuy(app, cabal: cabal, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S1-A-proposal-sent")
        }

        try session.scenario("S2") {
            let proposer = try session.account("A")
            let voter = try session.account("B")
            try session.act(as: "B")
            GovernanceProposeBuyJourney.secondVoterVotes(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S2-B-voted-yes")
            try session.act(as: "A")
            GovernanceProposeBuyJourney.firstVoterPasses(
                app, proposer: proposer.name, voter: voter.name, recorder: recorder)
            attachScreenshot(of: app, named: "S2-A-buying")
        }
    }
}
