import AuthenticationServices
import Combine
import Foundation
import MonacoCore
import os
import PrivySDK

/// Wraps Privy SDK init, session restore, Apple and Google login and access-token refresh.
@MainActor
class PrivyAuthService: ObservableObject {
    /// Where sign-in is. See `LoginPhase`.
    /// Settable so the Debug sample sign-in (a subclass) can put the screen in any phase; nothing
    /// in the app assigns it from outside.
    @Published var phase: LoginPhase
    @Published private(set) var accessToken: String?
    /// Set when the user lands back on login without asking to (the backend rejected
    /// their token, or the saved session is gone), so LoginView can explain why.
    /// Cleared on the next sign-in attempt.
    @Published var lastSignOutReason: String?

    private var sessionStore = MonacoSessionStore()
    private let tokenRefresh = SingleFlight<String?>()
    private var isRestoreInFlight = false
    /// The access tokens this sign-in has used. A 401 for a token that is not in here
    /// belongs to a session that has already ended, so it must neither mint a token nor
    /// sign out whoever is signed in now.
    private var sessionTokens = SessionTokenLedger()
    /// Set the moment a sign-out starts, so a late 401 cannot stamp "session expired"
    /// over a sign-out the member asked for, and a second tap cannot start a second one.
    private var isSigningOut = false
    /// Best-effort revoke of the Privy session, running after local state is already gone.
    private let pendingRevoke = PendingRevoke()
    /// Bumped by every successful sign-in, so a revoke started for an earlier session can
    /// tell that the session it was told to end is no longer the one that is open.
    private var signInEpoch = 0
    /// How long a new sign-in will wait out a revoke before going ahead anyway.
    private static let revokeWait = Duration.seconds(2)

    /// Who is signed in. Screens key their loads on this rather than on `accessToken`, so
    /// the hourly token rotation is not mistaken for a new session.
    var sessionIdentity: String? {
        guard case .authenticated(let userID) = phase, accessToken != nil else { return nil }
        return userID
    }

    let privy: Privy

    init(settings: PrivyAuthSettings) {
        let config = PrivyConfig(
            appId: settings.appID,
            appClientId: settings.appClientID,
            loggingConfig: .init(logLevel: .none)
        )
        privy = PrivySdk.initialize(config: config)
        phase = sessionStore.hasExplicitLogin ? .restoring : .idle

        AccessTokenRefreshRegistry.shared.register { [weak self] rejectedToken in
            try await self?.refreshedAccessToken(replacing: rejectedToken)
        }
    }

    convenience init() {
        self.init(settings: Config.privy)
    }

    // MARK: Session restore

    func restoreSessionIfNeeded() async {
        guard accessToken == nil, sessionStore.hasExplicitLogin else {
            if phase == .restoring { phase = .idle }
            return
        }
        switch phase {
        case .restoring, .restoreFailed: break
        default: return
        }
        // Launch and the first scene activation both ask for a restore.
        guard !isRestoreInFlight else { return }
        isRestoreInFlight = true
        defer { isRestoreInFlight = false }

        phase = .restoring
        switch await privy.getAuthState() {
        case .authenticated(let user):
            await storeAuthenticatedUser(user, isRestore: true)
        case .unauthenticated:
            AppLogger.session.notice("Session restore: Privy has no saved session")
            endSession(reason: LoginFailureCopy.sessionExpired)
        case .authenticatedUnverified, .notReady:
            // Privy has a saved session but could not reach its servers to confirm it.
            AppLogger.session.notice("Session restore: saved session could not be verified (offline)")
            phase = .restoreFailed(message: LoginFailureCopy.restoreOffline)
        @unknown default:
            phase = .restoreFailed(message: LoginFailureCopy.restoreOffline)
        }
    }

    func shouldInvalidateBackendSession(serverUserId: String) -> Bool {
        sessionStore.shouldInvalidateSession(serverUserId: serverUserId)
    }

    func recordBackendSession(userId: String) {
        sessionStore.recordSession(userId: userId)
    }

    // MARK: Access token refresh

