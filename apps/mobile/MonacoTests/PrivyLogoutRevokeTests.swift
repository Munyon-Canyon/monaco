import MonacoCore
import Synchronization
import Testing

@testable import Monaco

#if DEBUG
private nonisolated final class RevokedTokens: Sendable {
    private let storage = Mutex<[String]>([])

    var tokens: [String] { storage.withLock { $0 } }

    func record(_ token: String) { storage.withLock { $0.append(token) } }
}

@MainActor
struct PrivyLogoutRevokeTests {
    private func service(recording revoked: RevokedTokens) -> PrivyAuthService {
        let auth = PrivyAuthService.processInstance ?? PrivyAuthService()
        auth.revokeSession = { token in revoked.record(token) }
        auth.adoptDevAccessToken("copied-token")
        return auth
    }

    @Test func signingOutAsksTheServerToEndTheSessionOfTheTokenItHeld() async {
        let revoked = RevokedTokens()
        let auth = service(recording: revoked)

        await auth.logout()
        await auth.awaitPendingRevoke()

        #expect(revoked.tokens == ["copied-token"])
        #expect(auth.accessToken == nil)
    }

    @Test func signingOutDeletesTheInteractionsDonatedForContactSuggestions() async {
        let deleted = RevokedTokens()
        let auth = service(recording: RevokedTokens())
        auth.deleteDonatedInteractions = { deleted.record("deleted") }

        await auth.logout()
        await auth.awaitPendingRevoke()

        #expect(deleted.tokens == ["deleted"])
    }

    @Test func aSessionTheServerAlreadyRejectedIsNotRevokedAgain() async {
        let revoked = RevokedTokens()
        let auth = service(recording: revoked)

        await auth.logout(reason: LoginFailureCopy.sessionExpired)
        await auth.awaitPendingRevoke()

        #expect(revoked.tokens.isEmpty)
        #expect(auth.accessToken == nil)
    }
}
#endif
