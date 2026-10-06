import Foundation
import HTTPTypes
import MonacoAPI
import Testing

import enum MonacoCore.FirstRunGate
import enum MonacoCore.LoginFailureCopy
import struct MonacoCore.SessionAPI
import struct MonacoCore.SessionProfile

@testable import Monaco

/// Stands in for `PrivyAuthService`, including its guard: a sign-out is only carried out
/// when the token the server rejected still belongs to the open session. `SessionTokenLedger`
/// is what proves that guard itself; here it is mirrored so the store's *own* half — naming
/// the token each request actually used — is what the assertions are reading.
@MainActor
private final class StubAuth: SessionAuthenticating {
    var accessToken: String? = "token-a"
    /// The tokens the open session will accept a sign-out for.
    var liveTokens: Set<String> = ["token-a"]
    /// Fresh tokens `refreshedAccessToken` hands back, oldest first. Empty means Privy has
    /// nothing newer, which is how a really-dead session behaves.
    var freshTokens: [String] = []

    /// Tokens reported through `signOutAfterRejectedSession`, in order.
    var rejectedTokens: [String] = []
    /// Tokens `refreshedAccessToken` was asked about, in order.
    var refreshRequests: [String] = []
    /// Sign-outs that actually took effect, as (reason, token).
    private(set) var signOuts: [(reason: String, token: String)] = []

    func shouldInvalidateBackendSession(serverUserId: String) -> Bool { false }
    func recordBackendSession(userId: String) {}

    func refreshedAccessToken(replacing rejectedToken: String) async throws -> String? {
        refreshRequests.append(rejectedToken)
        guard liveTokens.contains(rejectedToken), !freshTokens.isEmpty else { return nil }
        let fresh = freshTokens.removeFirst()
        liveTokens.insert(fresh)
        accessToken = fresh
        return fresh
    }

    func signOut(reason: String, rejectedToken: String) async {
        guard liveTokens.contains(rejectedToken) else { return }
        signOuts.append((reason, rejectedToken))
        liveTokens.removeAll()
        accessToken = nil
    }

    func signOutAfterRejectedSession(rejectedToken: String) async {
        rejectedTokens.append(rejectedToken)
    }
}

@MainActor
struct AppSessionStoreBootstrapTests {
    @Test func bootstrapDoesNotAskForTheProfileTwice() async throws {
        let (transport, store, auth, environment) = await boot(.json(.ok, SessionWire.me))
        await store.refresh(auth: auth)
        let sent = await transport.sent
        #expect(sent.filter { $0.path == "/v1/auth/session" }.map(\.method) == [.post])
        #expect(store.profile?.displayName == "Kai Cenat")
        #expect(environment.viewer == Viewer(userID: "01890a5d-ac96-774b-bcce-b302099a8058", handle: "kai"))
    }

    @Test func bootstrapRetriesOnceAndThenStops() async throws {
        let (transport, _, auth, _) = await boot(.json(.unauthorized, "{}"))
        let sent = await transport.sent
        #expect(sent.map(\.path) == ["/v1/auth/session"])
        #expect(auth.signOuts.map(\.reason) == [LoginFailureCopy.sessionExpired])
    }

    /// A bootstrap whose 401 outlived its sign-in. The member signed out and someone else
    /// signed in while `openSession` was in flight; the rejection that finally lands belongs
    /// to a session that no longer exists. It must not sign out the account signed in now,
    /// nor stamp their login screen with a reason meant for the previous one.
    @Test func aBootstrap401ThatOutlivedItsSignInLeavesTheNewSessionAlone() async throws {
        let transport = StubTransport(.gate)
        let store = AppSessionStore(sessions: sessionAPI(transport))
        let auth = StubAuth()
        let pending = Task { await store.bootstrap(auth: auth) }
        await transport.waitForRequest()
        auth.liveTokens = ["token-next"]
        auth.accessToken = "token-next"
        await transport.releaseGate(.json(.unauthorized, "{}"))
        await pending.value
        #expect(auth.signOuts.isEmpty)
        #expect(auth.accessToken == "token-next")
    }

