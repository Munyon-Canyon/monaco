import MonacoAPI
import MonacoCore
import Observation
import SwiftUI
import os

/// The token source and rejected-session sink; production uses `PrivyAuthService`.
@MainActor
protocol SessionAuthenticating: AnyObject, Sendable {
    var accessToken: String? { get }
    func shouldInvalidateBackendSession(serverUserId: String) -> Bool
    func recordBackendSession(userId: String)
    func refreshedAccessToken(replacing rejectedToken: String) async throws -> String?
    /// Ends the session with a reason to show, but only if `rejectedToken` still belongs to
    /// it. Every sign-out driven by a server reply names the token that reply rejected, so a
    /// 401 that outlived its sign-in cannot end the session that replaced it.
    func signOut(reason: String, rejectedToken: String) async
    func signOutAfterRejectedSession(rejectedToken: String) async
}

extension PrivyAuthService: SessionAuthenticating {}

/// Shared post-auth home + profile state. Tabs read this instead of a one-shot DTO.
@Observable
@MainActor
final class AppSessionStore {
    /// True once a refresh has finished, so Home shows its screen rather than the skeleton.
    var hasLoaded = false
    var profile: SessionProfile? { didSet { onProfileChange?(profile) } }
    var onProfileChange: ((SessionProfile?) -> Void)?
    var errorMessage: String?
    #if DEBUG
    var errorDebugDetail: String?
    #endif
    var isLoading = true
    var nudgeDismissed = false

    let sessions: SessionAPI?
    var skipsSessionOpen = false
    private var refreshGeneration = 0
    /// Protects a local profile write from an older `/v1/me` response.
    private var profileWriteGeneration = 0

    init(sessions: SessionAPI? = nil) {
        self.sessions = sessions
    }

    func bootstrap(auth: SessionAuthenticating, devSession: Bool = false) async {
        guard let token = await accessToken(auth: auth) else {
            reset()
            errorMessage = "Missing sign-in token."
            return
        }
        guard let sessions else {
            isLoading = false
            errorMessage = "Your account didn't load"
            return
        }

        let generation = refreshGeneration
        if profile == nil { isLoading = true }
        errorMessage = nil
        #if DEBUG
        errorDebugDetail = nil
        #endif

        skipsSessionOpen = devSession
        do {
            let profile = try await (devSession ? sessions.me() : sessions.openSession())
            guard mayWrite(generation) else { return }
            if auth.shouldInvalidateBackendSession(serverUserId: profile.userID) {
                await auth.signOutAfterRejectedSession(rejectedToken: token)
                return
            }
            auth.recordBackendSession(userId: profile.userID)
            self.profile = profile
            isLoading = false
            await refresh(auth: auth, accessToken: auth.accessToken, includeProfile: false)
        } catch {
            if error.isRequestCancellation { return }
            await failOpen(error, rejectedToken: token, auth: auth)
        }
    }

    func noteForeground(auth: SessionAuthenticating) async {
        guard profile != nil, let token = await accessToken(auth: auth) else { return }
        let (generation, profileGeneration) = (refreshGeneration, profileWriteGenerationValue())
        if let loaded = await loadProfile(auth: auth, token: token),
            mayWrite(generation), profileGeneration == profileWriteGenerationValue(),
            await accessToken(auth: auth) == token
        {
            profile = loaded
        }
    }

    /// Reloads Home and Profile.
    func refresh(
        auth: SessionAuthenticating,
        accessToken: String? = nil,
        includeProfile: Bool = true
    ) async {
        let token = await resolvedAccessToken(accessToken, auth: auth)
        guard let token else {
            errorMessage = "Missing sign-in token."
            isLoading = false
            return
        }
        refreshGeneration += 1
        let generation = refreshGeneration
        let profileGeneration = profileWriteGeneration

        let loadedProfile = includeProfile ? await loadProfile(auth: auth, token: token) : nil
        await finishRefresh(
            profile: loadedProfile,
            generation: generation,
            profileGeneration: profileGeneration,
            auth: auth,
            token: token
        )
    }

    private func loadProfile(auth: SessionAuthenticating, token: String) async -> SessionProfile? {
        guard let sessions else { return nil }
        do {
            return try await sessions.me()
        } catch {
            return await failedProfileLoad(error, token: token, auth: auth)
        }
    }

    private func resetGenerations() {
        refreshGeneration += 1
        profileWriteGeneration += 1
    }

    func refreshGenerationValue() -> Int { refreshGeneration }
    func profileWriteGenerationValue() -> Int { profileWriteGeneration }

    func bumpProfileWriteGeneration() { profileWriteGeneration += 1 }

    func reset() {
        resetGenerations()
        hasLoaded = false
        profile = nil
        errorMessage = nil
        #if DEBUG
        errorDebugDetail = nil
        #endif
        isLoading = false
        nudgeDismissed = false
        skipsSessionOpen = false
    }

}
