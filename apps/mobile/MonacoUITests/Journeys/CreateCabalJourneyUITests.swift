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

    @MainActor
    func testS4Phase1ACreatesAnOpenCabal() throws {
        let account = try JourneyAccount.load()
        let run = try JourneyRun.id()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        try CreateCabalJourney.creatorStartsAnOpenCabal(app, run: run, recorder: CreateCabalJourney.recorder())
        attachScreenshot(of: app, named: "S4-A-invite-code")
    }

    @MainActor
    func testS4Phase2BJoinsWithTheCode() throws {
        let account = try JourneyAccount.load()
        let run = try JourneyRun.id()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        try CreateCabalJourney.friendJoinsWithTheCode(app, run: run, recorder: CreateCabalJourney.recorder())
        attachScreenshot(of: app, named: "S4-B-joined")
    }

    @MainActor
    func testS4Phase3ASeesBOnTheBoard() throws {
        let account = try JourneyAccount.load()
        let friend = try JourneyAccount.load(actor: "B")
        let run = try JourneyRun.id()
        let app = XCUIApplication.monacoForJourneys()
        SignInJourney.ensureSignedIn(app, as: account)
        CreateCabalJourney.creatorSeesTheFriend(
            app, run: run, friend: friend.name, recorder: CreateCabalJourney.recorder())
        attachScreenshot(of: app, named: "S4-A-member-board")
    }
}
