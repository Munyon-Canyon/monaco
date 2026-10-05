import XCTest

nonisolated final class GovernanceVoteJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testS1Phase1AVotesAndChanges() throws {
        let account = try JourneyAccount.load()
        let run = try JourneyRun.id()
        let proposalID = try JourneyHandoff.read("proposalID")
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        GovernanceVoteJourney.voteFromHome(
            app, proposalID: proposalID, run: run, recorder: GovernanceVoteJourney.recorder())
        attachScreenshot(of: app, named: "S1-A-voted-yes")
    }

    @MainActor
    func testS1Phase2BMakesTheMajority() throws {
        let account = try JourneyAccount.load()
        let first = try JourneyAccount.load(actor: "A")
        let proposalID = try JourneyHandoff.read("proposalID")
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        GovernanceVoteJourney.secondVoterPasses(
            app, proposalID: proposalID, voter: first.name, recorder: GovernanceVoteJourney.recorder())
        attachScreenshot(of: app, named: "S1-B-passed")
    }

    @MainActor
    func testS1Phase3ASeesItClose() throws {
        let account = try JourneyAccount.load()
        let proposalID = try JourneyHandoff.read("proposalID")
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        GovernanceVoteJourney.firstVoterSeesItLeave(
            app, proposalID: proposalID, recorder: GovernanceVoteJourney.recorder())
        attachScreenshot(of: app, named: "S1-A-home-cleared")
    }

    @MainActor
    func testS2VoteOnTheCabalCard() throws {
        let account = try JourneyAccount.load()
        let cabalName = try JourneyHandoff.read("cabalName")
        let proposalID = try JourneyHandoff.read("proposalID")
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        GovernanceVoteJourney.voteOnCabalCard(
            app, cabalName: cabalName, proposalID: proposalID, recorder: GovernanceVoteJourney.recorder())
        attachScreenshot(of: app, named: "S2-A-cabal-card")
    }

    @MainActor
    func testS3SeeAllWithClosedHistory() throws {
        let account = try JourneyAccount.load()
        let cabalName = try JourneyHandoff.read("cabalName")
        let proposalID = try JourneyHandoff.read("proposalID")
        let closedProposalID = try JourneyHandoff.read("closedProposalID")
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        GovernanceVoteJourney.seeAllHistory(
            app, cabalName: cabalName, proposalID: proposalID, closedProposalID: closedProposalID,
            recorder: GovernanceVoteJourney.recorder())
        attachScreenshot(of: app, named: "S3-A-see-all")
    }
}
