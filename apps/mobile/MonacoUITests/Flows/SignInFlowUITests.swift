import XCTest

nonisolated final class SignInFlowUITests: XCTestCase {
    nonisolated override func setUpWithError() throws {
        continueAfterFailure = false
    }

    @MainActor
    func testS1SignIn() throws {
        let account = try FlowAccount.load()
        let app = XCUIApplication.monacoForFlows()
        let recorder = SignInFlow.recorder()

        SignInFlow.startSignedOut(app, recorder: recorder)
        SignInFlow.signIn(app, as: account, recorder: recorder)
        attachScreenshot(of: app, named: "S1-signed-in")
    }

    @MainActor
    func testS2SessionSurvivesRelaunch() throws {
        let account = try FlowAccount.load()
        let app = XCUIApplication.monacoForFlows()
        let recorder = SignInFlow.recorder()

        SignInFlow.ensureSignedIn(app, as: account)

        recorder.step("S2.1", "terminate and relaunch") {
            app.terminate()
            app.launch()
        }
        recorder.step("S2.2", "land on the tab bar without the login form") {
            XCTAssertEqual(SignInFlow.currentScreen(app), .tabs, "S2.2: the relaunch did not restore the session")
        }
        attachScreenshot(of: app, named: "S2-restored")
    }

    @MainActor
    func testS3SignOut() throws {
        let account = try FlowAccount.load()
        let app = XCUIApplication.monacoForFlows()
        let recorder = SignInFlow.recorder()

        SignInFlow.ensureSignedIn(app, as: account)
        SignInFlow.signOut(app, recorder: recorder)

        recorder.step("S3.3", "relaunch and stay on the login form") {
            app.terminate()
            app.launch()
            XCTAssertEqual(
                SignInFlow.currentScreen(app), .login, "S3.3: a relaunch after sign-out did not show the login form")
        }
        attachScreenshot(of: app, named: "S3-signed-out")
    }
}
