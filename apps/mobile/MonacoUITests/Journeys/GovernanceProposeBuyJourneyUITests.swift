import XCTest

nonisolated final class GovernanceProposeBuyJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testS1Phase1AProposesBuy() throws {
        let account = try JourneyAccount.load()
        let run = try JourneyRun.id()
        let cabal = try JourneyHandoff.read("cabalName")
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        GovernanceProposeBuyJourney.proposeBuy(
            app, cabal: cabal, run: run, recorder: GovernanceProposeBuyJourney.recorder())
        attachScreenshot(of: app, named: "S1-A-proposal-sent")
    }

    @MainActor
    func testS2Phase1BVotes() throws {
        let account = try JourneyAccount.load()
        let run = try JourneyRun.id()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        GovernanceProposeBuyJourney.secondVoterVotes(app, run: run, recorder: GovernanceProposeBuyJourney.recorder())
        attachScreenshot(of: app, named: "S2-B-voted-yes")
    }

    @MainActor
    func testS2Phase2AVotesAndSeesBuying() throws {
        let account = try JourneyAccount.load()
        let second = try JourneyAccount.load(actor: "B")
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        GovernanceProposeBuyJourney.firstVoterPasses(
            app, proposer: account.name, voter: second.name, recorder: GovernanceProposeBuyJourney.recorder())
        attachScreenshot(of: app, named: "S2-A-buying")
    }
}
