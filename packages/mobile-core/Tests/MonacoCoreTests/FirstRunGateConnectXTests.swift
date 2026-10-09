import XCTest

@testable import MonacoCore

final class FirstRunGateConnectXTests: XCTestCase {
    private func me(authState: SessionProfile.AuthState, phoneLinked: Bool = false) -> SessionProfile {
        SessionProfile(
            userID: "user-1", handle: "ana", displayName: "", photoURL: nil,
            authState: authState, accountStatus: .active, loginProvider: .email,
            memberWalletAddress: "wallet-1",
            phoneLinked: phoneLinked, xUsername: nil, handleChangeableAt: nil,
            createdAt: Date()
        )
    }

    func testConnectXOffSkipsTheXStepAndKeepsFindFriends() {
        let awaitingSocials = me(authState: .awaitingSocials, phoneLinked: true)
        XCTAssertEqual(
            FirstRunGate.destination(for: awaitingSocials, onboardingCursor: .socials, connectX: true), .socials)
        XCTAssertEqual(
            FirstRunGate.destination(for: awaitingSocials, onboardingCursor: .socials, connectX: false),
            .app(restricted: false))
        XCTAssertEqual(
            FirstRunGate.destination(
                for: awaitingSocials, onboardingCursor: .socials, contactsPromptSeen: false, connectX: false),
            .findFriends)
        let created = me(authState: .created)
        XCTAssertEqual(
            FirstRunGate.destination(for: created, onboardingCursor: .start, connectX: false), .phone)
    }
}
