import XCTest

nonisolated final class CreateCabalJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testS1FormShowsTheFourRules() throws {
        let account = try JourneyAccount.load()
        let app = XCUIApplication.monacoForJourneys()
        let recorder = CreateCabalJourney.recorder()

        SignInJourney.ensureSignedIn(app, as: account)
        CreateCabalJourney.openForm(app, recorder: recorder)
        attachScreenshot(of: app, named: "S1-create-form")
    }

    @MainActor
    func testS2CreateACabal() throws {
        let account = try JourneyAccount.load()
        let app = XCUIApplication.monacoForJourneys()
        let recorder = CreateCabalJourney.recorder()
        let name = CreateCabalJourney.uniqueName()

        SignInJourney.ensureSignedIn(app, as: account)
        CreateCabalJourney.openForm(app, recorder: recorder)
        try JourneyHandoff.write("cabalName", name)
        CreateCabalJourney.create(app, named: name, recorder: recorder)
        attachScreenshot(of: app, named: "S2-cabals-list")
    }

    @MainActor
    func testS3BlankNameCannotBeSubmitted() throws {
        let account = try JourneyAccount.load()
        let app = XCUIApplication.monacoForJourneys()
        let recorder = CreateCabalJourney.recorder()

        SignInJourney.ensureSignedIn(app, as: account)
        CreateCabalJourney.openForm(app, recorder: recorder)
        CreateCabalJourney.blankNameStaysDisabled(app, recorder: recorder)
        attachScreenshot(of: app, named: "S3-blank-name")
    }
}
