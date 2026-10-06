import MonacoAPI
import Observation

@Observable
@MainActor
public final class CabalValueHistoryModel {
    public enum Phase: Equatable, Sendable {
        case loading
        case hidden
        case failed
        case loaded
    }

    public struct Line: Equatable, Sendable, Identifiable {
        public let id: String
        public let name: String
        public let curve: ValueCurve
    }

    private let portfolio: PortfolioModel
    private let chart: ValueChartModel
    private let hints: any HintSource

    @ObservationIgnored private lazy var refresher = HintRefresher { [weak self] in await self?.refresh() }

    public init(api: APIClient, hints: any HintSource) {
        self.hints = hints
        portfolio = PortfolioModel(api: api, hints: hints)
        chart = ValueChartModel(
            subjects: [], ranges: Self.ranges, range: .oneMonth, api: api, hints: hints)
    }

    public static let ranges: [LeaderboardRange] = [.oneDay, .oneWeek, .oneMonth, .all]

    public var range: LeaderboardRange { chart.range }

    public var ranges: [LeaderboardRange] { chart.ranges }

    public var phase: Phase {
        switch portfolio.state {
        case .idle, .loading: return .loading
        case .failed: return .failed
        case .loaded(let summary):
            if summary.isEmpty { return .hidden }
            switch chart.state {
            case .idle, .loading: return .loading
            case .failed: return .failed
            case .loaded: return .loaded
            }
        }
    }

    public var lines: [Line] {
        guard let summary = portfolio.summary, let curves = chart.curves else { return [] }
        return summary.rows.compactMap { row in
            curves[.cabal(id: row.id)].map { Line(id: row.id, name: row.name, curve: $0) }
        }
    }

    public var hasEnoughHistory: Bool {
        lines.contains { $0.curve.hasEnoughHistory }
    }

    public var toast: String? { portfolio.toast ?? chart.toast }

    public func load() async {
        await refresh()
    }

    public func select(_ range: LeaderboardRange) async {
        await chart.select(range)
    }

    public func observe() async {
        let chart = chart
        let refresher = refresher
        let streams = [
            hints.hints(matching: .user(what: nil)),
            hints.hints(matching: .global(what: "leaderboards_updated")),
        ]
        await withTaskGroup(of: Void.self) { group in
            group.addTask { await chart.observe() }
            group.addTask { await refresher.observe(streams) }
        }
    }

    public func setVisible(_ visible: Bool) {
        chart.setVisible(visible)
        refresher.setVisible(visible)
    }

    public func dismissToast() {
        portfolio.dismissToast()
        chart.dismissToast()
    }

    private func refresh() async {
        await portfolio.load()
        guard let summary = portfolio.summary else { return }
        let subjects = summary.rows.map { PnLHistoryLoader.Subject.cabal(id: $0.id) }
        await chart.setSubjects(subjects)
    }
}
