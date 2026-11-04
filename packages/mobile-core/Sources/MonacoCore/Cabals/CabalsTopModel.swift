import MonacoAPI
import Observation

public struct CabalsTopTile: Identifiable, Equatable, Sendable {
    public let id: String
    public let name: String
    public let pictureURL: String?
    public let line: String
}

@Observable
@MainActor
public final class CabalsTopModel {
    public enum Phase: Equatable, Sendable {
        case loading
        case hidden
        case ready([CabalsTopTile])
    }

    public private(set) var phase = Phase.loading

    private let api: APIClient
    private let hints: any HintSource
    @ObservationIgnored private lazy var refresher = HintRefresher { [weak self] in await self?.load() }

    public init(api: APIClient, hints: any HintSource) {
        self.api = api
        self.hints = hints
    }

    public func load() async {
        do {
            let board = try await api.read {
                try await $0.getCallLeaderboard(query: .init(board: .cabals, period: .month)).ok.body.json
            }
            guard !Task.isCancelled else { return }
            let tiles = Self.tiles(board)
            phase = tiles.isEmpty ? .hidden : .ready(tiles)
        } catch {
            guard !Task.isCancelled else { return }
            if case .ready = phase { return }
            phase = .hidden
        }
    }

    public func observe() async {
        await refresher.observe(hints.hints(matching: .global(what: "leaderboards_updated")))
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }

    static func tiles(_ board: Components.Schemas.CallBoard) -> [CabalsTopTile] {
        let tiles = board.top.compactMap { row -> CabalsTopTile? in
            guard !row.isMine, let ratePct = row.ratePct else { return nil }
            return CabalsTopTile(
                id: row.subject.id, name: row.subject.name, pictureURL: row.subject.pictureUrl,
                line: line(ratePct: ratePct, right: row.right, resolved: row.resolved))
        }
        return Array(tiles.prefix(3))
    }

    static func line<Rate: BinaryInteger, Count: BinaryInteger>(
        ratePct: Rate, right: Count, resolved: Count
    ) -> String {
        "\(ratePct)% right · \(right) of \(resolved)"
    }
}
