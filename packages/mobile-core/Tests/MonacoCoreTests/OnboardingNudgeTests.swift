import Foundation
import MonacoAPI
import MonacoCore
import XCTest

final class OnboardingNudgeTests: XCTestCase {
    private let authStates: [SessionProfile.AuthState] = [
        .created, .awaitingPhone, .awaitingSocials, .onboardingCompleted, .unknown("AWAITING_VIDEO"),
    ]
    private let accountStatuses: [SessionProfile.AccountStatus] = [
        .active, .suspended, .banned, .unknown("frozen"),
    ]

    func testEveryAuthStateAndAccountStatus() {
        for authState in authStates {
            for accountStatus in accountStatuses {
                var profile = SessionProfile(Components.Schemas.Me.sample)
                profile.authState = authState
                profile.accountStatus = accountStatus

                XCTAssertEqual(
                    nudge(for: profile),
                    expected(authState, accountStatus),
                    "\(authState) × \(accountStatus)"
                )
            }
        }
    }

    func testTheMessageIsTheBannerCopy() {
        let phone = "Add your number to find friends"
        let x = "Connect X to find people you follow"
        XCTAssertEqual(OnboardingNudge.addPhone(phone).message, phone)
        XCTAssertEqual(OnboardingNudge.linkX(x).message, x)
    }

    private func expected(
        _ authState: SessionProfile.AuthState,
        _ accountStatus: SessionProfile.AccountStatus
    ) -> OnboardingNudge? {
        switch (authState, accountStatus) {
        case (.awaitingPhone, .active): .addPhone("Add your number to find friends")
        case (.awaitingSocials, .active): .linkX("Connect X to find people you follow")
        default: nil
        }
    }
}
