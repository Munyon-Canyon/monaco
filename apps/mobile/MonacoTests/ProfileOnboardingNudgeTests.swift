import MonacoCore
import XCTest

@testable import Monaco

nonisolated final class ProfileOnboardingNudgeTests: XCTestCase {
    private func profile(authState: String) throws -> SessionProfile {
        let json = """
            {"id":"01890a5d-ac96-774b-bcce-b302099a8058","handle":"kai","display_name":"Kai Cenat",\
            "auth_state":"\(authState)","account_status":"active","login_provider":"google",\
            "member_wallet_address":"wallet-1","phone_linked":false,"created_at":"2026-09-30T12:00:00Z"}
            """
        return try SessionProfile(json: Data(json.utf8))
    }

    @MainActor
    func testTheOwnProfileNudgesAMemberWhoStillNeedsAPhone() async throws {
        let member = try profile(authState: "AWAITING_PHONE")
        let shown = OnboardingNudgeBanner.nudge(profile: member, dismissed: false, connectX: true)
        XCTAssertEqual(shown, .addPhone("Add your number to find friends"))
    }

    @MainActor
    func testTheOwnProfileHidesTheNudgeOnceOnboardingIsComplete() async throws {
        let member = try profile(authState: "ONBOARDING_COMPLETED")
        XCTAssertNil(OnboardingNudgeBanner.nudge(profile: member, dismissed: false, connectX: true))
    }

    @MainActor
    func testHidingTheNudgeKeepsItGone() async throws {
        let member = try profile(authState: "AWAITING_PHONE")
        XCTAssertNil(OnboardingNudgeBanner.nudge(profile: member, dismissed: true, connectX: true))
    }
}
