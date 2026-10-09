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
        let refundAddress = try MoneyWithdrawJourney.refundAddress()

        try session.scenario("S1") {
            let cabal = try JourneyHandoff.read("cabalName")
            try session.act(as: "A")
            GovernanceProposeBuyJourney.fillThePotAsA(app, cabal: cabal, recorder: recorder)
            attachScreenshot(of: app, named: "S1-A-funded")
            try session.act(as: "B")
            GovernanceProposeBuyJourney.fillThePotAsB(app, cabal: cabal, recorder: recorder)
            attachScreenshot(of: app, named: "S1-B-funded")
        }

        try session.scenario("S2") {
            let cabal = try JourneyHandoff.read("cabalName")
            try session.act(as: "A")
            GovernanceProposeBuyJourney.proposeBuy(app, cabal: cabal, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S2-A-proposal-sent")
        }

        try session.scenario("S3") {
            let proposer = try session.account("A")
            let voter = try session.account("B")
            try session.act(as: "B")
            GovernanceProposeBuyJourney.secondVoterVotes(app, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S3-B-voted-yes")
            try session.act(as: "A")
            GovernanceProposeBuyJourney.firstVoterPasses(
                app, proposer: proposer.name, voter: voter.name, recorder: recorder)
            attachScreenshot(of: app, named: "S3-A-bought")
        }

        try session.scenario("S4") {
            let cabal = try JourneyHandoff.read("cabalName")
            try session.act(as: "A")
            GovernanceProposeBuyJourney.seeTheTrade(app, cabal: cabal, recorder: recorder)
            attachScreenshot(of: app, named: "S4-A-trade")
        }

        try session.scenario("S5") {
            let cabal = try JourneyHandoff.read("cabalName")
            try session.act(as: "A")
            GovernanceProposeBuyJourney.cashOutAsA(app, cabal: cabal, recorder: recorder)
            try session.act(as: "B")
            GovernanceProposeBuyJourney.cashOutAsB(app, cabal: cabal, recorder: recorder)
            attachScreenshot(of: app, named: "S5-B-cashed-out")
            try session.act(as: "A")
            GovernanceProposeBuyJourney.withdrawAsA(app, to: refundAddress, recorder: recorder)
            attachScreenshot(of: app, named: "S5-A-withdrawn")
            try session.act(as: "B")
            GovernanceProposeBuyJourney.withdrawAsB(app, to: refundAddress, recorder: recorder)
            attachScreenshot(of: app, named: "S5-B-withdrawn")
        }
    }
}
