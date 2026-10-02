import MonacoAPI
import MonacoCore
import Observation
import SwiftUI
import os

/// The reads every tab shares. `MonacoAPIClient` is the production implementation;
/// tests inject a stub so store behaviour can be checked without a server.
///
/// `@MainActor` here is not a decision about where decoding belongs — it matches what
/// `MonacoAPIClient` already is (implicitly main-actor, #326 / cluster item 15), and a
/// `nonisolated` protocol would not compile against it today. When `API/` is made
/// non-isolated this annotation should come off with it, so the JSON decode stops running
/// on the main thread.
@MainActor
protocol AppSessionDataSource: Sendable {
    func getPlatformBalance(accessToken: String) async throws -> PlatformBalanceDTO
    func getHome(accessToken: String) async throws -> HomeViewDTO
    func getHomeDashboard(accessToken: String, leaderboardRange: HomeLeaderboardRange) async throws -> HomeDashboardDTO
    func getHomePnLSeries(accessToken: String, range: HomeLeaderboardRange) async throws -> HomePnLSeriesDTO
    func getPopularAssets(accessToken: String, limit: Int) async throws -> PopularAssetsResponse
}

/// The session the store reads tokens from and reports rejected ones to. `PrivyAuthService`
/// is the only implementation outside tests.
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
    typealias SessionTokenProvider = @MainActor @Sendable () async -> String?
    typealias ProfileClientFactory = @MainActor @Sendable (String) -> MonacoCore.MonacoAPIClient
    var home: HomeViewDTO?
    var dashboard: HomeDashboardDTO?
    var profile: SessionProfile? { didSet { onProfileChange?(profile) } }
    var onProfileChange: ((SessionProfile?) -> Void)?
    var platformBalance: PlatformBalanceDTO?
    var popularAssets: [MarketAssetDTO] = []
    var homePnLSeries: [HomePnLSeriesPointDTO]?
    var isHomePnLSeriesLoading = false
    var isBalanceLoading = false
    var errorMessage: String?
    #if DEBUG
    var errorDebugDetail: String?
    #endif
    var isLoading = true
    var leaderboardRange: HomeLeaderboardRange = .all

    let apiClient: AppSessionDataSource
    private let sessions: SessionAPI?
    let sessionToken: SessionTokenProvider?
    let profileClientFactory: ProfileClientFactory
    var skipsSessionOpen = false
    var refreshGeneration = 0
    var dashboardGeneration = 0
    var pollGeneration = 0
    /// Home boards, popular assets and the P&L curve: started by a refresh but not awaited by
    /// it. Owned here so the next refresh cancels what the last one left running, instead of
    /// letting a session the member has left behind keep writing.
    var deferredWork: [Task<Void, Never>] = []
    /// Bumped by every self-profile write, so a `/v1/me` read that started before the write
    /// cannot put the old name back.
    var profileWriteGeneration = 0

    init(
        apiClient: AppSessionDataSource,
        sessions: SessionAPI? = nil,
        profileClientFactory: ProfileClientFactory? = nil,
        sessionToken: SessionTokenProvider? = nil
    ) {
        self.apiClient = apiClient
        self.sessions = sessions
        self.sessionToken = sessionToken
        self.profileClientFactory =
            profileClientFactory ?? { token in
                MonacoCore.MonacoAPIClient(baseURL: Config.apiBaseURL, accessTokenProvider: { token })
            }
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
        guard profile != nil, let token = auth.accessToken else { return }
        let generation = refreshGeneration
        if let loaded = await loadProfile(auth: auth, token: token),
            mayWrite(generation),
            profile != nil,
            auth.accessToken == token
        {
            profile = loaded
        }
    }

    private func failOpen(_ error: Error, rejectedToken: String, auth: SessionAuthenticating) async {
        isLoading = false
        if case APIError.accountDeleted = error {
            await auth.signOut(reason: ToastCopy.message(for: .accountDeleted), rejectedToken: rejectedToken)
            return
        }
        if case APIError.signedOut = error {
            await auth.signOut(reason: LoginFailureCopy.sessionExpired, rejectedToken: rejectedToken)
            return
        }
        let mapped = SessionErrorMapping.describe(error, apiBaseURL: Config.apiBaseURL)
        errorMessage = mapped.message
        #if DEBUG
        errorDebugDetail = "\(mapped.debugDetail)\n\(Config.api.debugSummary)"
        #endif
    }

    /// Reloads what Home and Profile show. `leaderboardRange` is a *selection*: pass it only
    /// when the member picked a range, and leave it out everywhere else so the board they are
    /// looking at survives the refresh.
    func refresh(
        auth: SessionAuthenticating,
        accessToken: String? = nil,
        leaderboardRange: HomeLeaderboardRange? = nil,
        includeProfile: Bool = true
    ) async {
        let token = accessToken ?? auth.accessToken
        guard let token else {
            errorMessage = "Missing sign-in token."
            isLoading = false
            return
        }
        if let leaderboardRange {
            self.leaderboardRange = leaderboardRange
        }

        cancelDeferredWork()
        refreshGeneration += 1
        let generation = refreshGeneration
        let request = beginDashboardRequest()
        let profileGeneration = profileWriteGeneration

        do {
            isBalanceLoading = platformBalance == nil
            async let dashboardLoad = apiClient.getHomeDashboard(
                accessToken: token,
                leaderboardRange: request.range
            )
            async let meLoad: SessionProfile? = includeProfile ? await self.loadProfile(auth: auth, token: token) : nil
            async let balanceLoad = apiClient.getPlatformBalance(accessToken: token)
            let loadedDashboard = try await dashboardLoad
            let loadedProfile = await meLoad
            let loadedBalance = try? await balanceLoad
            finishRefresh(
                dashboard: loadedDashboard,
                profile: loadedProfile,
                balance: loadedBalance,
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

        if generation == refreshGeneration {
            isBalanceLoading = false
        }
    }

    private func loadProfile(auth: SessionAuthenticating, token: String) async -> SessionProfile? {
        guard let sessions else { return nil }
        do {
            return try await sessions.me()
        } catch APIError.accountDeleted {
            await auth.signOut(reason: ToastCopy.message(for: .accountDeleted), rejectedToken: token)
            return nil
        } catch APIError.signedOut {
            await auth.signOut(reason: LoginFailureCopy.sessionExpired, rejectedToken: token)
            return nil
        } catch {
            return nil
        }
    }

    /// One background poll of what Home and Profile show: dashboard, balance, joined cabals, and
    /// the 1H curve. Unlike `refresh` it never raises an error or a loading flag, and it
    /// only writes values the server actually changed — a poll that fails, or that comes back
    /// identical, leaves the screen exactly as the member last saw it. A poll that lands does
    /// clear a stale error banner, since the condition it described is over.
    ///
    /// Throws when the dashboard read fails so the caller's poll loop can back off. That includes
    /// a 401: signing the member out is for a request they made, not one they never saw.
    func pollLive(auth: SessionAuthenticating) async throws {
        guard let token = auth.accessToken else { return }
        let generation = refreshGeneration
        let request = currentDashboardRequest()
        pollGeneration += 1
        let poll = pollGeneration

        async let dashboardLoad = apiClient.getHomeDashboard(accessToken: token, leaderboardRange: request.range)
        async let balanceLoad = apiClient.getPlatformBalance(accessToken: token)
        async let homeLoad = apiClient.getHome(accessToken: token)
        async let seriesLoad = apiClient.getHomePnLSeries(accessToken: token, range: .oneHour)

        let loadedDashboard = try await dashboardLoad
        let balance = try? await balanceLoad
        let boards = try? await homeLoad
        let series = try? await seriesLoad

        // A pull-to-refresh or a range change started while this was in flight: theirs is
        // newer. So is a poll from the other tab that has already landed.
        guard generation == refreshGeneration,
            isCurrent(request),
            poll == pollGeneration,
            !Task.isCancelled
        else { return }
        QuietUpdate.apply(loadedDashboard, over: dashboard) { dashboard = $0 }
        if let balance { QuietUpdate.apply(balance, over: platformBalance) { platformBalance = $0 } }
        if let boards { QuietUpdate.apply(boards, over: home) { home = $0 } }
        if let series { QuietUpdate.apply(series.points, over: homePnLSeries) { homePnLSeries = $0 } }
        // Only once the screen actually has what the banner said was missing. A poll can
        // land while bootstrap is still failing — `profile` and `home` never arrived — and
        // clearing it there leaves an empty screen with the explanation wiped off it.
        if profile != nil {
            errorMessage = nil
        }
    }

    /// Legacy home boards + popular strip. Does not block Home first paint.
    func refreshDeferredHomePayloads(auth: SessionAuthenticating, accessToken: String? = nil) async {
        await refreshHomeBoards(accessToken: accessToken ?? auth.accessToken)
        await refreshPopular(auth: auth)
    }

    /// Loads GET /v1/home for profile/cabals surfaces. Create-group flows (209) can call this alone.
    func refreshHomeBoards(accessToken: String?) async {
        guard let accessToken else { return }
        let generation = refreshGeneration
        do {
            let boards = try await apiClient.getHome(accessToken: accessToken)
            guard mayWrite(generation) else { return }
            home = boards
        } catch {
            if error.isRequestCancellation { return }
            if case MonacoAPIError.httpStatus(let status) = error, status == 401 {
                return
            }
        }
    }

    /// GET /v1/home/pnl-series for the Home chart. Does not block login or dashboard shell.
    func refreshHomePnLSeries(auth: SessionAuthenticating, accessToken: String? = nil) async {
        let token = accessToken ?? auth.accessToken
        guard let token else { return }
        let generation = refreshGeneration
        isHomePnLSeriesLoading = homePnLSeries == nil
        defer { isHomePnLSeriesLoading = false }
        do {
            let series = try await apiClient.getHomePnLSeries(accessToken: token, range: .oneHour)
            guard mayWrite(generation) else { return }
            homePnLSeries = series.points
        } catch {
            if error.isRequestCancellation { return }
            if case MonacoAPIError.httpStatus(let status) = error, status == 401 {
                await auth.signOutAfterRejectedSession(rejectedToken: token)
            }
        }
    }

    func refreshPopular(auth: SessionAuthenticating) async {
        guard let token = auth.accessToken else { return }
        let generation = refreshGeneration
        do {
            let popular = try await apiClient.getPopularAssets(accessToken: token, limit: 10)
            guard mayWrite(generation) else { return }
            popularAssets = popular.assets
        } catch {
            if error.isRequestCancellation { return }
            if case MonacoAPIError.httpStatus(let status) = error, status == 401 {
                await auth.signOutAfterRejectedSession(rejectedToken: token)
            }
        }
    }

}
