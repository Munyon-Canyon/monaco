import MonacoAPI
import MonacoCore

extension AppSessionStore {
    func failOpen(_ error: Error, rejectedToken: String, auth: SessionAuthenticating) async {
        isLoading = false
        if case APIError.accountDeleted = error {
            await auth.signOut(reason: ToastCopy.message(for: .accountDeleted), rejectedToken: rejectedToken)
            return
        }
        if case APIError.signedOut = error {
            await auth.signOut(reason: LoginFailureCopy.sessionExpired, rejectedToken: rejectedToken)
            return
        }
        if case APIError.missingAccessToken = error {
            errorMessage = ToastCopy.message(for: .missingAccessToken(""))
            return
        }
        let mapped = SessionErrorMapping.describe(error, apiBaseURL: Config.apiBaseURL)
        errorMessage = mapped.message
        #if DEBUG
        errorDebugDetail = "\(mapped.debugDetail)\n\(Config.api.debugSummary)"
        #endif
    }

    func failedProfileLoad(_ error: Error, token: String, auth: SessionAuthenticating) async -> SessionProfile? {
        if case APIError.accountDeleted = error {
            await auth.signOut(reason: ToastCopy.message(for: .accountDeleted), rejectedToken: token)
        } else if case APIError.signedOut = error {
            await auth.signOut(reason: LoginFailureCopy.sessionExpired, rejectedToken: token)
        } else if case APIError.missingAccessToken = error {
            errorMessage = ToastCopy.message(for: .missingAccessToken(""))
        }
        return nil
    }

    func refreshDeferredHomePayloads(auth: SessionAuthenticating, accessToken: String? = nil) async {
        await refreshHomeBoards(accessToken: accessToken ?? auth.accessToken)
    }

    func refreshHomeBoards(accessToken: String?) async {
        guard let accessToken else { return }
        let generation = refreshGenerationValue()
        do {
            let boards = try await apiClient.getHome(accessToken: accessToken)
            guard mayWrite(generation) else { return }
            home = boards
        } catch {
            if error.isRequestCancellation { return }
        }
    }

