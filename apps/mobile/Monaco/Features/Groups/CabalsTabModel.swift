import MonacoCore
import Observation
import SwiftUI

/// Reads for the Cabals tab. The live source calls the API; Debug builds can
/// swap in sample data for screenshots.
@MainActor
protocol CabalsTabDataSource {
    func leaderboard() async throws -> GroupLeaderboardResponseDTO
    func pnlHistory(range: GroupPnLRange) async throws -> MyGroupsPnLHistoryDTO
}

@MainActor
struct LiveCabalsTabDataSource: CabalsTabDataSource {
    let auth: PrivyAuthService
    private let apiClient = MonacoAPIClient()

    init(auth: PrivyAuthService) {
        self.auth = auth
    }

    func leaderboard() async throws -> GroupLeaderboardResponseDTO {
        try await auth.withAccessToken { try await apiClient.groupLeaderboard(accessToken: $0, limit: 20) }
    }

    func pnlHistory(range: GroupPnLRange) async throws -> MyGroupsPnLHistoryDTO {
        try await auth.withAccessToken { try await apiClient.myGroupsPnLHistory(accessToken: $0, range: range) }
    }
}

/// State for the Cabals tab: P&L chart and platform board.
@Observable
@MainActor
final class CabalsTabModel {
    /// A scrub-worthy line needs at least 3 points, and a comparison needs at
    /// least two cabals. Lives here, not in the view, so the chart section and
    /// its tests agree on one rule.
    static func isChartable(_ series: [GroupPnLSeriesDTO]) -> Bool {
        GroupPnLChartModel.drawable(series).filter { $0.points.count >= 3 }.count >= 2
    }

    private(set) var range: GroupPnLRange = .oneMonth
    private(set) var series: [GroupPnLSeriesDTO] = []
    /// The range the drawn lines belong to, or nil when nothing is drawn. A
    /// failed range switch must not leave the previous range's lines under a
    /// newly highlighted chip.
    private(set) var seriesRange: GroupPnLRange?
    /// The range a load is in flight for, or nil. Cleared by the load that set
    /// it and only while that load is still the newest one — see
    /// `chartGeneration`.
    private(set) var loadingRange: GroupPnLRange?
    private(set) var chartFailed = false
    /// Some range has drawn a real chart in this session, so the section has
    /// earned its place. Switching to a range with thin history then shows an
    /// in-card note instead of deleting the section and its range picker.
    private(set) var hasChartableHistory = false

    private(set) var leaderboard: [GroupLeaderboardRowDTO] = []
    private(set) var isLeaderboardLoading = false
    private(set) var leaderboardFailed = false

    /// Set when the server rejects the session; the view signs out.
    private(set) var sessionExpired = false

    private let dataSource: CabalsTabDataSource
    private var chartTask: Task<Void, Never>?
    private var reloadTask: Task<Void, Never>?
    /// Bumped on every board load, so a slow response cannot overwrite a newer one.
    private var leaderboardGeneration = 0
    /// Bumped on every chart load. A load identifies itself by this token, not
    /// by the range it asked for: two loads of the *same* range overlap on
    /// every cold start, and a range cannot tell them apart.
    private var chartGeneration = 0

    init(dataSource: CabalsTabDataSource) {
        self.dataSource = dataSource
    }

    /// True while the chart has nothing to draw and a first load is running.
    var isChartLoading: Bool {
        loadingRange != nil && series.isEmpty
    }

    /// True while a load for the selected range is in flight over lines that are
    /// already drawn — a range switch, or a refresh. The chart on screen is not
    /// yet the chart the highlighted chip promises.
    var isChartReloading: Bool {
        guard let loadingRange else { return false }
        return !series.isEmpty && loadingRange == range
    }

