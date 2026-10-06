import XCTest

nonisolated final class SignInJourneyUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testJourney() throws {
        let session = try JourneySession()
        let app = session.app
        let recorder = SignInJourney.recorder()

        try session.scenario("S1") {
            try session.act(as: "A")
            attachScreenshot(of: app, named: "S1-signed-in")
        }

        try session.scenario("S2") {
            try session.act(as: "A")
            recorder.step("S2.1", "terminate and relaunch") {
                app.terminate()
                app.launch()
            }
            recorder.step("S2.2", "land on the tab bar without the login form") {
                XCTAssertEqual(
                    SignInJourney.currentScreen(app), .tabs, "S2.2: the relaunch did not restore the session")
            }
            attachScreenshot(of: app, named: "S2-restored")
        }

        try session.scenario("S3") {
            try session.act(as: "A")
            SignInJourney.signOut(app, recorder: recorder)
            recorder.step("S3.4", "relaunch and stay on the login form") {
                app.terminate()
                app.launch()
                XCTAssertEqual(
                    SignInJourney.currentScreen(app), .login,
                    "S3.4: a relaunch after sign-out did not show the login form")
            }
            attachScreenshot(of: app, named: "S3-signed-out")
        }
    }
}
