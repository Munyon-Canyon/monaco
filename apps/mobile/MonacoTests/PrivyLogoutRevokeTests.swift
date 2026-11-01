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

private nonisolated struct RecordingDevices: DeviceRegistering {
    let signedIn: @Sendable () async -> Bool
    let events: RevokedTokens

    func register(token _: String, environment _: PushEnvironment) async throws {}

    func unregister(token _: String) async throws {
        let held = await signedIn()
        events.record(held ? "unregistered while signed in" : "unregistered after sign-out")
    }
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

    private func registered(_ auth: PrivyAuthService, events: RevokedTokens) async throws {
        let registrar = PushRegistrar(
            service: RecordingDevices(signedIn: { await auth.accessToken != nil }, events: events),
            environment: .sandbox, clock: ContinuousClock())
        try await registrar.register(token: "apns")
        auth.pushRegistrar = registrar
    }

    @Test func aSessionTheServerEndedStillRemovesThePushTokenBeforeTheSessionIsGone() async throws {
        let events = RevokedTokens()
        let auth = service(recording: RevokedTokens())
        try await registered(auth, events: events)

        await auth.logout(reason: LoginFailureCopy.sessionExpired)
        await auth.awaitPendingRevoke()

        #expect(events.tokens == ["unregistered while signed in"])
    }

    @Test func signingOutClearsTheNotificationsAlreadyOnTheLockScreen() async {
        let cleared = RevokedTokens()
        let auth = service(recording: RevokedTokens())
        auth.clearDeliveredNotifications = { cleared.record("cleared") }

        await auth.logout()
        await auth.awaitPendingRevoke()

        #expect(cleared.tokens == ["cleared"])
    }

    @Test func endingEverySessionAsksTheServerWithTheHeldTokenAndThenSkipsTheSingleRevoke() async {
        let everywhere = RevokedTokens()
        let single = RevokedTokens()
        let auth = service(recording: single)
        auth.revokeEverywhere = { token in
            everywhere.record(token)
            return true
        }

        let ended = await auth.endAllSessions()
        await auth.logout()
        await auth.awaitPendingRevoke()

        #expect(ended)
        #expect(everywhere.tokens == ["copied-token"])
        #expect(single.tokens.isEmpty)
        #expect(auth.accessToken == nil)
    }

    @Test func aFailedEndAllSessionsLeavesTheMemberSignedInAndKeepsThePushTokenRemovedOnlyOnce() async throws {
        let events = RevokedTokens()
        let auth = service(recording: RevokedTokens())
        try await registered(auth, events: events)
        auth.revokeEverywhere = { _ in false }

        let ended = await auth.endAllSessions()

        #expect(!ended)
        #expect(auth.accessToken == "copied-token")
        #expect(events.tokens == ["unregistered while signed in"])
    }

    @Test func endAllSessionsDoesNothingWithoutASession() async {
        let auth = PrivyAuthService.processInstance ?? PrivyAuthService()
        auth.adoptDevAccessToken("x")
        await auth.logout()

        let ended = await auth.endAllSessions()

        #expect(!ended)
    }
}
#endif
