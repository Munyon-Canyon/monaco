import MonacoAPI
import MonacoCore
import Observation
import SwiftUI
import os

/// Shared app-session reads; production uses `MonacoAPIClient`, tests use a stub.
@MainActor
protocol AppSessionDataSource: Sendable {
    func getHomeDashboard(accessToken: String) async throws -> HomeDashboardDTO
}

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

/// Shared post-auth home + profile payload. Tabs read this instead of a one-shot DTO.
@Observable
@MainActor
final class AppSessionStore {
    var dashboard: HomeDashboardDTO?
    var profile: SessionProfile? { didSet { onProfileChange?(profile) } }
    var onProfileChange: ((SessionProfile?) -> Void)?
    var errorMessage: String?
    #if DEBUG
    var errorDebugDetail: String?
    #endif
    var isLoading = true
    var nudgeDismissed = false

    let apiClient: AppSessionDataSource
    let sessions: SessionAPI?
    var skipsSessionOpen = false
    private var refreshGeneration = 0
    private var dashboardGeneration = 0
    private var pollGeneration = 0
    /// Deferred home work is cancelled before the next refresh.
    private var deferredWork: [Task<Void, Never>] = []
    /// Protects a local profile write from an older `/v1/me` response.
    private var profileWriteGeneration = 0

    init(apiClient: AppSessionDataSource, sessions: SessionAPI? = nil) {
        self.apiClient = apiClient
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
        cancelDeferredWork()
        refreshGeneration += 1
        let generation = refreshGeneration
        let request = beginDashboardRequest()
        let profileGeneration = profileWriteGeneration

        do {
            async let dashboardLoad = apiClient.getHomeDashboard(accessToken: token)
            async let meLoad: SessionProfile? = includeProfile ? await self.loadProfile(auth: auth, token: token) : nil
            let loadedDashboard = try await dashboardLoad
            let loadedProfile = await meLoad
            await finishRefresh(
                dashboard: loadedDashboard,
                profile: loadedProfile,
                generation: generation,
                profileGeneration: profileGeneration,
                request: request,
                auth: auth,
                token: token
            )
        } catch MonacoAPIError.httpStatus(let status) where status == 401 {
            await auth.signOutAfterRejectedSession(rejectedToken: token)
        } catch MonacoAPIError.httpStatus {
            guard generation == refreshGeneration else { return }
            errorMessage = "Couldn't load this. Try again."
        } catch {
            if error.isRequestCancellation { return }
            guard generation == refreshGeneration else { return }
            errorMessage = "No connection. Check your internet and try again."
        }
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
        dashboardGeneration += 1
        pollGeneration += 1
        profileWriteGeneration += 1
    }

    func refreshGenerationValue() -> Int { refreshGeneration }
    func dashboardGenerationValue() -> Int { dashboardGeneration }
    func pollGenerationValue() -> Int { pollGeneration }
    func profileWriteGenerationValue() -> Int { profileWriteGeneration }
    func deferredWorkValue() -> [Task<Void, Never>] { deferredWork }

    func bumpProfileWriteGeneration() { profileWriteGeneration += 1 }

    func nextDashboardRequest() -> DashboardRequest {
        dashboardGeneration += 1
        return DashboardRequest(generation: dashboardGeneration)
    }

    func nextPollGeneration() -> Int {
        pollGeneration += 1
        return pollGeneration
    }

    func replaceDeferredWork(with task: Task<Void, Never>) {
        deferredWork.removeAll(where: \.isCancelled)
        deferredWork.append(task)
    }

    func cancelAndClearDeferredWork() {
        for task in deferredWork { task.cancel() }
        deferredWork.removeAll()
    }

    func reset() {
        cancelAndClearDeferredWork()
        resetGenerations()
        dashboard = nil
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
