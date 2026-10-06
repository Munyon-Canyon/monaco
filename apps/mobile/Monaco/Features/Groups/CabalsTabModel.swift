import MonacoCore
import Observation
import SwiftUI

/// Reads for the Cabals tab. The live source calls the API; Debug builds can
/// swap in sample data for screenshots.
@MainActor
protocol CabalsTabDataSource {
    func leaderboard() async throws -> GroupLeaderboardResponseDTO
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
}

/// State for the Cabals tab: the platform board.
@Observable
@MainActor
final class CabalsTabModel {
    private(set) var leaderboard: [GroupLeaderboardRowDTO] = []
    private(set) var isLeaderboardLoading = false
    private(set) var leaderboardFailed = false

    /// Set when the server rejects the session; the view signs out.
    private(set) var sessionExpired = false

    private let dataSource: CabalsTabDataSource
    private var reloadTask: Task<Void, Never>?
    /// Bumped on every board load, so a slow response cannot overwrite a newer one.
    private var leaderboardGeneration = 0

    init(dataSource: CabalsTabDataSource) {
        self.dataSource = dataSource
    }

    func reload() async {
        reloadTask?.cancel()
        let task = Task { [weak self] in
            guard let self else { return }
            await self.loadLeaderboard()
        }
        reloadTask = task
        await withTaskCancellationHandler {
            await task.value
        } onCancel: {
            task.cancel()
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
