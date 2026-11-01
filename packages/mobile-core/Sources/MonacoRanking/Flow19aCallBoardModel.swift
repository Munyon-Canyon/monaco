import Foundation
import MonacoAPI
import Observation

@Observable
@MainActor
public final class CallBoardModel {
    public private(set) var state: LoadState<CallBoardScreen> = .idle
    public private(set) var period: CallPeriod
    public private(set) var tab: CallBoardTab
    public private(set) var refreshFailed = false

    private let api: APIClient
    private let hints: any HintSource
    private var generation = 0
    private var isPaging = false
    @ObservationIgnored private lazy var refresher = HintRefresher { [weak self] in await self?.load() }

    public init(api: APIClient, hints: any HintSource, board: CallBoardKind, period: CallPeriod, filter: CallFilter) {
        self.api = api
        self.hints = hints
        self.period = period
        tab = CallBoardTab(board: board, filter: filter)
    }

    public func select(period: CallPeriod) async {
        guard period != self.period else { return }
        self.period = period
        await load()
    }

    public func select(tab: CallBoardTab) async {
        guard tab != self.tab else { return }
        self.tab = tab
        generation += 1
        state = .loading
        refreshFailed = false
        await load()
    }

    public func load() async {
        generation += 1
        let mine = generation
        if case .loaded = state {} else { state = .loading }
        let (board, period, filter) = (tab.board, period, tab.filter)
        do {
            let result = try await api.read { client in
                try await client.getCallLeaderboard(
                    query: .init(
                        board: board == .cabals ? .cabals : .people,
                        period: Self.generated(period),
                        filter: filter == .friends ? .friends : .all)
                ).ok.body.json
            }
            guard mine == generation else { return }
            state = .loaded(CallBoardScreen(result))
            refreshFailed = false
        } catch {
            guard mine == generation else { return }
            if case .loaded = state {
                refreshFailed = true
                return
            }
            state = .failed(APIError(error))
        }
    }

    public func loadMore() async {
        guard case .loaded(let screen) = state, let cursor = screen.afterCursor, !isPaging else { return }
        isPaging = true
        defer { isPaging = false }
        let mine = generation
        let (board, period, filter) = (tab.board, period, tab.filter)
        do {
            let page = try await api.read { client in
                try await client.getCallLeaderboard(
                    query: .init(
                        board: board == .cabals ? .cabals : .people,
                        period: Self.generated(period),
                        filter: filter == .friends ? .friends : .all,
                        cursor: cursor)
                ).ok.body.json
            }
            guard mine == generation, case .loaded(let current) = state else { return }
            state = .loaded(current.appending(page))
        } catch {
            guard mine == generation, case .problem(let problem) = APIError(error),
                problem.code == .known(.cursorStale)
            else { return }
            await load()
        }
    }

    public func observe() async {
        await refresher.observe(hints.hints(matching: .global(what: "leaderboards_updated")))
    }

    public func setVisible(_ visible: Bool) {
        refresher.setVisible(visible)
    }

    private nonisolated static func generated(_ period: CallPeriod)
        -> Operations.GetCallLeaderboard.Input.Query
        .PeriodPayload
    {
        switch period {
        case .week: .week
        case .month: .month
        case .all: .all
        }
    }
}

#if DEBUG
extension CallBoardModel {
    public static func preview(
        state: LoadState<CallBoardScreen>, tab: CallBoardTab = .people, period: CallPeriod = .month
    ) -> CallBoardModel {
        let serverURL = URL(string: "http://127.0.0.1:9") ?? URL(fileURLWithPath: "/")
        let model = CallBoardModel(
            api: APIClient(serverURL: serverURL, tokens: PreviewTokens(), transport: PreviewTransport()),
            hints: PreviewHints(), board: tab.board, period: period, filter: tab.filter)
        model.state = state
        return model
    }
}

private struct PreviewTransport: ClientTransport {
    func send(
        _ request: HTTPRequest, body _: HTTPBody?, baseURL _: URL, operationID _: String
    ) async throws -> (HTTPResponse, HTTPBody?) {
        throw URLError(.notConnectedToInternet)
    }
}

private struct PreviewTokens: AccessTokenProvider {
    func accessToken() async throws -> String? { "preview-token" }
    func refreshedToken(replacing _: String) async throws -> String? { nil }
}

private struct PreviewHints: HintSource {
    func hints(matching _: HintFilter) -> AsyncStream<Hint> { AsyncStream { $0.finish() } }
}
#endif
