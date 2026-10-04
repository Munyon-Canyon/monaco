import XCTest

nonisolated final class FirstRunJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testS1PickAHandleAndSkip() throws {
        let member = try FirstRunJourney.member()
        let takenHandle = try JourneyHandoff.read("takenHandle")
        let app = XCUIApplication.monacoForJourneys()
        FirstRunJourney.pickHandleAndSkip(app, as: member, takenHandle: takenHandle)
        attachScreenshot(of: app, named: "S1 Home with the add-number nudge")
    }

    @MainActor
    func testS2NumberLinkedElsewhere() throws {
        let member = try FirstRunJourney.member()
        let other = try FirstRunJourney.otherMember()
        let app = XCUIApplication.monacoForJourneys()
        FirstRunJourney.ensureSignedIn(app, as: member)
        FirstRunJourney.numberLinkedElsewhere(app, other: other)
        attachScreenshot(of: app, named: "S2 Home after Not now")
    }

    @MainActor
    func testS3RelaunchPastTheGate() throws {
        let member = try FirstRunJourney.member()
        let app = XCUIApplication.monacoForJourneys()
        FirstRunJourney.ensureSignedIn(app, as: member)
        FirstRunJourney.relaunchPastTheGate(app)
        attachScreenshot(of: app, named: "S3 Profile with the handle")
    }

    @MainActor
    func testS4LinkANumber() throws {
        let member = try FirstRunJourney.member()
        let number = try FirstRunJourney.linkNumber()
        let app = XCUIApplication.monacoForJourneys()
        FirstRunJourney.ensureSignedIn(app, as: member)
        FirstRunJourney.linkANumber(app, number: number)
        attachScreenshot(of: app, named: "S4 Home with the X nudge")
    }

    @MainActor
    func testS5LinkFakeX() throws {
        _ = try FirstRunJourney.member()
        let devToken = try JourneyHandoff.read("devToken")
        let app = XCUIApplication.monacoForJourneys()
        FirstRunJourney.linkFakeX(app, devToken: devToken)
        attachScreenshot(of: app, named: "S5 Home with no nudge")
    }
}
