import XCTest

nonisolated final class SignInJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testS1SignIn() throws {
        let account = try JourneyAccount.load()
        let app = XCUIApplication.monacoForJourneys()
        let recorder = SignInJourney.recorder()

        SignInJourney.startSignedOut(app, recorder: recorder)
        SignInJourney.signIn(app, as: account, recorder: recorder)
        attachScreenshot(of: app, named: "S1-signed-in")
    }

    @MainActor
    func testS2SessionSurvivesRelaunch() throws {
        let account = try JourneyAccount.load()
        let app = XCUIApplication.monacoForJourneys()
        let recorder = SignInJourney.recorder()

        SignInJourney.ensureSignedIn(app, as: account)

        recorder.step("S2.1", "terminate and relaunch") {
            app.terminate()
            app.launch()
        }
        recorder.step("S2.2", "land on the tab bar without the login form") {
            XCTAssertEqual(SignInJourney.currentScreen(app), .tabs, "S2.2: the relaunch did not restore the session")
        }
        attachScreenshot(of: app, named: "S2-restored")
    }

    @MainActor
    func testS3SignOut() throws {
        let account = try JourneyAccount.load()
        let app = XCUIApplication.monacoForJourneys()
        let recorder = SignInJourney.recorder()

        SignInJourney.ensureSignedIn(app, as: account)
        SignInJourney.signOut(app, recorder: recorder)

        recorder.step("S3.4", "relaunch and stay on the login form") {
            app.terminate()
            app.launch()
            XCTAssertEqual(
                SignInJourney.currentScreen(app), .login, "S3.4: a relaunch after sign-out did not show the login form")
        }
        attachScreenshot(of: app, named: "S3-signed-out")
    }
}
