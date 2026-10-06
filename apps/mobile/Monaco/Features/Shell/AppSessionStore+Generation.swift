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

    func resolvedAccessToken(_ provided: String?, auth: SessionAuthenticating) async -> String? {
        if let provided { return provided }
        return await accessToken(auth: auth)
    }

    func accessToken(auth: SessionAuthenticating) async -> String? {
        return auth.accessToken
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
        startDeferredWork { [self] in await deferredRefreshAfterCreate(auth: auth) }
    }

    private func deferredRefreshAfterCreate(auth: SessionAuthenticating) async {
        guard let token = await accessToken(auth: auth) else { return }
        let generation = refreshGenerationValue()
        let request = beginDashboardRequest()
        do {
            let loadedDashboard = try await apiClient.getHomeDashboard(accessToken: token)
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
        let generation: Int
    }

    func beginDashboardRequest() -> DashboardRequest {
        nextDashboardRequest()
    }

    func currentDashboardRequest() -> DashboardRequest {
        DashboardRequest(generation: dashboardGenerationValue())
    }

    func isCurrent(_ request: DashboardRequest) -> Bool {
        request.generation == dashboardGenerationValue()
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
    }

    func refreshBoardsAfterProfileWrite(auth: SessionAuthenticating) {
        startDeferredWork { [self] in
            try? await pollLive(auth: auth)
        }
    }

    func pollLive(auth: SessionAuthenticating) async throws {
        guard let token = await accessToken(auth: auth) else { return }
        let generation = refreshGenerationValue()
        let request = currentDashboardRequest()
        let poll = nextPollGeneration()

        let loadedDashboard = try await apiClient.getHomeDashboard(accessToken: token)
        guard generation == refreshGenerationValue(), isCurrent(request), poll == pollGenerationValue(),
            !Task.isCancelled
        else { return }
        QuietUpdate.apply(loadedDashboard, over: dashboard) { dashboard = $0 }
        if profile != nil { errorMessage = nil }
    }
}
