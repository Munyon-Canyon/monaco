import XCTest

nonisolated final class FirstRunJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let member = try FirstRunJourney.member()

        try session.scenario("S1") {
            let takenHandle = try JourneyHandoff.read("takenHandle")
            FirstRunJourney.pickHandleAndSkip(app, as: member, takenHandle: takenHandle)
            attachScreenshot(of: app, named: "S1 Home with the add-number nudge")
        }

        try session.scenario("S2") {
            let other = try FirstRunJourney.otherMember()
            FirstRunJourney.resumeAsMember(app, as: member)
            FirstRunJourney.numberLinkedElsewhere(app, other: other)
            attachScreenshot(of: app, named: "S2 Home after Not now")
        }

        try session.scenario("S3") {
            FirstRunJourney.resumeAsMember(app, as: member)
            FirstRunJourney.relaunchPastTheGate(app)
            attachScreenshot(of: app, named: "S3 Profile with the handle")
        }

        try session.scenario("S4") {
            let number = try FirstRunJourney.linkNumber()
            FirstRunJourney.resumeAsMember(app, as: member)
            FirstRunJourney.linkANumber(app, number: number)
            attachScreenshot(of: app, named: "S4 Home with the X nudge")
        }

        try session.scenario("S5") {
            let devToken = try JourneyHandoff.read("devToken")
            FirstRunJourney.linkFakeX(app, devToken: devToken)
            attachScreenshot(of: app, named: "S5 Home with no nudge")
        }
    }
}
