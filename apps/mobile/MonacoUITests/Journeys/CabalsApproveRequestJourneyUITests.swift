import XCTest

nonisolated final class CabalsApproveRequestJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testS1Phase1BAsksAndCancels() throws {
        let account = try JourneyAccount.load()
        let run = try JourneyRun.id()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        CabalsApproveRequestJourney.memberAsksAndCancels(
            app, run: run, recorder: CabalsApproveRequestJourney.recorder())
        attachScreenshot(of: app, named: "S1-B-requested")
    }

    @MainActor
    func testS1Phase2ADenies() throws {
        let account = try JourneyAccount.load()
        let member = try JourneyAccount.load(actor: "B")
        let run = try JourneyRun.id()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        CabalsApproveRequestJourney.creatorDenies(
            app, run: run, member: member.name, recorder: CabalsApproveRequestJourney.recorder())
        attachScreenshot(of: app, named: "S1-A-denied")
    }

    @MainActor
    func testS2Phase1AApproves() throws {
        let account = try JourneyAccount.load()
        let member = try JourneyAccount.load(actor: "B")
        let run = try JourneyRun.id()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        CabalsApproveRequestJourney.creatorApproves(
            app, run: run, member: member.name, recorder: CabalsApproveRequestJourney.recorder())
        attachScreenshot(of: app, named: "S2-A-approved")
    }
}