    /// A token to use instead of `rejectedToken`, which the backend just answered with 401.
    /// Privy access tokens last about an hour; `getAccessToken()` mints a new one from the
    /// saved session. Returns nil when the user is really signed out; throws when the
    /// token could not be fetched right now (offline).
    func refreshedAccessToken(replacing rejectedToken: String) async throws -> String? {
        // The request was made by a session that has since ended (sign-out, or another
        // account signed in). Retrying it under the current token would run one member's
        // request as another.
        guard sessionTokens.contains(rejectedToken) else { return nil }
        if let current = accessToken, current != rejectedToken {
            return current
        }
        let privy = self.privy
        let fresh = try await tokenRefresh.run {
            guard let user = await privy.getUser() else { return nil }
            do {
                return try await user.getAccessToken()
            } catch where PrivyAuthService.isSignedOutError(error) {
                return nil
            }
        }
        // Check again: the session can have ended while Privy was minting the token.
        guard sessionTokens.contains(rejectedToken) else { return nil }
        if let fresh, fresh != rejectedToken, accessToken != nil {
            adoptAccessToken(fresh)
        }
        return fresh
    }

    private func adoptAccessToken(_ token: String) {
        accessToken = token
        sessionTokens.adopt(token)
    }

    // MARK: Apple and Google

    func loginWithApple() async {
        await authorize(.apple) { try await self.privy.oAuth.login(with: .apple, appUrlScheme: "monaco") }
    }

    func loginWithGoogle() async {
        await authorize(.google) { try await self.privy.oAuth.login(with: .google, appUrlScheme: "monaco") }
    }

    private func authorize(_ provider: LoginProvider, _ login: () async throws -> PrivyUser) async {
        // A sign-out whose Privy revoke is still running would tear this session down again.
        await awaitPendingRevoke()
        // A second tap while the sheet is up must not open a second one.
        guard phase.beginAuthorizing(provider) else { return }
        lastSignOutReason = nil

        do {
            let user = try await login()
            await storeAuthenticatedUser(user, isRestore: false)
        } catch {
            accessToken = nil
            AppLogger.session.error("\(String(describing: provider), privacy: .public) sign-in failed: \(String(describing: error), privacy: .public)")
            phase.authorizationFailed(Self.loginFailure(from: error, step: .authorize))
        }
    }

    #if DEBUG
    // MARK: Dev text-message login

    /// Dev builds only: agents and simulators sign in with the Privy dev app's test number.
    /// The production Privy app has SMS login off.
    func sendSMSCode(to phoneNumberE164: String) async throws {
        await awaitPendingRevoke()
        lastSignOutReason = nil
        try await privy.sms.sendCode(to: phoneNumberE164)
    }

    func loginWithSMSCode(_ code: String, sentTo phoneNumberE164: String) async throws {
        await awaitPendingRevoke()
        let user = try await privy.sms.loginWithCode(code, sentTo: phoneNumberE164)
        await storeAuthenticatedUser(user, isRestore: false)
    }
    #endif

    // MARK: Sign out

    func logout() async {
        performLogout(reason: nil)
    }

    /// Same as `logout()`, but records why so LoginView can explain it instead of
    /// silently bouncing the user back with no context.
    ///
    /// Private on purpose: a sign-out driven by a server reply must name the token that
    /// reply rejected, so it goes through `signOut(reason:rejectedToken:)`. The member's own
    /// sign-out goes through `logout()`. Leaving this reachable is what let a stale 401 end
    /// the wrong session.
    private func signOut(reason: String) async {
        performLogout(reason: reason)
    }

    /// A 401 answering a request made with `rejectedToken`, ending the session with a
    /// reason for the login screen to show. Guarded exactly like
    /// `signOutAfterRejectedSession(rejectedToken:)`: a reply that outlived its sign-in must
    /// not sign out the account signed in now, nor stamp its login screen with a reason
    /// meant for the previous one.
    func signOut(reason: String, rejectedToken: String) async {
        guard sessionTokens.contains(rejectedToken) else { return }
        await signOut(reason: reason)
    }

    /// The backend still answered 401 after a token refresh: the session is over.
    /// Private on purpose — every caller must come through a `rejectedToken` overload so
    /// the unguarded path cannot be reintroduced from another area.
    private func signOutAfterRejectedSession() async {
        await signOut(reason: LoginFailureCopy.sessionExpired)
    }

    /// A 401 answering a request made with `rejectedToken`. Ignored unless that token
    /// belongs to the session that is still open, so a reply that outlived its sign-in
    /// cannot sign out the next account or contradict a deliberate sign-out.
    func signOutAfterRejectedSession(rejectedToken: String) async {
        guard sessionTokens.contains(rejectedToken) else { return }
        await signOutAfterRejectedSession()
    }

