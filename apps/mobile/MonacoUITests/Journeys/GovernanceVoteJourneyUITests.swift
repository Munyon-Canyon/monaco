import XCTest

nonisolated final class GovernanceVoteJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let recorder = GovernanceVoteJourney.recorder()
        let run = try JourneyRun.id()

        try session.scenario("S1") {
            let proposalID = try JourneyHandoff.read("proposalID")
            try session.act(as: "A")
            GovernanceVoteJourney.voteFromHome(app, proposalID: proposalID, run: run, recorder: recorder)
            GovernanceVoteJourney.castAndChange(app, recorder: recorder)
            attachScreenshot(of: app, named: "S1-A-voted-yes")
            try session.act(as: "B")
            GovernanceVoteJourney.secondVoterPasses(app, proposalID: proposalID, recorder: recorder)
            attachScreenshot(of: app, named: "S1-B-passed")
            try session.act(as: "A")
            GovernanceVoteJourney.firstVoterSeesItLeave(app, proposalID: proposalID, recorder: recorder)
            attachScreenshot(of: app, named: "S1-A-settled")
        }

        try session.scenario("S2") {
            let cabalName = try JourneyHandoff.read("cabalName")
            let proposalID = try JourneyHandoff.read("proposalID")
            try session.act(as: "A")
            GovernanceVoteJourney.voteFromCabalPage(
                app, cabalName: cabalName, proposalID: proposalID, run: run, recorder: recorder)
            attachScreenshot(of: app, named: "S2-A-cabal-page")
        }

        try session.scenario("S3") {
            let proposalID = try JourneyHandoff.read("proposalID")
            let closedProposalID = try JourneyHandoff.read("closedProposalID")
            try session.act(as: "A")
            GovernanceVoteJourney.allProposals(
                app, proposalID: proposalID, closedProposalID: closedProposalID, recorder: recorder)
            attachScreenshot(of: app, named: "S3-A-all-proposals")
        }

        try session.scenario("S4") {
            let sidesClosedID = try JourneyHandoff.read("sidesClosedProposalID")
            let plainClosedID = try JourneyHandoff.read("plainClosedProposalID")
            let otherName = try session.account("B").name
            try session.act(as: "A")
            GovernanceVoteJourney.sidesAfterClose(
                app, sidesClosedID: sidesClosedID, plainClosedID: plainClosedID, hostName: "QA host",
                otherName: otherName, recorder: recorder)
            attachScreenshot(of: app, named: "S4-A-sides")
        }
    }
}