    func refreshHomePnLSeries(auth: SessionAuthenticating, accessToken: String? = nil) async {
        let token = accessToken ?? auth.accessToken
        guard let token else { return }
        let generation = refreshGenerationValue()
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

    func resolvedAccessToken(_ provided: String?, auth: SessionAuthenticating) async -> String? {
        if let provided { return provided }
        return await accessToken(auth: auth)
    }

    func accessToken(auth: SessionAuthenticating) async -> String? {
        return auth.accessToken
    }
    var joinedCabals: [HomeGroupBoardRowDTO] {
        (home?.groups ?? []).filter(\.isJoined)
    }

    func noteProfileWrite() {
        bumpProfileWriteGeneration()
    }

    func awaitDeferredWork() async {
        for task in deferredWorkValue() {
            await task.value
        }
    }

    func cancelDeferredWork() {
        cancelAndClearDeferredWork()
    }

    func refreshAfterCreate(auth: SessionAuthenticating, created: Components.Schemas.Cabal) {
        insertJoinedCabal(from: created)
        startDeferredWork { [self] in await deferredRefreshAfterCreate(auth: auth) }
    }

    private func insertJoinedCabal(from created: Components.Schemas.Cabal) {
        let row = HomeGroupBoardRowDTO(
            groupId: created.id, name: created.name, potValueUsd: "0.00", percentReturn: nil, dollarPnl: "+0.00",
            isJoined: true)
        if let current = home {
            guard !current.groups.contains(where: { $0.groupId == created.id }) else { return }
            home = HomeViewDTO(groups: [row] + current.groups, people: current.people)
        } else {
            home = HomeViewDTO(groups: [row], people: [])
        }
    }

    private func deferredRefreshAfterCreate(auth: SessionAuthenticating) async {
        guard let token = await accessToken(auth: auth) else { return }
        let generation = refreshGenerationValue()
        let request = beginDashboardRequest()
        do {
            async let homeLoad = apiClient.getHome(accessToken: token)
            async let dashboardLoad = apiClient.getHomeDashboard(accessToken: token, leaderboardRange: request.range)
            let loadedHome = try await homeLoad
            guard mayWrite(generation) else { return }
            home = loadedHome
            let loadedDashboard = try await dashboardLoad
            guard mayWrite(generation) else { return }
            apply(loadedDashboard, for: request)
        } catch {
            if error.isRequestCancellation { return }
            if case MonacoAPIError.httpStatus(let status) = error, status == 401 {
                await auth.signOutAfterRejectedSession(rejectedToken: token)
            }
        }
    }

    struct DashboardRequest {
        let range: HomeLeaderboardRange
        let generation: Int
    }

    func beginDashboardRequest() -> DashboardRequest {
        nextDashboardRequest()
    }

    func currentDashboardRequest() -> DashboardRequest {
        DashboardRequest(range: leaderboardRange, generation: dashboardGenerationValue())
    }

    func isCurrent(_ request: DashboardRequest) -> Bool {
        request.generation == dashboardGenerationValue() && request.range == leaderboardRange
    }

    func apply(_ loaded: HomeDashboardDTO, for request: DashboardRequest) {
        guard isCurrent(request) else { return }
        dashboard = loaded
    }

    func startDeferredWork(_ work: @escaping @MainActor () async -> Void) {
        replaceDeferredWork(with: Task { await work() })
    }

    func mayWrite(_ generation: Int) -> Bool {
        generation == refreshGenerationValue() && !Task.isCancelled
    }

    func finishRefresh(
        dashboard: HomeDashboardDTO,
        profile: SessionProfile?,
        generation: Int,
        profileGeneration: Int,
        request: DashboardRequest,
        auth: SessionAuthenticating,
        token: String
    ) async {
        guard mayWrite(generation), await accessToken(auth: auth) == token else { return }
        apply(dashboard, for: request)
        if let profile, profileGeneration == profileWriteGenerationValue(), mayWrite(generation) {
            self.profile = profile
        }
        guard mayWrite(generation) else { return }
        errorMessage = nil
        startDeferredWork { [self] in
            async let deferred: Void = refreshDeferredHomePayloads(auth: auth, accessToken: token)
            async let pnlSeries: Void = refreshHomePnLSeries(auth: auth, accessToken: token)
            _ = await (deferred, pnlSeries)
        }
    }

    func refreshBoardsAfterProfileWrite(auth: SessionAuthenticating) {
        startDeferredWork { [self] in
            try? await pollLive(auth: auth)
        }
    }

    func selectLeaderboardRange(_ range: HomeLeaderboardRange, auth: SessionAuthenticating) async {
        await refreshDashboard(auth: auth, leaderboardRange: range)
    }

    func refreshDashboard(auth: SessionAuthenticating, leaderboardRange: HomeLeaderboardRange) async {
        guard let token = await accessToken(auth: auth) else { return }
        setLeaderboardRange(leaderboardRange)
        let request = beginDashboardRequest()
        do {
            let loaded = try await apiClient.getHomeDashboard(
                accessToken: token,
                leaderboardRange: request.range
            )
            apply(loaded, for: request)
        } catch {
            if error.isRequestCancellation { return }
            if case MonacoAPIError.httpStatus(let status) = error, status == 401 {
                await auth.signOutAfterRejectedSession(rejectedToken: token)
            }
        }
    }

    func pollLive(auth: SessionAuthenticating) async throws {
        guard let token = await accessToken(auth: auth) else { return }
        let generation = refreshGenerationValue()
        let request = currentDashboardRequest()
        let poll = nextPollGeneration()

        async let dashboardLoad = apiClient.getHomeDashboard(accessToken: token, leaderboardRange: request.range)
        async let homeLoad = apiClient.getHome(accessToken: token)
        async let seriesLoad = apiClient.getHomePnLSeries(accessToken: token, range: .oneHour)

        let loadedDashboard = try await dashboardLoad
        let boards = try? await homeLoad
        let series = try? await seriesLoad
        guard generation == refreshGenerationValue(), isCurrent(request), poll == pollGenerationValue(),
            !Task.isCancelled
        else { return }
        QuietUpdate.apply(loadedDashboard, over: dashboard) { dashboard = $0 }
        if let boards { QuietUpdate.apply(boards, over: home) { home = $0 } }
        if let series { QuietUpdate.apply(series.points, over: homePnLSeries) { homePnLSeries = $0 } }
        if profile != nil { errorMessage = nil }
    }
}