    @Test func aDevSessionReadsMeAndDoesNotOpen() async throws {
        let (transport, store, _, environment) = await boot(.json(.ok, SessionWire.me), dev: true)
        let sent = await transport.sent
        #expect(sent.map(\.path) == ["/v1/me"])
        #expect(store.profile != nil)
        #expect(environment.viewer == Viewer(userID: "01890a5d-ac96-774b-bcce-b302099a8058", handle: "kai"))
    }

    @Test func aDeletedAccountSignsOut() async throws {
        let (_, _, auth, _) = await boot(
            .response(
                status: .forbidden, contentType: "application/problem+json", body: Data(SessionWire.deleted.utf8)
            ))
        #expect(auth.signOuts.map(\.reason) == ["This account was deleted."])
    }

    @Test func foregroundReadsMeAndDoesNotOpen() async throws {
        let (transport, store, auth, _) = await boot(.json(.ok, SessionWire.me))
        let opened = await transport.sent.count
        await store.noteForeground(auth: auth)
        let sent = await transport.sent
        let posts = sent.filter { $0.path == "/v1/auth/session" }
        let after = sent.dropFirst(opened)
        #expect(posts.count == 1)
        #expect(after.map(\.path) == ["/v1/me"])
    }

    @Test func aForegroundRefreshAfterSignOutCannotReplaceTheNextMember() async throws {
        let transport = StubTransport(scripted: [.json(.ok, SessionWire.me), .gate, .json(.ok, SessionWire.next)])
        let store = AppSessionStore(sessions: sessionAPI(transport))
        let auth = StubAuth()
        await store.bootstrap(auth: auth)
        let refresh = Task { await store.noteForeground(auth: auth) }
        while await transport.sent.count < 2 { await Task.yield() }
        store.profile = nil
        auth.accessToken = "token-b"
        auth.liveTokens = ["token-b"]
        await store.bootstrap(auth: auth)
        await transport.releaseGate(.json(.ok, SessionWire.me))
        await refresh.value
        #expect(store.profile?.userID == "01890a5d-ac96-774b-bcce-b302099a9999")
        #expect(store.profile?.memberWalletAddress == "wallet-b")
    }

    @Test func aForegroundReadCannotUndoASavedDisplayName() async throws {
        let renamed = SessionWire.me.replacingOccurrences(of: "Kai Cenat", with: "New name")
        let transport = StubTransport(scripted: [.json(.ok, SessionWire.me), .gate, .json(.ok, renamed)])
        let store = AppSessionStore(sessions: sessionAPI(transport))
        let auth = StubAuth()
        await store.bootstrap(auth: auth)

        let foreground = Task { await store.noteForeground(auth: auth) }
        while await transport.sent.count < 2 { await Task.yield() }
        let outcome = await saveNewName(store, auth: auth)
        #expect(outcome == .saved)
        await transport.releaseGate(.json(.ok, SessionWire.me))
        await foreground.value

        #expect(store.profile?.displayName == "New name")
    }

    @Test func aForegroundReadStartedDuringNameSaveCannotUndoIt() async throws {
        let renamed = SessionWire.me.replacingOccurrences(of: "Kai Cenat", with: "New name")
        let transport = StubTransport(scripted: [.json(.ok, SessionWire.me), .gate, .gate])
        let store = AppSessionStore(sessions: sessionAPI(transport))
        let auth = StubAuth()
        await store.bootstrap(auth: auth)

        let save = Task { await saveNewName(store, auth: auth) }
        while await transport.sent.count < 2 { await Task.yield() }
        let foreground = Task { await store.noteForeground(auth: auth) }
        while await transport.sent.count < 3 { await Task.yield() }
        await transport.releaseGate(.json(.ok, renamed))
        #expect(await save.value == .saved)
        await transport.releaseGate(.json(.ok, SessionWire.me))
        await foreground.value

        #expect(store.profile?.displayName == "New name")
    }

    @Test func signOutDropsAnInFlightRefresh() async {
        let transport = StubTransport(.gate)
        let store = AppSessionStore(sessions: sessionAPI(transport))
        let auth = StubAuth()
        store.profile = try? SessionProfile(json: Data(SessionWire.me.utf8))
        store.hasLoaded = true
        let environment = AppEnvironment(
            auth: PrivyAuthService.processInstance ?? PrivyAuthService(), hints: FakeHintSource(),
            sessionStore: store, isAuthenticated: { true }, endAuthSession: {}
        )

        let refresh = Task { await store.refresh(auth: auth, includeProfile: true) }
        await transport.waitForRequest()
        await environment.signOut()
        await transport.releaseGate(.json(.ok, SessionWire.me))
        await refresh.value

        #expect(store.profile == nil)
        #expect(store.hasLoaded == false)
    }

