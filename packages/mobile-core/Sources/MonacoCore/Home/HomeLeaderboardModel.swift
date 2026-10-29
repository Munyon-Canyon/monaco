import MonacoAPI
import Observation

public struct HomeLeaderboardRow: Equatable, Sendable {
    public let leaderID: String
    public let leaderName: String
    public let leaderPictureURL: String?
    public let title: String
    public let subtitle: String
    public let rateText: String?

    public init(
        leaderID: String, leaderName: String, leaderPictureURL: String?, leaderPercent: Int?, viewerLeads: Bool,
        rank: Int?, rate: Int?, needed: Int?, totalRanked: Int
    ) {
        self.leaderID = leaderID
        self.leaderName = leaderName
        self.leaderPictureURL = leaderPictureURL
        let who = viewerLeads ? "You lead" : "\(Self.firstName(leaderName)) leads"
        title = leaderPercent.map { "\(who) · \($0)%" } ?? who
        if let rank, rate != nil {
            subtitle = "This month · you are #\(rank) of \(totalRanked)"
        } else if let needed {
            subtitle = "This month · \(needed) more to rank"
        } else {
            subtitle = "This month · \(totalRanked) ranked"
        }
        rateText = rank == nil ? nil : rate.map { "\($0)%" }
    }

    public var spoken: String {
        [title, subtitle, rateText.map { "your rate \($0)" }].compactMap { $0 }.joined(separator: ", ")
    }

    static func firstName(_ name: String) -> String {
        name.split(separator: " ").first.map(String.init) ?? name
    }
}

extension HomeLeaderboardRow {
    init?(_ board: Components.Schemas.CallBoard) {
        guard let leader = board.top.first else { return nil }
        let me = board.me
        self.init(
            leaderID: leader.subject.id, leaderName: leader.subject.name, leaderPictureURL: leader.subject.pictureUrl,
            leaderPercent: leader.ratePct.map(Int.init), viewerLeads: leader.isViewer,
            rank: me?.rank.map(Int.init), rate: me?.ratePct.map(Int.init),
            needed: (me?.needed ?? board.unrankedMe?.needed).map(Int.init), totalRanked: Int(board.totalRanked))
    }
}

@Observable
@MainActor
public final class HomeLeaderboardModel {
    public enum Phase: Equatable, Sendable {
        case loading
        case hidden
        case ready(HomeLeaderboardRow)
        case failed(APIError)
    }

    public private(set) var phase = Phase.loading

    private let api: APIClient
    private let hints: any HintSource
    @ObservationIgnored private lazy var refresher = HintRefresher { [weak self] in await self?.load() }

    public init(api: APIClient, hints: any HintSource) {
        self.api = api
        self.hints = hints
    }

    public var readStatus: HomeReadStatus {
        switch phase {
        case .loading: .pending
        case .hidden, .ready: .settled
        case .failed(.transport): .offline
        case .failed: .failed
        }
    }

    public func load() async {
        do {
            let board = try await api.read {
                try await $0.getCallLeaderboard(query: .init(board: .people, period: .month)).ok.body.json
            }
            guard !Task.isCancelled else { return }
            phase = HomeLeaderboardRow(board).map(Phase.ready) ?? .hidden
        } catch {
            guard !Task.isCancelled else { return }
            if case .ready = phase { return }
            phase = .failed(APIError(error))
        }
    }

    public func observe() async {
        await refresher.observe(hints.hints(matching: .global(what: "leaderboards_updated")))
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }
}
