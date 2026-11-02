import XCTest

@testable import MonacoCore

final class FirstRunNewAccountTests: XCTestCase {
    private typealias AuthState = SessionProfile.AuthState
    private typealias AccountStatus = SessionProfile.AccountStatus

    private func me(handle: String?, authState: AuthState, accountStatus: AccountStatus) -> SessionProfile {
        SessionProfile(
            userID: "user-1", handle: handle, displayName: "", photoURL: nil,
            authState: authState, accountStatus: accountStatus, loginProvider: .email,
            memberWalletAddress: "wallet-1",
            phoneLinked: false, xUsername: nil, handleChangeableAt: nil,
            createdAt: Date()
        )
    }

    func testANewMemberIsAskedBeforeTheHandleStepUntilTheyConfirm() {
        let fresh = me(handle: nil, authState: .created, accountStatus: .active)
        XCTAssertEqual(
            FirstRunGate.destination(for: fresh, onboardingCursor: .start, newAccountConfirmed: false), .newAccount)
        XCTAssertEqual(
            FirstRunGate.destination(for: fresh, onboardingCursor: .start, newAccountConfirmed: true), .handle)
        XCTAssertEqual(FirstRunGate.destination(for: fresh, onboardingCursor: .start), .handle)
    }

    func testTheNewAccountAskOnlyGatesAMemberWithoutAHandle() {
        let named = me(handle: "ana", authState: .created, accountStatus: .active)
        XCTAssertEqual(
            FirstRunGate.destination(for: named, onboardingCursor: .start, newAccountConfirmed: false), .phone)
        for status in [AccountStatus.banned, .suspended] {
            let restricted = me(handle: nil, authState: .created, accountStatus: status)
            XCTAssertNotEqual(
                FirstRunGate.destination(for: restricted, onboardingCursor: .start, newAccountConfirmed: false),
                .newAccount)
        }
    }

    func testConfirmingANewAccountIsRememberedForThatUserOnly() {
        let suite = "new-account-gate-\(UUID().uuidString)"
        guard let defaults = UserDefaults(suiteName: suite) else {
            XCTFail("the new account suite did not open")
            return
        }
        defaults.removePersistentDomain(forName: suite)
        defer { defaults.removePersistentDomain(forName: suite) }
        XCTAssertFalse(FirstRunGate.newAccountConfirmed(userID: "user-a", in: defaults))
        FirstRunGate.markNewAccountConfirmed(userID: "user-a", in: defaults)
        XCTAssertTrue(FirstRunGate.newAccountConfirmed(userID: "user-a", in: defaults))
        XCTAssertFalse(FirstRunGate.newAccountConfirmed(userID: "user-b", in: defaults))
    }
}