    /// Runs `call` with the session's access token and, when the server answers 401, ends the
    /// session naming that token. The layer that holds the token reports the rejection, so a
    /// screen never has to guess which sign-in a late 401 belonged to.
    func withAccessToken<T>(_ call: (String) async throws -> T) async throws -> T {
        guard let token = accessToken else { throw Monaco.MonacoAPIError.missingAccessToken }
        do {
            return try await call(token)
        } catch {
            if Self.isUnauthorized(error) {
                await signOutAfterRejectedSession(rejectedToken: token)
            }
            throw error
        }
    }

    private static func isUnauthorized(_ error: Error) -> Bool {
        if case Monaco.MonacoAPIError.httpStatus(401) = error { return true }
        if case Monaco.MonacoAPIError.apiError(status: 401, _) = error { return true }
        return (error as? MonacoCore.MonacoAPIError)?.statusCode == 401
    }

    /// Sign-out is local-first: the session is gone before Privy is told, so the login
    /// screen comes back immediately even offline, polling loops lose their token at once,
    /// and a second tap has nothing left to do.
    private func performLogout(reason: String?) {
        guard !isSigningOut else { return }
        isSigningOut = true
        endSession(reason: reason)

        let privy = self.privy
        let epoch = signInEpoch
        pendingRevoke.start { [self] in
            // Someone has signed in since this revoke was scheduled: the session it was told
            // to end is no longer the one Privy holds, and revoking now would tear down the
            // session that replaced it. This is what makes giving up on the wait safe.
            guard let user = await privy.getUser(), signInEpoch == epoch else { return }
            await user.logout()
        }
    }

    /// Lets a new sign-in wait out a revoke that is still in flight, so a late
    /// `user.logout()` cannot tear down the session it is about to create — but only
    /// briefly. See `PendingRevoke` for why the bound matters.
    private func awaitPendingRevoke() async {
        await pendingRevoke.wait(atMost: Self.revokeWait)
    }

    private func endSession(reason: String?) {
        accessToken = nil
        sessionTokens.clear()
        lastSignOutReason = reason
        phase = .idle
        sessionStore.clear()
    }

    // MARK: Error mapping

    /// True when Privy says there is no usable session, as opposed to a request that
    /// failed on the way (offline, timeout), which must never sign the user out.
    nonisolated static func isSignedOutError(_ error: Error) -> Bool {
        guard let privyError = error as? PrivyError,
              case .authenticationFailure(let reason) = privyError.errorCode else {
            return false
        }
        switch reason {
        case .notLoggedIn, .sessionExpired, .invalidJwt:
            return true
        case .failureDuringAuthentication(let underlying):
            return isSignedOutError(underlying)
        default:
            return false
        }
    }

    nonisolated static func loginFailure(from error: Error, step: LoginStep) -> LoginFailure {
        if let failure = error as? LoginFailure {
            return failure
        }
        if isUserCancellation(error) {
            return .cancelled
        }
        if error is URLError || (error as NSError).domain == NSURLErrorDomain {
            return .offline
        }
        if let apiError = error as? ApiError {
            switch apiError {
            case .apiError(let httpCode, _, let description):
                return LoginFailureCopy.failure(forHTTPStatus: httpCode, step: step, detail: description)
            case .networkError(let responseCode, let description):
                // Privy reports transport failures with a non-HTTP response code.
                guard (400..<600).contains(responseCode) else { return .offline }
                return LoginFailureCopy.failure(forHTTPStatus: responseCode, step: step, detail: description)
            case .couldNotConstructRequest, .decodingError, .malformedResponse:
                return .other(detail: nil)
            @unknown default:
                return .other(detail: nil)
            }
        }
        if let privyError = error as? PrivyError,
           case .authenticationFailure(let reason) = privyError.errorCode {
            switch reason {
            case .incorrectCredentials:
                return .codeRejected
            case .failureDuringAuthentication(let underlying):
                return loginFailure(from: underlying, step: step)
            default:
                return .other(detail: privyError.errorDescription)
            }
        }
        return .other(detail: nil)
    }

    /// The member closed the Apple sheet or the Google web sheet.
    private nonisolated static func isUserCancellation(_ error: Error) -> Bool {
        if error is CancellationError { return true }
        if let webError = error as? ASWebAuthenticationSessionError, webError.code == .canceledLogin { return true }
        if let appleError = error as? ASAuthorizationError, appleError.code == .canceled { return true }
        return false
    }

