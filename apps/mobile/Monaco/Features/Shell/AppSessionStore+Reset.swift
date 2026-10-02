extension AppSessionStore {
    func reset() {
        cancelDeferredWork()
        refreshGeneration += 1
        dashboardGeneration += 1
        pollGeneration += 1
        profileWriteGeneration += 1
        home = nil
        dashboard = nil
        profile = nil
        platformBalance = nil
        popularAssets = []
        homePnLSeries = nil
        isHomePnLSeriesLoading = false
        isBalanceLoading = false
        errorMessage = nil
        #if DEBUG
        errorDebugDetail = nil
        #endif
        isLoading = false
        leaderboardRange = .all
        skipsSessionOpen = false
    }
}
