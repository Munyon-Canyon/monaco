import Foundation
import MonacoCore
import Testing

@testable import Monaco

private typealias CabalsTabModel = Monaco.CabalsTabModel

@MainActor
private final class CallGate {
    private var waiters: [CheckedContinuation<Void, Never>] = []
    private var banked = 0

    func wait() async {
        if banked > 0 {
            banked -= 1
            return
        }
        await withCheckedContinuation { waiters.append($0) }
    }

    func release() {
        if waiters.isEmpty {
            banked += 1
            return
        }
        waiters.removeFirst().resume()
    }
}

@MainActor
private final class RecordingDataSource: CabalsTabDataSource {
    var boardRows: [GroupLeaderboardRowDTO] = []
    var boardLoads = 0
    var boardError: Error?
    var boardGate: (() async -> Void)?

    func leaderboard() async throws -> GroupLeaderboardResponseDTO {
        boardLoads += 1
        if let boardGate { await boardGate() }
        if let boardError { throw boardError }
        return GroupLeaderboardResponseDTO(groups: boardRows)
    }
}

private func boardRow(_ id: String) -> GroupLeaderboardRowDTO {
    GroupLeaderboardRowDTO(
        rank: 1, groupID: id, name: "Cabal \(id)", memberCount: 3, potValueUsd: "100.00", percentReturn: "0.10",
        dollarPnl: "+10.00", isJoined: false, joinMode: .open)
}

@MainActor
struct CabalsTabModelTests {
    @Test func reloadLoadsTheBoard() async {
        let source = RecordingDataSource()
        source.boardRows = [boardRow("a")]
        let model = CabalsTabModel(dataSource: source)

        await model.reload()

        #expect(model.leaderboard.map(\.groupID) == ["a"])
        #expect(!model.leaderboardFailed)
        #expect(!model.isLeaderboardLoading)
    }

    @Test func aFailedBoardLoadIsMarkedFailed() async {
        let source = RecordingDataSource()
        source.boardError = Monaco.MonacoAPIError.httpStatus(500)
        let model = CabalsTabModel(dataSource: source)

        await model.reload()

        #expect(model.leaderboardFailed)
        #expect(!model.sessionExpired)
    }

    @Test func aRejectedSessionIsReportedForTheViewToSignOut() async {
        let source = RecordingDataSource()
        source.boardError = Monaco.MonacoAPIError.httpStatus(401)
        let model = CabalsTabModel(dataSource: source)

        await model.reload()

        #expect(model.sessionExpired)
        #expect(!model.leaderboardFailed)
    }

    @Test func aSupersededReloadWritesNothingAndTheNewerOneWins() async throws {
        let source = RecordingDataSource()
        source.boardRows = [boardRow("a")]
        let gate = CallGate()
        source.boardGate = { await gate.wait() }
        let model = CabalsTabModel(dataSource: source)

        let first = Task { await model.reload() }
        try await waitUntil("the first read to start") { source.boardLoads == 1 }
        source.boardRows = [boardRow("b")]
        let second = Task { await model.reload() }
        try await waitUntil("the second read to start") { source.boardLoads == 2 }

        gate.release()
        await first.value
        #expect(model.leaderboard.isEmpty)

        gate.release()
        await second.value

        #expect(model.leaderboard.map(\.groupID) == ["b"])
        #expect(!model.isLeaderboardLoading)
    }
}

extension CabalsTabModelTests {
    fileprivate func waitUntil(_ description: String, _ condition: () -> Bool) async throws {
        let start = ContinuousClock.now
        while !condition() {
            if ContinuousClock.now - start > .seconds(2) {
                Issue.record("timed out waiting for \(description)")
                return
            }
            await Task.yield()
        }
    }
}
