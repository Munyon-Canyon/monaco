import XCTest

nonisolated final class JoinJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testS1Phase1ACopiesTheCode() throws {
        let account = try JourneyAccount.load()
        let run = try JourneyRun.id()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        try JoinJourney.creatorCopiesCode(app, run: run, recorder: JoinJourney.recorder())
        attachScreenshot(of: app, named: "S1-A-code-copied")
    }

    @MainActor
    func testS1Phase2BRequestsByCode() throws {
        let account = try JourneyAccount.load()
        let run = try JourneyRun.id()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        try JoinJourney.memberRequestsByCode(app, run: run, recorder: JoinJourney.recorder())
        attachScreenshot(of: app, named: "S1-B-requested")
    }

    @MainActor
    func testS1Phase3AApproves() throws {
        let account = try JourneyAccount.load()
        let member = try JourneyAccount.load(actor: "B")
        let run = try JourneyRun.id()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        JoinJourney.creatorApproves(app, run: run, member: member.name, recorder: JoinJourney.recorder())
        attachScreenshot(of: app, named: "S1-A-approved")
    }

    @MainActor
    func testS1Phase4BEntersAndJoinsOpen() throws {
        let account = try JourneyAccount.load()
        let run = try JourneyRun.id()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        JoinJourney.memberEntersAndJoinsOpen(app, run: run, recorder: JoinJourney.recorder())
        attachScreenshot(of: app, named: "S1-B-joined-open")
    }
}