    @Test func memberBSeesNoMemberADataWhileTheirSessionOpens() async throws {
        let transport = StubTransport(scripted: [.json(.ok, SessionWire.me), .gate, .json(.ok, SessionWire.next)])
        let store = AppSessionStore(sessions: sessionAPI(transport))
        let auth = StubAuth()
        await store.bootstrap(auth: auth)
        let environment = AppEnvironment(
            auth: PrivyAuthService.processInstance ?? PrivyAuthService(), hints: FakeHintSource(),
            sessionStore: store, isAuthenticated: { true }, endAuthSession: {}
        )
        await environment.signOut()
        auth.accessToken = "token-b"

        let openingB = Task { await store.bootstrap(auth: auth) }
        while await transport.sent.count < 2 { await Task.yield() }
        #expect(store.profile == nil)
        #expect(store.hasLoaded == false)

        await transport.releaseGate(.json(.ok, SessionWire.next))
        await openingB.value
        #expect(store.profile?.userID == "01890a5d-ac96-774b-bcce-b302099a9999")
    }

    @Test func aNameSaveUsesPatchMeWithBearerAndDisplayName() async throws {
        let tokens = SessionTokens(privyToken: { "privy-token" }, refresh: { _ in nil })
        let transport = StubTransport(scripted: [.json(.ok, SessionWire.me), .json(.ok, SessionWire.me)])
        let store = AppSessionStore(
            sessions: SessionAPI(api: APIClient(serverURL: testServerURL, tokens: tokens, transport: transport))
        )
        let auth = StubAuth()
        await store.bootstrap(auth: auth)

        let outcome = await saveNewName(store, auth: auth)

        let sent = await transport.sent
        let bodies = await transport.sentBodies
        let body = try #require(bodies.last ?? nil)
        #expect(outcome == .saved)
        #expect(sent.map(\.path) == ["/v1/auth/session", "/v1/me"])
        #expect(sent.allSatisfy { $0.headerFields[.authorization] == "Bearer privy-token" })
        #expect(sent.last?.method == .patch)
        let json = try #require(JSONSerialization.jsonObject(with: body) as? [String: String])
        #expect(json["display_name"] == "New name")
        #expect(store.profile?.displayName == "Kai Cenat")
    }

    @Test func aNameSave4xxShowsTheProblemMessageWithoutEndingTheSession() async throws {
        let tokens = SessionTokens(privyToken: { "privy-token" }, refresh: { _ in nil })
        let problem = Components.Schemas.Problem(
            _type: .about_colon_blank, title: "Bad Request", status: 400, code: .displayNameInvalid,
            message: "That name is unavailable.", traceId: "trace", retryable: false
        )
        let transport = StubTransport(scripted: [.json(.ok, SessionWire.me), try .problem(problem)])
        let store = AppSessionStore(
            sessions: SessionAPI(api: APIClient(serverURL: testServerURL, tokens: tokens, transport: transport))
        )
        let auth = StubAuth()
        await store.bootstrap(auth: auth)

        let outcome = await saveNewName(store, auth: auth)

        #expect(outcome == .failed("That name is unavailable."))
        #expect(auth.rejectedTokens.isEmpty)
        #expect(store.profile?.displayName == "Kai Cenat")
    }

    @Test func aMissingTokenOnTheProfileReloadKeepsTheMemberSignedIn() async {
        let auth = StubAuth()
        let tokens = SessionTokens(
            privyToken: { await MainActor.run { auth.accessToken } },
            refresh: { _ in nil }
        )
        let unauthorized = """
            {"status":401,"code":"unauthorized","message":"Sign in again.","trace_id":"t","retryable":false}
            """
        let transport = StubTransport(scripted: [
            .json(.ok, SessionWire.me), .json(.unauthorized, unauthorized),
        ])
        let store = AppSessionStore(
            sessions: SessionAPI(api: APIClient(serverURL: testServerURL, tokens: tokens, transport: transport))
        )

        await store.bootstrap(auth: auth)
        auth.accessToken = nil
        await store.refresh(auth: auth, accessToken: "token-a", includeProfile: true)

        let sent = await transport.sent
        #expect(sent.map(\.path) == ["/v1/auth/session"])
        #expect(auth.signOuts.isEmpty)
        #expect(store.errorMessage == "Your account didn't load")
    }

}