    /// Whether the chart section exists at all — a different question from
    /// whether the *selected* range has enough history. Once any range has drawn
    /// a real chart the section stays put, so tapping 1D on young cabals shows a
    /// note inside the card instead of deleting the section and its range
    /// picker. Lives here, not in the view, so a test can pin it directly.
    func showsChartSection(hasCabals: Bool) -> Bool {
        guard hasCabals else { return false }
        if isChartLoading { return true }
        if chartFailed, series.isEmpty { return true }
        return Self.isChartable(series) || hasChartableHistory
    }

    // MARK: Chart + board

    /// Reloads the chart and the board. Cancel-and-replace: a reload started
    /// while another is in flight supersedes it, so a membership change and a
    /// pull-to-refresh in the same breath cost one round of requests, not two.
    func reload(hasCabals: Bool = true) async {
        reloadTask?.cancel()
        let task = Task { [weak self] in
            guard let self else { return }
            // No cabals means no chart on screen; don't pay for the history read.
            guard hasCabals else {
                await self.loadLeaderboard()
                return
            }
            async let chart: Void = self.loadChart()
            async let board: Void = self.loadLeaderboard()
            _ = await (chart, board)
        }
        reloadTask = task
        // `Task.value` on a non-throwing task is not a cancellation point and
        // does not pass cancellation on, so tearing down the tab's `.task` used
        // to leave both reads running and still writing the model. Forward it.
        await withTaskCancellationHandler {
            await task.value
        } onCancel: {
            task.cancel()
        }
    }

    func selectRange(_ newRange: GroupPnLRange) {
        guard newRange != range else { return }
        range = newRange
        chartTask?.cancel()
        chartTask = Task { await loadChart(newRange) }
    }

    /// `requested` is captured by the caller rather than read here, so a task
    /// that starts after a newer range was picked still knows which range it is
    /// the load for.
    ///
    /// Identity is the generation token, not the range. Keying the spinner on
    /// the range meant two loads for the same range — `.task` calling `reload()`
    /// and `onChange(of: joinedIDs)` calling it again the moment `/v1/home`
    /// lands, which is every cold start — each answered to `.oneMonth`, so
    /// whichever exited first cleared the other's flag while it was still out.
    func loadChart(_ requested: GroupPnLRange? = nil) async {
        let requested = requested ?? range
        chartGeneration += 1
        let generation = chartGeneration
        loadingRange = requested
        defer {
            if generation == chartGeneration { loadingRange = nil }
        }
        do {
            let history = try await dataSource.pnlHistory(range: requested)
            guard generation == chartGeneration, !Task.isCancelled else { return }
            series = history.series
            seriesRange = requested
            hasChartableHistory = hasChartableHistory || Self.isChartable(history.series)
            chartFailed = false
        } catch {
            guard generation == chartGeneration, !Task.isCancelled else { return }
            handle(error) {
                // A range switch that failed: the lines on screen are the range
                // the member just left. Leaving them under a newly highlighted
                // chip is exactly the disagreement the dimming was added for, so
                // drop them and let the card say the load failed. A failed
                // refresh of the range already drawn keeps what it has.
                if seriesRange != requested {
                    series = []
                    seriesRange = nil
                }
                chartFailed = true
            }
        }
    }

    func loadLeaderboard() async {
        leaderboardGeneration += 1
        let generation = leaderboardGeneration
        isLeaderboardLoading = leaderboard.isEmpty
        defer {
            if generation == leaderboardGeneration { isLeaderboardLoading = false }
        }
        do {
            let rows = try await dataSource.leaderboard().groups
            guard generation == leaderboardGeneration, !Task.isCancelled else { return }
            leaderboard = rows
            leaderboardFailed = false
        } catch {
            guard generation == leaderboardGeneration, !Task.isCancelled else { return }
            handle(error) { leaderboardFailed = true }
        }
    }

    private func handle(_ error: Error, otherwise: () -> Void) {
        if error.isRequestCancellation { return }
        if case MonacoAPIError.httpStatus(401) = error {
            sessionExpired = true
            return
        }
        otherwise()
    }
}
