import MonacoAPI
import MonacoCore

extension AppSessionStore {
    func accessToken(auth: SessionAuthenticating) async -> String? {
        if let token = await sessionToken?(), !token.isEmpty { return token }
        return auth.accessToken
    }
    var joinedCabals: [HomeGroupBoardRowDTO] {
        (home?.groups ?? []).filter(\.isJoined)
    }

    func noteProfileWrite() {
        profileWriteGeneration += 1
    }

    func awaitDeferredWork() async {
        for task in deferredWork {
            await task.value
        }
    }

    func cancelDeferredWork() {
        for task in deferredWork {
            task.cancel()
        }
        deferredWork.removeAll()
    }

    func refreshAfterCreate(auth: SessionAuthenticating, created: CreateGroupResponse) {
        insertJoinedCabal(from: created)
        startDeferredWork { [self] in await deferredRefreshAfterCreate(auth: auth) }
    }

    private func insertJoinedCabal(from created: CreateGroupResponse) {
        let row = HomeGroupBoardRowDTO(
            groupId: created.groupId, name: created.name, potValueUsd: "0.00", percentReturn: nil, dollarPnl: "+0.00",
            isJoined: true)
        if let current = home {
            guard !current.groups.contains(where: { $0.groupId == created.groupId }) else { return }
            home = HomeViewDTO(groups: [row] + current.groups, people: current.people)
        } else {
            home = HomeViewDTO(groups: [row], people: [])
        }
    }

    private func deferredRefreshAfterCreate(auth: SessionAuthenticating) async {
        guard let token = auth.accessToken else { return }
        let generation = refreshGeneration
        let request = beginDashboardRequest()
        do {
            async let homeLoad = apiClient.getHome(accessToken: token)
            async let dashboardLoad = apiClient.getHomeDashboard(accessToken: token, leaderboardRange: request.range)
            async let balanceLoad = apiClient.getPlatformBalance(accessToken: token)
            let loadedHome = try await homeLoad
            guard mayWrite(generation) else { return }
            home = loadedHome
            let loadedDashboard = try await dashboardLoad
            guard mayWrite(generation) else { return }
            apply(loadedDashboard, for: request)
            if let balance = try? await balanceLoad, mayWrite(generation) { platformBalance = balance }
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
        dashboardGeneration += 1
        return DashboardRequest(range: leaderboardRange, generation: dashboardGeneration)
    }

    func currentDashboardRequest() -> DashboardRequest {
        DashboardRequest(range: leaderboardRange, generation: dashboardGeneration)
    }

    func isCurrent(_ request: DashboardRequest) -> Bool {
        request.generation == dashboardGeneration && request.range == leaderboardRange
    }

    func apply(_ loaded: HomeDashboardDTO, for request: DashboardRequest) {
        guard isCurrent(request) else { return }
        dashboard = loaded
    }

    func startDeferredWork(_ work: @escaping @MainActor () async -> Void) {
        deferredWork.removeAll(where: \.isCancelled)
        deferredWork.append(Task { await work() })
    }

    func mayWrite(_ generation: Int) -> Bool {
        generation == refreshGeneration && !Task.isCancelled
    }

    func finishRefresh(
        dashboard: HomeDashboardDTO,
        profile: SessionProfile?,
        balance: PlatformBalanceDTO?,
        generation: Int,
        profileGeneration: Int,
        request: DashboardRequest,
        auth: SessionAuthenticating,
        token: String
    ) {
        guard mayWrite(generation), auth.accessToken == token else { return }
        apply(dashboard, for: request)
        if let profile, profileGeneration == profileWriteGeneration, mayWrite(generation) {
            self.profile = profile
        }
        if let balance, mayWrite(generation) {
            platformBalance = balance
        }
        guard mayWrite(generation) else { return }
        errorMessage = nil
        isBalanceLoading = false
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
        guard let token = auth.accessToken else { return }
        self.leaderboardRange = leaderboardRange
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
}