@MainActor
private func boot(_ reply: StubTransport.Reply, dev: Bool = false) async -> (
    StubTransport, AppSessionStore, StubAuth, AppEnvironment
) {
    let transport = StubTransport(reply)
    let store = AppSessionStore(sessions: sessionAPI(transport))
    let environment = AppEnvironment(
        auth: PrivyAuthService.processInstance ?? PrivyAuthService(), hints: FakeHintSource(),
        sessionStore: store, isAuthenticated: { true }, endAuthSession: {}
    )
    let auth = StubAuth()
    await store.bootstrap(auth: auth, devSession: dev)
    return (transport, store, auth, environment)
}

@MainActor
struct AppSessionStorePhotoTests {
    @Test func aPhotoSaveStoresTheServerProfile() async throws {
        let withPhoto = SessionWire.me.replacingOccurrences(
            of: #""display_name":"Kai Cenat","#,
            with: #""display_name":"Kai Cenat","photo_url":"https://cdn.test/kai.jpg","#
        )
        let transport = StubTransport(scripted: [.json(.ok, SessionWire.me), .json(.ok, withPhoto)])
        let store = AppSessionStore(sessions: sessionAPI(transport))
        let auth = StubAuth()
        await store.bootstrap(auth: auth)

        let outcome = await store.saveProfilePhoto(
            Data([0xFF, 0xD8, 0xFF]), auth: auth, submission: IdempotentSubmission())

        #expect(outcome == .saved)
        #expect(await transport.sent.last?.path == "/v1/me/profile-photo")
        #expect(store.profile?.photoURL?.absoluteString == "https://cdn.test/kai.jpg")
    }

    @Test func aRateLimitedPhotoSaveShowsTheServerMessage() async throws {
        let problem = Components.Schemas.Problem(
            _type: .about_colon_blank, title: "Too Many Requests", status: 429, code: .rateLimited,
            message: "Slow down. Try again soon.", traceId: "trace", retryable: true
        )
        let transport = StubTransport(scripted: [.json(.ok, SessionWire.me), try .problem(problem)])
        let store = AppSessionStore(sessions: sessionAPI(transport))
        let auth = StubAuth()
        await store.bootstrap(auth: auth)

        let outcome = await store.saveProfilePhoto(
            Data([0xFF, 0xD8, 0xFF]), auth: auth, submission: IdempotentSubmission())

        #expect(outcome == .failed("Slow down. Try again soon."))
        #expect(auth.rejectedTokens.isEmpty)
    }
}

@MainActor
private func saveNewName(_ store: AppSessionStore, auth: SessionAuthenticating) async -> ProfileSaveOutcome {
    await store.updateDisplayName("New name", auth: auth, optimistic: false, submission: IdempotentSubmission())
}

private func sessionAPI(_ transport: StubTransport) -> SessionAPI {
    SessionAPI(
        api: APIClient(
            serverURL: testServerURL, tokens: StubTokenProvider(token: "token-a"), transport: transport
        ))
}

private enum SessionWire {
    static let me = """
        {"id":"01890a5d-ac96-774b-bcce-b302099a8058","handle":"kai","display_name":"Kai Cenat",\
        "auth_state":"ONBOARDING_COMPLETED","account_status":"active",\
        "member_wallet_address":"wallet-1","phone_linked":true,"created_at":"2026-09-30T12:00:00Z"}
        """
    static let next =
        #"{"id":"01890a5d-ac96-774b-bcce-b302099a9999","handle":"bee","display_name":"Bee","auth_state":"CREATED","account_status":"active","member_wallet_address":"wallet-b","phone_linked":false,"created_at":"2026-09-30T12:00:00Z"}"#
    static let deleted =
        #"{"status":403,"code":"account_deleted","message":"x","trace_id":"t","retryable":false}"#
}
