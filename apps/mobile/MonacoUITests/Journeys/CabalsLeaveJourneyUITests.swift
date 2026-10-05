import XCTest

nonisolated final class CabalsLeaveJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testS1Phase1BLeaves() throws {
        let account = try JourneyAccount.load()
        let run = try JourneyRun.id()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        CabalsLeaveJourney.memberLeaves(app, run: run, recorder: CabalsLeaveJourney.recorder())
        attachScreenshot(of: app, named: "S1-B-left")
    }

    @MainActor
    func testS2Phase1ACreatorStays() throws {
        let account = try JourneyAccount.load()
        let run = try JourneyRun.id()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        CabalsLeaveJourney.creatorStays(app, run: run, recorder: CabalsLeaveJourney.recorder())
        attachScreenshot(of: app, named: "S2-A-creator-note")
    }

    @MainActor
    func testS3Phase1BSellsAndLeaves() throws {
        let account = try JourneyAccount.load()
        let run = try JourneyRun.id()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        CabalsLeaveJourney.memberSellsAndLeaves(app, run: run, recorder: CabalsLeaveJourney.recorder())
        attachScreenshot(of: app, named: "S3-B-leave-dialog")
    }
}