    private func storeAuthenticatedUser(_ user: PrivyUser, isRestore: Bool) async {
        do {
            let token = try await user.getAccessToken()
            isSigningOut = false
            // From here a revoke scheduled by the previous sign-out is stale: this is the
            // session Privy holds now, and ending it would sign the member straight out.
            signInEpoch += 1
            adoptAccessToken(token)
            lastSignOutReason = nil
            phase = .authenticated(userID: user.id)
            if !isRestore {
                sessionStore.markExplicitLogin()
            }
            #if DEBUG
            exportDebugTokens(accessToken: token, user: user)
            await ensureServerSweepSigner(for: user)
            #endif
        } catch {
            AppLogger.session.error("getAccessToken failed (restore: \(isRestore)): \(String(describing: error), privacy: .public)")
            accessToken = nil
            if Self.isSignedOutError(error) {
                endSession(reason: LoginFailureCopy.sessionExpired)
            } else if isRestore {
                // Couldn't reach Privy. The saved session is still good; let the user retry.
                phase = .restoreFailed(message: LoginFailureCopy.restoreOffline)
            } else {
                phase = .failed(message: LoginFailureCopy.tokenUnavailable)
            }
        }
    }

    #if DEBUG
    private func serverSweepSignerID() -> String? {
        let environment = ProcessInfo.processInfo.environment
        let fromEnvironment = environment["PRIVY_AUTHORIZATION_KEY_ID"]
            ?? environment["SIMCTL_CHILD_PRIVY_AUTHORIZATION_KEY_ID"]
        if let fromEnvironment, !fromEnvironment.isEmpty {
            return fromEnvironment
        }
        let fromPlist = Bundle.main.object(forInfoDictionaryKey: "PRIVY_AUTHORIZATION_KEY_ID") as? String
        if let fromPlist, !fromPlist.isEmpty {
            return fromPlist
        }
        return "j2ygtljjgxmn5tzao5vjov1t"
    }

    private func ensureServerSweepSigner(for user: PrivyUser) async {
        guard let signerID = serverSweepSignerID() else {
            exportSignerMigrationResults([[
                "address": "",
                "success": false,
                "error": "missing signer id",
            ]])
            return
        }

        do {
            try await user.migrateWalletsIfNeeded()
            try await user.refresh()
        } catch {
            exportSignerMigrationResults([[
                "address": "",
                "success": false,
                "error": "wallet refresh: \(error.localizedDescription)",
            ]])
            return
        }

        guard !user.embeddedSolanaWallets.isEmpty else {
            exportSignerMigrationResults([[
                "address": "",
                "success": false,
                "error": "no embedded solana wallet",
            ]])
            return
        }

        var results: [[String: Any]] = []
        for wallet in user.embeddedSolanaWallets {
            do {
                try await wallet.addSigner(SignerInput(signerId: signerID))
                results.append([
                    "address": wallet.address,
                    "success": true,
                ])
            } catch {
                let message = error.localizedDescription
                let alreadyAdded = message.localizedCaseInsensitiveContains("duplicate signer")
                results.append([
                    "address": wallet.address,
                    "success": alreadyAdded,
                    "error": alreadyAdded ? "" : message,
                ])
            }
        }
        exportSignerMigrationResults(results)
    }

    private func exportSignerMigrationResults(_ results: [[String: Any]]) {
        let payload: [String: Any] = ["wallets": results]
        guard JSONSerialization.isValidJSONObject(payload),
              let data = try? JSONSerialization.data(withJSONObject: payload) else {
            return
        }
        let url = FileManager.default.urls(for: .documentDirectory, in: .userDomainMask)[0]
            .appendingPathComponent("privy-signer-migration.json")
        try? data.write(to: url, options: .atomic)
    }

    private func exportDebugTokens(accessToken: String, user: PrivyUser) {
        var payload: [String: String] = [
            "userID": user.id,
            "accessToken": accessToken,
        ]
        if let identityToken = user.identityToken {
            payload["identityToken"] = identityToken
        }
        guard JSONSerialization.isValidJSONObject(payload),
              let data = try? JSONSerialization.data(withJSONObject: payload) else {
            return
        }
        let url = FileManager.default.urls(for: .documentDirectory, in: .userDomainMask)[0]
            .appendingPathComponent("privy-tokens-export.json")
        try? data.write(to: url, options: .atomic)
    }
    #endif
}
